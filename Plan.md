# Exchangely Plan

## Goal

Exchangely is a high-availability crypto historical-data service for curated EUR and USDT pairs.

Core stack:
- Go backend
- React frontend scaffold
- Kafka event bus
- TimescaleDB/PostgreSQL persistence
- Docker Compose local topology

## Current State

Implemented:
- single Go binary with API + planner + worker runtime
- DB-backed planner lease and DB-backed task lifecycle
- planner logs and publishes only newly inserted tasks
- stale running tasks are reclaimed by the worker poller after timeout
- pair-level advisory locking for worker exclusivity
- raw candle persistence
- hourly consolidation
- daily consolidation derived from hourly only
- realtime Kafka task and market-event flow
- structured backend logging via `log/slog`
- configurable CORS allowlist
- Swagger/OpenAPI file serving
- Docker Compose stack with Kafka KRaft, TimescaleDB, backend, frontend, and Kafka topic init

Ingestion sources:
- Binance Vision for historical archive-backed backfill
- Binance live API for recent USDT windows
- Kraken live API for EUR windows
- source cooldown handling for Binance and Kraken
- gap detection for historical hourly backfill
- source fallthrough when a supported source returns no candles

## Verified

Verified locally:
- `go test ./...`
- planner lifecycle and scheduling tests
- worker execution and idempotency-oriented tests
- Kafka admin and consumer tests
- source adapter tests for Binance, Binance Vision, Kraken, and registry behavior
- Compose smoke workflow via `./scripts/compose-smoke.sh`
- live Compose runtime logs now show planner leadership, worker task start/end, and source fetch activity
- stale `running` tasks were reclaimed and completed successfully in the live stack

Compose smoke assertions currently cover:
- backend health
- seeded assets and pairs
- planner leadership in API and DB lease table
- sync-status rows
- task enqueue, task claim, and task progress in DB
- deterministic `/historical` and `/ticker` reads from a seeded fixture
- Kafka topics
- Kafka consumer groups

## Runtime Notes

Locked-in architecture choices:
- PostgreSQL/Timescale is the source of truth for leases, sync state, and task state
- Kafka is transport, not coordination state
- daily candles are derived from hourly candles
- Compose owns Kafka topic creation; the backend no longer does app-level topic bootstrap

Operational notes:
- local backend rebuilds may require `DOCKER_DEFAULT_PLATFORM=linux/arm64` if the shell environment is polluted
- immediately after container recreation there can be a brief HTTP reset window before the backend is reachable

## Remaining Gaps

1. Backfill/source strategy still needs hardening.
   - free sources can still underfill or rate-limit requests
   - source selection is better, but still not final
   - live-source behavior still needs more explicit coverage for realtime windows

2. Realtime mode is still basic.
   - consolidation works, but live source strategy and 24h ticker behavior can be improved

3. Compose coverage is stronger, but not full failure-mode coverage.
   - current smoke path validates happy-path runtime state
   - restart/failure/recovery scenarios are still open

## Current Focus

Active workstream:
- improve runtime observability
- harden ingestion/backfill behavior
- keep extending verification only when it materially improves confidence

Next likely steps:
1. validate realtime task execution and market-event ingestion under Compose
2. keep hardening source behavior around rate limits, fallbacks, and status reporting
3. extend Compose coverage where it checks failure recovery, especially stale/retried task paths

## Deferred TODOs

- Evaluate Go Testcontainers for isolated per-test Kafka/Timescale integration tests.
  - keep Docker Compose as the main local stack and smoke/deployment-shape verification path
  - revisit Testcontainers only after the current Compose-backed verification path stops being the main bottleneck

## Read First

New agent starting points:
- `Plan.md`
- `docker-compose.yml`
- `backend/internal/app/app.go`
- `backend/internal/planner/runtime.go`
- `backend/internal/worker/backfill_executor.go`
- `backend/internal/ingest/registry/registry.go`
- `backend/internal/ingest/binancevision/client.go`
- `backend/internal/ingest/binance/client.go`
- `backend/internal/ingest/kraken/client.go`
