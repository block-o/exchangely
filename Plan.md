# Exchangely Plan

## Goal

Exchangely is a high-availability crypto historical-data service for curated EUR and USDT pairs.

Core stack:
- Go backend
- React frontend
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
- README architecture diagram and updated runtime notes
- initial codebase documentation/test coverage for operational behavior
- GitHub Actions CI workflow (linting, test suite, and Compose e2e test validation)
- premium React frontend layout matching a CoinMarketCap-style Market dashboard and System Operations dashboard
- realtime frontend market updates powered by Server-Sent Events (SSE) and an in-memory Go EventBus
- frontend testing stack using Vitest, React Testing Library, and Happy-DOM
- global `/api/v1/tickers` endpoint with optimized 3-CTE SQL query (latest price, 24h variation, 24h high/low)
- `/api/v1/tickers/stream` SSE endpoint for zero-polling realtime browser updates
- RealtimeIngestService wired to MarketNotifier so Kafka-driven live updates signal the SSE stream
- comprehensive inline documentation on all critical methods, interfaces, and SQL queries
- browser-timezone-aware date rendering with Intl API; footer shows timezone context (e.g. "Europe/Madrid")
- polished settings gear icon: borderless inline SVG with hover glow, matching nav pill design language
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
- Compose smoke now covers Kafka-driven realtime market-event ingestion through to API-visible hourly candles

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

Architectural Decisions:
- **Frontend Realtime Webhooks:** The UI avoids inefficient long-polling logic. Instead, we use Server-Sent Events (SSE) at `/api/v1/tickers/stream`. The Golang backend utilizes an in-memory PubSub Channel `EventBus` that the `worker` directly signals when a database upsert finishes, triggering the streaming API to broadcast diff payloads downstream instantly.
- **Frontend Ticker Optimization:** We abandoned the "multi-fetch map-reduce" loop and replaced it with a single, highly optimized `DISTINCT ON` SQL query via the global `/api/v1/tickers` endpoint, stripping away irrelevant columns (like Source) which reduces both server compute and browser memory payload.
- **Frontend Container Dependency Management:** Forced `npm ci` within the multi-stage Alpine Docker build explicitly to bypass host-OS specific engine leakages (e.g. macOS Node bindings bleeding into the Linux container state).
- **Dual Notification Paths:** Both `BackfillExecutor` (historical/daily tasks) and `RealtimeIngestService` (Kafka consumer) independently signal the SSE EventBus after successful Postgres writes. This ensures the UI is notified regardless of whether data arrives via scheduled backfill or live Kafka streaming.
- **24h Analytics in SQL:** The Tickers query uses three CTEs (`latest`, `past`, `window_24h`) to compute price, 24h variation, 24h high, and 24h low in a single database round-trip. This avoids N+1 queries and keeps the frontend payload minimal.

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

4. Frontend polish is ongoing.
   - the sparkline charts use basic bar rendering; a proper SVG line chart would look more polished
   - accessibility and responsive breakpoints need attention

5. Code documentation is now comprehensive for critical paths.
   - all SSE, EventBus, and SQL query methods have detailed doc comments
   - future work should maintain this standard as new methods are added

## Current Focus

Active workstream:
- improve runtime observability
- harden ingestion/backfill behavior
- plan the first real frontend experience
- improve codebase documentation around critical runtime paths
- keep extending verification only when it materially improves confidence

Next likely steps:
1. keep hardening source behavior around rate limits, fallbacks, and status reporting
2. extend Compose coverage where it checks failure recovery, especially stale/retried task paths
3. continue adding documentation to critical backend flows:
   - scheduler and task generation
   - registry source selection and fallback rules
   - worker execution lifecycle and realtime ingest path

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
