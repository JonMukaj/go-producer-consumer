package config

import (
	"os"
	"testing"
)

var producerYAML = []byte(`
prometheus_port: 9090
profiling_port: 6060
log_level: info
log_format: json
grpc_target: "localhost:50051"
rate_per_second: 5
max_backlog: 100
db:
  host: localhost
  port: 5432
  user: postgres
  password: postgres
  name: tasks
  sslmode: disable
`)

var consumerYAML = []byte(`
prometheus_port: 9091
profiling_port: 6061
log_level: debug
log_format: console
grpc_listen: ":50051"
rate_limit: 10
rate_burst: 20
db:
  host: localhost
  port: 5432
  user: postgres
  password: postgres
  name: tasks
  sslmode: disable
`)

func TestLoadProducer_Defaults(t *testing.T) {
	cfg, err := LoadProducer(producerYAML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PrometheusPort != 9090 {
		t.Errorf("want PrometheusPort=9090, got %d", cfg.PrometheusPort)
	}
	if cfg.RatePerSecond != 5 {
		t.Errorf("want RatePerSecond=5, got %d", cfg.RatePerSecond)
	}
	if cfg.MaxBacklog != 100 {
		t.Errorf("want MaxBacklog=100, got %d", cfg.MaxBacklog)
	}
	if cfg.DB.Host != "localhost" {
		t.Errorf("want DB.Host=localhost, got %s", cfg.DB.Host)
	}
}

func TestLoadProducer_EnvOverride(t *testing.T) {
	t.Setenv("PRODUCER_RATE_PER_SECOND", "20")
	t.Setenv("PRODUCER_MAX_BACKLOG", "500")

	cfg, err := LoadProducer(producerYAML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RatePerSecond != 20 {
		t.Errorf("want RatePerSecond=20 from env, got %d", cfg.RatePerSecond)
	}
	if cfg.MaxBacklog != 500 {
		t.Errorf("want MaxBacklog=500 from env, got %d", cfg.MaxBacklog)
	}
}

func TestLoadConsumer_Defaults(t *testing.T) {
	cfg, err := LoadConsumer(consumerYAML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PrometheusPort != 9091 {
		t.Errorf("want PrometheusPort=9091, got %d", cfg.PrometheusPort)
	}
	if cfg.RateLimit != 10 {
		t.Errorf("want RateLimit=10, got %f", cfg.RateLimit)
	}
	if cfg.RateBurst != 20 {
		t.Errorf("want RateBurst=20, got %d", cfg.RateBurst)
	}
	if cfg.LogFormat != "console" {
		t.Errorf("want LogFormat=console, got %s", cfg.LogFormat)
	}
}

func TestLoadConsumer_EnvOverride(t *testing.T) {
	t.Setenv("CONSUMER_RATE_LIMIT", "50")
	t.Setenv("CONSUMER_LOG_LEVEL", "warn")

	cfg, err := LoadConsumer(consumerYAML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RateLimit != 50 {
		t.Errorf("want RateLimit=50 from env, got %f", cfg.RateLimit)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("want LogLevel=warn from env, got %s", cfg.LogLevel)
	}
}

func TestDB_DSN(t *testing.T) {
	d := DB{Host: "localhost", Port: 5432, User: "u", Password: "p", Name: "db", SSLMode: "disable"}
	want := "host=localhost port=5432 user=u password=p dbname=db sslmode=disable"
	if got := d.DSN(); got != want {
		t.Errorf("DSN mismatch\nwant: %s\n got: %s", want, got)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
