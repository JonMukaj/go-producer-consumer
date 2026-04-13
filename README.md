# go-producer-consumer

A Go producer/consumer task processing system built for the DevOps interview technical task.

## Tech Choices

| Concern | Choice | Why |
|---|---|---|
| **Communication** | gRPC (HTTP/2) | Type-safe via proto, binary protocol, no Swagger needed, built-in streaming path for future use |
| **Database** | PostgreSQL (Docker) | Native enum support for task state, better concurrent access than SQLite, production-realistic |
| **Logging** | `log/slog` (stdlib) | Standard library, supports JSON + console handlers, structured key-value logs |
| **Config** | `github.com/spf13/viper` | Env var override + YAML in one package; defaults embedded via `go:embed` |
| **Migrations** | `golang-migrate/migrate` | Required by task |
| **SQL codegen** | `sqlc` | Required by task |

## Goroutines / Channels / Mutexes — Design Rationale

- **Producer**: single `time.Ticker` goroutine — no channel needed because the DB is the backlog buffer. Rate is enforced by the ticker interval.
- **Consumer**: gRPC server spawns one goroutine per incoming request automatically. The per-type `valueSums` map is protected by a `sync.Mutex` (low contention, simpler than a channel-based accumulator for this use case).
- **Rate limiter**: `golang.org/x/time/rate` token bucket — uses its own internal mutex; no custom sync required.

## Scalability Bottlenecks

- Single Postgres instance is the primary bottleneck at scale → partitioned tasks table + read replicas for metric queries.
- gRPC is synchronous per-task send → future: gRPC streaming or a queue (Kafka/RabbitMQ) for true decoupling.
- Consumer's sleep-based processing is I/O-bound → a goroutine pool with a semaphore pattern would increase throughput.

---

## Quick Start

```bash
# Start everything (postgres, consumer, producer, prometheus, grafana)
# Migrations run automatically on startup — no separate step needed
docker compose up --build -d

# Grafana:    http://localhost:3000  (admin / admin)
# Prometheus: http://localhost:9092
```

### Fresh Start (wipe DB and restart)

```bash
# Removes all containers, volumes (postgres data), and rebuilds images from scratch
docker compose down -v && docker compose up --build -d
```

## Running Locally (without Docker)

```bash
# 1. Start Postgres
docker run -d --name pg -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=tasks -p 5432:5432 postgres:16-alpine

# 2. Run migrations
make migrate-up

# 3. Build
make build

# 4. Start consumer first
./bin/consumer

# 5. Start producer
./bin/producer
```

---

## Build Flags

### Standard build (strip debug info + inject version)

```bash
make build
# expands to:
go build -ldflags="-s -w -X main.Version=$(git describe --tags --always)" -o bin/producer ./cmd/producer
go build -ldflags="-s -w -X main.Version=$(git describe --tags --always)" -o bin/consumer ./cmd/consumer
```

- `-s` strips the symbol table
- `-w` strips DWARF debug info
- `-X main.Version=<ver>` injects the version string at link time

Check all available linker flags:
```bash
go tool link   # shows all -ldflags options
go tool compile -help  # shows all compiler flags
```

### Version flag

```bash
./bin/producer -version
./bin/consumer -version
```

### Profile-Guided Optimization (PGO)

PGO uses a real CPU profile from a running binary to guide inlining and optimisation decisions on the next build.

```bash
# Step 1 — run the producer and collect a 30-second CPU profile
make pgo-producer
# internally:
#   curl -o default.pgo http://localhost:6060/debug/pprof/profile?seconds=30
#   go build -pgo=default.pgo -ldflags="..." -o bin/producer-pgo ./cmd/producer

# Step 2 — compare binary sizes / benchmark to observe the difference
ls -lh bin/producer bin/producer-pgo
```

---

## Database Migrations (live)

```bash
# Apply all migrations (runs automatically on startup via golang-migrate embedded in each service)
make migrate-up

# Add the comment column while services are running
migrate -path migrations -database "postgres://postgres:postgres@localhost:5432/tasks?sslmode=disable" up

# Remove the comment column while services are running
make migrate-down
```

---

## Testing

```bash
# Unit tests only (no DB required)
go test -short -cover ./...

# All tests including DB integration (requires Postgres)
DB_URL="host=localhost port=5432 user=postgres password=postgres dbname=tasks sslmode=disable" \
  go test -cover ./...

# Coverage report
make test        # generates coverage.html
```

---

## Profiling

pprof ports: producer `6060`, consumer `6061`.

### Flame graph (CPU)

```bash
make profile-cpu-producer   # collect 30s CPU profile, open flame graph on :8080
make profile-cpu-consumer   # same for consumer, opens on :8081
```

### Memory graph (heap)

```bash
make profile-heap-producer  # snapshot live heap, open graph on :8082
make profile-heap-consumer  # same for consumer, opens on :8083
```

### Trace (goroutine timeline)

```bash
make profile-trace-producer
make profile-trace-consumer
```

---

## GOGC vs GOMEMLIMIT

| Parameter | Effect | When to tune |
|---|---|---|
| `GOGC=100` (default) | GC triggers when heap doubles | Lower (e.g. 50) → more frequent GC → lower peak memory, more CPU |
| `GOMEMLIMIT=128MiB` | Hard ceiling; GC goes aggressive before OOM | Set to ~80% of container memory limit |

Both are exposed as environment variables in `docker-compose.yml` so they can be tuned without a rebuild.

### Comparing GOGC settings

```bash
# Heap comparison
make gogc-baseline        # snapshot heap at GOGC=100
make gogc-aggressive      # restart consumer at GOGC=5, snapshot heap
make gogc-compare-heap    # open both side by side (:8084 and :8085)

# CPU comparison
make gogc-cpu-baseline    # collect CPU profile at GOGC=100
make gogc-cpu-aggressive  # restart consumer at GOGC=5, collect CPU profile
make gogc-compare-cpu     # open both side by side (:8086 and :8087)
```

### Comparing GOMEMLIMIT settings

```bash
# Heap comparison
make gomemlimit-baseline      # snapshot heap at GOMEMLIMIT=128MiB
make gomemlimit-constrained   # restart consumer at GOMEMLIMIT=32MiB, snapshot heap
make gomemlimit-compare-heap  # open both side by side (:8088 and :8089)

# CPU comparison
make gomemlimit-cpu-baseline      # collect CPU profile at GOMEMLIMIT=128MiB
make gomemlimit-cpu-constrained   # restart consumer at GOMEMLIMIT=32MiB, collect CPU profile
make gomemlimit-compare-cpu       # open both side by side (:8090 and :8091)
```
