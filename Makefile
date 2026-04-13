VERSION  := $(shell git describe --tags --always 2>/dev/null || echo "dev")
LDFLAGS  := -ldflags="-s -w -X main.Version=$(VERSION)"

DB_URL   ?= postgres://postgres:postgres@localhost:5432/tasks?sslmode=disable

PRODUCER_PPROF_PORT ?= 6060
CONSUMER_PPROF_PORT ?= 6061

PPROF_SECONDS ?= 30

.PHONY: all proto sqlc migrate-up migrate-down build build-producer build-consumer \
        pgo-producer pgo-consumer test test-short docker-up docker-down clean \
        profile-cpu-producer profile-cpu-consumer \
        profile-heap-producer profile-heap-consumer \
        profile-trace-producer profile-trace-consumer \
        gogc-baseline gogc-aggressive gogc-compare-heap \
        gogc-cpu-baseline gogc-cpu-aggressive gogc-compare-cpu \
        gomemlimit-baseline gomemlimit-constrained gomemlimit-compare-heap \
        gomemlimit-cpu-baseline gomemlimit-cpu-constrained gomemlimit-compare-cpu

all: proto sqlc build

# Code generation

proto:
	PATH="$$HOME/go/bin:$$PATH" protoc \
		--go_out=internal/proto/gen --go_opt=paths=source_relative \
		--go-grpc_out=internal/proto/gen --go-grpc_opt=paths=source_relative \
		-I internal/proto \
		internal/proto/task.proto

sqlc:
	PATH="$$HOME/go/bin:$$PATH" sqlc generate -f internal/db/sqlc.yaml

# Database migration

migrate-up:
	PATH="$$HOME/go/bin:$$PATH" migrate -path migrations -database "$(DB_URL)" up

migrate-down:
	PATH="$$HOME/go/bin:$$PATH" migrate -path migrations -database "$(DB_URL)" down 1

# Build

build: build-producer build-consumer

build-producer:
	go build $(LDFLAGS) -o bin/producer ./cmd/producer

build-consumer:
	go build $(LDFLAGS) -o bin/consumer ./cmd/consumer

# Profiling

# CPU flame graph — collects PPROF_SECONDS seconds then opens browser
profile-cpu-producer:
	curl -sfo cpu-producer.prof "http://localhost:$(PRODUCER_PPROF_PORT)/debug/pprof/profile?seconds=$(PPROF_SECONDS)"
	go tool pprof -http=:8080 cpu-producer.prof

profile-cpu-consumer:
	curl -sfo cpu-consumer.prof "http://localhost:$(CONSUMER_PPROF_PORT)/debug/pprof/profile?seconds=$(PPROF_SECONDS)"
	go tool pprof -http=:8081 cpu-consumer.prof

# Heap memory graph — snapshot of live heap then opens browser
profile-heap-producer:
	curl -sfo heap-producer.prof "http://localhost:$(PRODUCER_PPROF_PORT)/debug/pprof/heap"
	go tool pprof -http=:8082 heap-producer.prof

profile-heap-consumer:
	curl -sfo heap-consumer.prof "http://localhost:$(CONSUMER_PPROF_PORT)/debug/pprof/heap"
	go tool pprof -http=:8083 heap-consumer.prof

# Goroutine trace — timeline of goroutine scheduling and GC events
profile-trace-producer:
	curl -sfo trace-producer.out "http://localhost:$(PRODUCER_PPROF_PORT)/debug/pprof/trace?seconds=5"
	go tool trace trace-producer.out

profile-trace-consumer:
	curl -sfo trace-consumer.out "http://localhost:$(CONSUMER_PPROF_PORT)/debug/pprof/trace?seconds=5"
	go tool trace trace-consumer.out

# GOGC / GOMEMLIMIT comparison

# snapshot heap at default settings (GOGC=100)
gogc-baseline:
	@echo ">>> Collecting baseline heap (GOGC=100)..."
	curl -sfo heap-baseline.prof "http://localhost:$(CONSUMER_PPROF_PORT)/debug/pprof/heap"
	@echo ">>> Baseline saved to heap-baseline.prof"
	go tool pprof -http=:8084 heap-baseline.prof

# restart consumer with aggressive GC (GOGC=5), then snapshot heap
gogc-aggressive:
	@echo ">>> Restarting consumer with GOGC=20..."
	GOGC=5 docker compose up -d --no-deps --force-recreate consumer
	@echo ">>> Waiting 10s for consumer to stabilise..."
	sleep 10
	curl -sfo heap-aggressive.prof "http://localhost:$(CONSUMER_PPROF_PORT)/debug/pprof/heap"
	@echo ">>> Aggressive GC heap saved to heap-aggressive.prof"
	go tool pprof -http=:8085 heap-aggressive.prof

# open both heap profiles side by side (run after gogc-baseline and gogc-aggressive)
gogc-compare-heap:
	sh -c 'go tool pprof -http=:8084 heap-baseline.prof & go tool pprof -http=:8085 heap-aggressive.prof'

# collect CPU profile at default GOGC=100 (run while consumer is at baseline)
gogc-cpu-baseline:
	@echo ">>> Collecting CPU baseline (GOGC=100) for $(PPROF_SECONDS)s..."
	curl -sfo cpu-baseline.prof "http://localhost:$(CONSUMER_PPROF_PORT)/debug/pprof/profile?seconds=$(PPROF_SECONDS)"
	@echo ">>> CPU baseline saved to cpu-baseline.prof"

# restart consumer with GOGC=5, then collect CPU profile
gogc-cpu-aggressive:
	@echo ">>> Restarting consumer with GOGC=5..."
	GOGC=5 docker compose up -d --no-deps --force-recreate consumer
	@echo ">>> Waiting 10s for consumer to stabilise..."
	sleep 10
	@echo ">>> Collecting CPU aggressive (GOGC=5) for $(PPROF_SECONDS)s..."
	curl -sfo cpu-aggressive.prof "http://localhost:$(CONSUMER_PPROF_PORT)/debug/pprof/profile?seconds=$(PPROF_SECONDS)"
	@echo ">>> CPU aggressive saved to cpu-aggressive.prof"

# open both CPU profiles side by side (run after gogc-cpu-baseline and gogc-cpu-aggressive)
gogc-compare-cpu:
	sh -c 'go tool pprof -http=:8086 cpu-baseline.prof & go tool pprof -http=:8087 cpu-aggressive.prof'

# GOMEMLIMIT comparison

# snapshot heap at default GOMEMLIMIT (128MiB)
gomemlimit-baseline:
	@echo ">>> Collecting baseline heap (GOMEMLIMIT=128MiB)..."
	curl -sfo heap-memlimit-baseline.prof "http://localhost:$(CONSUMER_PPROF_PORT)/debug/pprof/heap"
	@echo ">>> Baseline saved to heap-memlimit-baseline.prof"

# restart consumer with tight GOMEMLIMIT (32MiB), then snapshot heap
gomemlimit-constrained:
	@echo ">>> Restarting consumer with GOMEMLIMIT=32MiB..."
	GOMEMLIMIT=32MiB docker compose up -d --no-deps --force-recreate consumer
	@echo ">>> Waiting 10s for consumer to stabilise..."
	sleep 10
	curl -sfo heap-memlimit-constrained.prof "http://localhost:$(CONSUMER_PPROF_PORT)/debug/pprof/heap"
	@echo ">>> Constrained heap saved to heap-memlimit-constrained.prof"

# open both heap profiles side by side
gomemlimit-compare-heap:
	sh -c 'go tool pprof -http=:8088 heap-memlimit-baseline.prof & go tool pprof -http=:8089 heap-memlimit-constrained.prof'

# collect CPU profile at default GOMEMLIMIT (run while consumer is at baseline)
gomemlimit-cpu-baseline:
	@echo ">>> Collecting CPU baseline (GOMEMLIMIT=128MiB) for $(PPROF_SECONDS)s..."
	curl -sfo cpu-memlimit-baseline.prof "http://localhost:$(CONSUMER_PPROF_PORT)/debug/pprof/profile?seconds=$(PPROF_SECONDS)"
	@echo ">>> CPU baseline saved to cpu-memlimit-baseline.prof"

# restart consumer with tight GOMEMLIMIT, then collect CPU profile
gomemlimit-cpu-constrained:
	@echo ">>> Restarting consumer with GOMEMLIMIT=32MiB..."
	GOMEMLIMIT=32MiB docker compose up -d --no-deps --force-recreate consumer
	@echo ">>> Waiting 10s for consumer to stabilise..."
	sleep 10
	@echo ">>> Collecting CPU constrained (GOMEMLIMIT=32MiB) for $(PPROF_SECONDS)s..."
	curl -sfo cpu-memlimit-constrained.prof "http://localhost:$(CONSUMER_PPROF_PORT)/debug/pprof/profile?seconds=$(PPROF_SECONDS)"
	@echo ">>> CPU constrained saved to cpu-memlimit-constrained.prof"

# open both CPU profiles side by side
gomemlimit-compare-cpu:
	sh -c 'go tool pprof -http=:8090 cpu-memlimit-baseline.prof & go tool pprof -http=:8091 cpu-memlimit-constrained.prof'

# PGO: collect CPU profile from a live producer, then rebuild with it
pgo-producer:
	curl -sfo default.pgo "http://localhost:$(PRODUCER_PPROF_PORT)/debug/pprof/profile?seconds=$(PPROF_SECONDS)"
	go build -pgo=default.pgo $(LDFLAGS) -o bin/producer-pgo ./cmd/producer
	@echo "PGO binary: bin/producer-pgo"

# PGO: collect CPU profile from a live consumer, then rebuild with it
pgo-consumer:
	curl -sfo default.pgo "http://localhost:$(CONSUMER_PPROF_PORT)/debug/pprof/profile?seconds=$(PPROF_SECONDS)"
	go build -pgo=default.pgo $(LDFLAGS) -o bin/consumer-pgo ./cmd/consumer
	@echo "PGO binary: bin/consumer-pgo"

# Tests

test:
	go test -cover ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-short:
	go test -short -cover ./...

# Docker

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down -v

# Cleanup

clean:
	rm -rf bin/ coverage.out coverage.html default.pgo bin/producer-pgo bin/consumer-pgo \
	       cpu-producer.prof cpu-consumer.prof \
	       heap-producer.prof heap-consumer.prof \
	       heap-baseline.prof heap-aggressive.prof \
	       cpu-baseline.prof cpu-aggressive.prof \
	       heap-memlimit-baseline.prof heap-memlimit-constrained.prof \
	       cpu-memlimit-baseline.prof cpu-memlimit-constrained.prof \
	       trace-producer.out trace-consumer.out
