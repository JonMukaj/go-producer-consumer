# DevOps / Golang Task — Implementation Plan

## Tech Decisions (with rationale)

| Concern | Choice | Why |
|---|---|---|
| **Communication** | **gRPC (HTTP/2)** | No Swagger docs needed (saves ~1h), type-safe via proto, binary protocol handles high-throughput data well, built-in streaming for future scalability |
| **Database** | **PostgreSQL (Docker)** | More production-realistic than SQLite, native enum support for task state, better concurrent access from two services, works well with sqlc |
| **Logging** | **`log/slog`** (stdlib, Go 1.21+) | Standard library preference explicitly stated, supports JSON structured logs + console, no extra dependency |
| **Config** | **`github.com/spf13/viper`** | Explicitly listed in helpful materials, env var override + YAML config + flag binding in one package |
| **Migrations** | **`golang-migrate/migrate`** | Explicitly required |
| **SQL codegen** | **`sqlc`** | Explicitly required |

---

## Repository Structure

```
go-marbl/
├── cmd/
│   ├── producer/
│   │   └── main.go            # entry point, -version flag, GOGC/GOMEMLIMIT awareness
│   └── consumer/
│       └── main.go            # entry point, -version flag
├── internal/
│   ├── config/
│   │   ├── producer.go        # producer config struct + viper load
│   │   ├── consumer.go        # consumer config struct + viper load
│   │   └── config_test.go
│   ├── db/                    # SHARED between both services
│   │   ├── queries/
│   │   │   └── tasks.sql      # sqlc input queries
│   │   ├── sqlc.yaml
│   │   └── generated/         # sqlc output (db.go, models.go, queries.go)
│   ├── metrics/
│   │   ├── producer.go        # producer prometheus counters/gauges
│   │   └── consumer.go        # consumer prometheus counters/gauges
│   └── proto/
│       ├── task.proto          # gRPC service + Task message definition
│       └── gen/               # protoc generated Go code
├── migrations/
│   ├── 000001_create_tasks.up.sql
│   ├── 000001_create_tasks.down.sql
│   ├── 000002_add_comment.up.sql    # demo migration
│   └── 000002_add_comment.down.sql  # demo downgrade
├── configs/
│   ├── producer.yaml           # default config (embedded via go:embed)
│   └── consumer.yaml           # default config (embedded via go:embed)
├── monitoring/
│   ├── prometheus.yml
│   └── grafana/
│       └── provisioning/
│           ├── datasources/datasource.yml
│           └── dashboards/
│               ├── dashboard.yml
│               └── tasks.json  # 4 panels pre-configured
├── docker-compose.yml
├── Dockerfile.producer
├── Dockerfile.consumer
├── Makefile
└── README.md
```

---

## Implementation Phases (8 hours total)

### Phase 1 — Scaffolding (45 min)
- [ ] `git init`, `go mod init github.com/<user>/go-marbl`
- [ ] Create directory structure above
- [ ] Write `task.proto` (Task message, TaskService with `SubmitTask` RPC)
- [ ] Run `protoc` to generate Go stubs
- [ ] Write `Makefile` targets: `proto`, `sqlc`, `migrate-up`, `migrate-down`, `build`, `test`, `docker-up`

### Phase 2 — Database Layer (45 min)
- [ ] Write migration `000001`: `tasks` table with columns: `id SERIAL PRIMARY KEY`, `type INT`, `value INT`, `state task_state ENUM('received','processing','done')`, `creation_time FLOAT8`, `last_update_time FLOAT8`
- [ ] Write migration `000002`: add `comment TEXT` column (demo only)
- [ ] Write `sqlc.yaml` pointing at Postgres driver
- [ ] Write SQL queries (`tasks.sql`): `CreateTask`, `UpdateTaskState`, `GetTask`, `CountByState`, `SumValueByType`, `CountByType`
- [ ] Run `sqlc generate`
- [ ] Write thin wrapper `db/store.go` with `New(connStr string) (*Store, error)` — reused by both services

### Phase 3 — Config + Embed (30 min)
- [ ] Write `configs/producer.yaml` with all required fields
- [ ] Write `configs/consumer.yaml` with all required fields
- [ ] In each `cmd/*/main.go`: `//go:embed ../../configs/producer.yaml` as default, override with viper from env/file
- [ ] Implement `-version` flag in both `main.go` using `ldflags` injected `var Version = "dev"`

### Phase 4 — Producer Service (75 min)
- [ ] Ticker loop generating tasks at configured `rate_per_second`
- [ ] For each tick: generate `type = rand.Intn(10)`, `value = rand.Intn(100)`
- [ ] Insert task to DB (state = `received`)
- [ ] Send via gRPC to consumer
- [ ] Backlog check: count `received` tasks in DB; if `>= max_backlog`, pause/stop production
- [ ] Prometheus counter: `tasks_produced_total`
- [ ] pprof HTTP server on `profiling_port`
- [ ] Prometheus HTTP server on `prometheus_port` at `/metrics`
- [ ] slog structured logging (level + format from config)

### Phase 5 — Consumer Service (75 min)
- [ ] Implement gRPC server `TaskService/SubmitTask`
- [ ] Rate limiter: `golang.org/x/time/rate` token bucket, configured `rate` + `burst`
- [ ] On receive: update DB state → `processing`
- [ ] Call `processTask(task)`: `time.Sleep(time.Duration(task.Value) * time.Millisecond)`
- [ ] Update DB state → `done`
- [ ] Prometheus metrics:
  - `tasks_processing_total` (counter)
  - `tasks_done_total` (counter)
  - `tasks_by_type` (counter vec, label `type`)
  - `tasks_value_sum_by_type` (counter vec, label `type`)
- [ ] Final log per task: task ID, type, value, running value-sum for that type
- [ ] pprof HTTP server
- [ ] Prometheus HTTP server

### Phase 6 — Metrics + Grafana (45 min)
- [ ] `prometheus.yml`: scrape both services every 5s
- [ ] Grafana `tasks.json` dashboard with 4 panels:
  1. **Tasks by state** — gauge/bar: `received`, `processing`, `done` (query DB via prometheus or use counters)
  2. **Service up/down** — stat panel: `up` metric from prometheus scrape targets
  3. **Sum of value per task type** — bar chart: `tasks_value_sum_by_type`
  4. **Tasks processed per task type** — bar chart: `tasks_by_type`
- [ ] Grafana datasource auto-provisioning YAML

### Phase 7 — Docker & Compose (30 min)
- [ ] `Dockerfile.producer`: multi-stage, `go build -ldflags="-s -w -X main.Version=$(VERSION)"`, distroless/scratch final image
- [ ] `Dockerfile.consumer`: same pattern
- [ ] `docker-compose.yml`: services: `postgres`, `producer`, `consumer`, `prometheus`, `grafana`
  - Health checks on postgres before producer/consumer start
  - Named volumes for postgres data, grafana data
  - Environment variable wiring for config
  - `GOGC` and `GOMEMLIMIT` env vars exposed and commented

### Phase 8 — Tests (45 min)
- [ ] `internal/config`: test loading from YAML, env override
- [ ] `internal/db`: integration tests against real postgres (use `testing.Short()` guard)
- [ ] Producer: test task generation logic, backlog gate
- [ ] Consumer: test rate limiter behavior, state transitions
- [ ] `Makefile test` target: `go test -cover ./... -coverprofile=coverage.out && go tool cover -html=coverage.out`

### Phase 9 — Build Flags + Profiling Docs (15 min)
- [ ] Makefile `build` target demonstrates:
  ```makefile
  go build -ldflags="-s -w -X main.Version=$(shell git describe --tags --always)" \
    -o bin/producer ./cmd/producer
  ```
- [ ] Add `README.md` section: how to capture flame graph:
  ```bash
  curl -o trace.out http://localhost:XXXX/debug/pprof/trace?seconds=5
  go tool trace trace.out
  # CPU profile → flame graph:
  go tool pprof -http=:8080 cpu.prof
  ```
- [ ] README section: GOGC vs GOMEMLIMIT tradeoff explanation

---

## What is Explicitly Out of Scope (time trade-off)

| Item | Reason skipped |
|---|---|
| PGO (profile-guided optimization) build | Requires profiling data collection first run; adds 30+ min for marginal demo value |
| GitHub Actions CI pipeline | Evaluation explicitly states pipelines not counted |
| Swagger/OpenAPI docs | Not needed for gRPC choice |
| Fuzz testing | Mentioned in helpful links but not in evaluation criteria |
| Full Grafana beautification | Evaluation explicitly states "beauty doesn't matter" |

---

## Key Design Notes for Interview

### Why goroutines / channels / mutexes:
- **Producer**: single goroutine per task via a `time.Ticker` — no channel needed because DB is the backlog buffer; rate is enforced by ticker interval
- **Consumer**: gRPC server uses goroutines per request automatically; value-sum accumulation per type uses a `sync.Mutex`-protected map (simple, low contention vs channel overhead)
- **Rate limiter**: `golang.org/x/time/rate` internally uses mutex; no custom sync needed

### GOGC vs GOMEMLIMIT:
- Lower `GOGC` (e.g., 50) → GC runs more often → lower peak memory, higher CPU cost
- `GOMEMLIMIT` is a hard ceiling — GC goes aggressive before OOM; useful in containerized environments where memory limits are set at the OS level
- For a high-throughput task processor: set `GOMEMLIMIT` to ~80% of container limit, keep `GOGC=100` (default), tune down only if heap grows unbounded

### Scalability bottlenecks:
- Single Postgres instance is the main bottleneck at scale → future: partition tasks table, read replicas for metrics queries
- gRPC is synchronous per task send — future: use gRPC streaming or a queue (RabbitMQ/Kafka) for true decoupling
- Consumer's sleep-based processing is I/O-bound → goroutine pool with `semaphore` pattern would increase throughput

---

## Estimated Time Budget

| Phase | Estimate |
|---|---|
| Scaffolding + proto | 45 min |
| Database (migrations + sqlc) | 45 min |
| Config + embed | 30 min |
| Producer service | 75 min |
| Consumer service | 75 min |
| Metrics + Grafana | 45 min |
| Docker + compose | 30 min |
| Tests | 45 min |
| Build flags + README | 15 min |
| **Total** | **~7h 25min** |
