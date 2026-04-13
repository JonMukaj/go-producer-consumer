package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/JonMukaj/go-producer-consumer/configs"
	"github.com/JonMukaj/go-producer-consumer/internal/config"
	store "github.com/JonMukaj/go-producer-consumer/internal/db"
	db "github.com/JonMukaj/go-producer-consumer/internal/db/generated"
	"github.com/JonMukaj/go-producer-consumer/internal/metrics"
	taskpb "github.com/JonMukaj/go-producer-consumer/internal/proto/gen"
)

// Version is injected at build time via -ldflags="-X main.Version=<ver>".
var Version = "dev"

func main() {
	versionFlag := flag.Bool("version", false, "print build version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println(Version)
		os.Exit(0)
	}

	cfg, err := config.LoadConsumer(configs.ConsumerYAML)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logger := buildLogger(cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(logger)

	// ── Prometheus ────────────────────────────────────────────────────────
	reg := prometheus.NewRegistry()
	m := metrics.NewConsumerMetrics(reg)

	go serveHTTP(fmt.Sprintf(":%d", cfg.PrometheusPort), promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), "metrics")

	// ── pprof ─────────────────────────────────────────────────────────────
	go serveHTTP(fmt.Sprintf(":%d", cfg.ProfilingPort), http.DefaultServeMux, "pprof")

	// ── DB ────────────────────────────────────────────────────────────────
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := store.Migrate(cfg.DB.DSN()); err != nil {
		slog.Error("migration failed", "err", err)
		os.Exit(1)
	}

	s, err := store.New(ctx, cfg.DB.DSN())
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer s.Close()

	// ── gRPC server ───────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", cfg.GRPCListen)
	if err != nil {
		slog.Error("failed to listen", "addr", cfg.GRPCListen, "err", err)
		os.Exit(1)
	}

	limiter := rate.NewLimiter(rate.Limit(cfg.RateLimit), cfg.RateBurst)
	srv := &taskServer{
		store:   s,
		limiter: limiter,
		metrics: m,
		// valueSums is a per-type running total, protected by mu.
		valueSums: make(map[int32]int64),
	}

	grpcSrv := grpc.NewServer()
	taskpb.RegisterTaskServiceServer(grpcSrv, srv)

	slog.Info("consumer started",
		"version", Version,
		"listen", cfg.GRPCListen,
		"rate_limit", cfg.RateLimit,
		"rate_burst", cfg.RateBurst,
	)

	go func() {
		<-ctx.Done()
		slog.Info("consumer shutting down")
		grpcSrv.GracefulStop()
	}()

	if err := grpcSrv.Serve(lis); err != nil {
		slog.Error("grpc server error", "err", err)
	}
}

// taskServer implements the TaskService gRPC interface.
type taskServer struct {
	taskpb.UnimplementedTaskServiceServer
	store   *store.Store
	limiter *rate.Limiter
	metrics *metrics.ConsumerMetrics

	mu        sync.Mutex
	valueSums map[int32]int64
}

func (s *taskServer) SubmitTask(ctx context.Context, t *taskpb.Task) (*taskpb.TaskResponse, error) {
	if !s.limiter.Allow() {
		return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}

	now := float64(time.Now().UnixNano()) / 1e9

	// Mark as processing.
	if err := s.store.UpdateTaskState(ctx, db.UpdateTaskStateParams{
		ID:             int32(t.Id),
		State:          db.TaskStateProcessing,
		LastUpdateTime: now,
	}); err != nil {
		slog.Error("failed to update task to processing", "task_id", t.Id, "err", err)
		return nil, status.Error(codes.Internal, "db update failed")
	}
	s.metrics.TasksProcessing.Inc()

	// Simulate work: sleep for value milliseconds.
	time.Sleep(time.Duration(t.Value) * time.Millisecond)

	now = float64(time.Now().UnixNano()) / 1e9

	// Mark as done.
	if err := s.store.UpdateTaskState(ctx, db.UpdateTaskStateParams{
		ID:             int32(t.Id),
		State:          db.TaskStateDone,
		LastUpdateTime: now,
	}); err != nil {
		slog.Error("failed to update task to done", "task_id", t.Id, "err", err)
		return nil, status.Error(codes.Internal, "db update failed")
	}

	// Update per-type running total.
	s.mu.Lock()
	s.valueSums[t.Type] += int64(t.Value)
	runningSum := s.valueSums[t.Type]
	s.mu.Unlock()

	// Prometheus.
	typeLabel := strconv.Itoa(int(t.Type))
	s.metrics.TasksDone.Inc()
	s.metrics.TasksByType.WithLabelValues(typeLabel).Inc()
	s.metrics.ValueSumByType.WithLabelValues(typeLabel).Add(float64(t.Value))

	slog.Info("task done",
		"task_id", t.Id,
		"type", t.Type,
		"value", t.Value,
		"running_value_sum_for_type", runningSum,
	)

	return &taskpb.TaskResponse{Accepted: true}, nil
}

func buildLogger(level, format string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	if format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func serveHTTP(addr string, handler http.Handler, name string) {
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	srv := &http.Server{Addr: addr, Handler: mux}
	slog.Info("http server listening", "name", name, "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("http server error", "name", name, "err", err)
	}
}
