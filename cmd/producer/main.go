package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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

	cfg, err := config.LoadProducer(configs.ProducerYAML)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logger := buildLogger(cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(logger)

	// ── Prometheus ────────────────────────────────────────────────────────
	reg := prometheus.NewRegistry()
	m := metrics.NewProducerMetrics(reg)

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

	// ── gRPC client ───────────────────────────────────────────────────────
	conn, err := grpc.NewClient(cfg.GRPCTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("failed to connect to consumer", "target", cfg.GRPCTarget, "err", err)
		os.Exit(1)
	}
	defer conn.Close()
	client := taskpb.NewTaskServiceClient(conn)

	// ── Produce loop ──────────────────────────────────────────────────────
	interval := time.Second / time.Duration(cfg.RatePerSecond)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("producer started",
		"version", Version,
		"rate_per_second", cfg.RatePerSecond,
		"max_backlog", cfg.MaxBacklog,
		"target", cfg.GRPCTarget,
	)

	var produced int64

	for {
		select {
		case <-ctx.Done():
			slog.Info("producer shutting down")
			return
		case <-ticker.C:
			if produced >= int64(cfg.MaxBacklog) {
				slog.Warn("backlog limit reached — stopping production",
					"produced", produced,
					"max_backlog", cfg.MaxBacklog,
				)
				ticker.Stop()
				// Keep the process alive so Prometheus can continue scraping metrics.
				<-ctx.Done()
				return
			}

			taskType := int32(rand.Intn(10))
			taskValue := int32(rand.Intn(100))
			now := float64(time.Now().UnixNano()) / 1e9

			row, err := s.CreateTask(ctx, db.CreateTaskParams{
				Type:         taskType,
				Value:        taskValue,
				CreationTime: now,
			})
			if err != nil {
				slog.Error("failed to insert task", "err", err)
				continue
			}

			_, err = client.SubmitTask(ctx, &taskpb.Task{
				Id:    int64(row.ID),
				Type:  taskType,
				Value: taskValue,
			})
			if err != nil {
				slog.Error("failed to submit task to consumer", "task_id", row.ID, "err", err)
				continue
			}

			produced++
			m.TasksProduced.Inc()
			slog.Debug("task produced", "task_id", row.ID, "type", taskType, "value", taskValue)
		}
	}
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
