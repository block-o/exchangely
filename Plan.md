# Exchangely Plan

## Purpose

This file is the handoff ledger for continuing work across agents/sessions.
It tracks:
- what has already been implemented
- what is currently verified
- what is still open
- what should be done next
- environment and operational gotchas

Current branch: `master`

Last committed work in git:
- `b32fed4` Validate historical source coverage
- `3d2cfbe` Add Binance API cooldown handling
- `2c82df0` Scope Binance Vision to historical windows
- `d7d3012` Remove synthetic candle fallback
- `4b21203` Add Kafka topic bootstrap coverage

Note:
- The working tree may change between sessions; always check `git status --short`.
- Before switching agents, read `git status --short`.

## Product Goal

Exchangely is a highly available crypto historical-data service:
- Go backend
- React frontend scaffold
- Kafka event bus
- TimescaleDB persistence
- Docker Compose local stack
- focus on historical OHLCV for EUR and USDT quoted pairs

## Architecture Decisions Locked In

These decisions were already made and implemented enough that new work should build on them, not replace them:

1. Coordinator/Planner leadership is DB-backed, not Kafka-backed.
   - PostgreSQL/Timescale is the source of truth for leases and task state.

2. Kafka is used for task/event transport, but worker safety is DB-enforced.
   - pair-level Postgres advisory locks prevent concurrent writes on the same pair
   - task state is persisted in DB

3. One Go binary runs API + planner + worker depending on role config.

4. Daily candles are derived from hourly candles, not fetched independently.

5. Static archive sources are valid for backfill.
   - Binance Vision is now treated as a real backfill source

## What Is Implemented

### Backend Runtime

- single binary app bootstrap
- migrations on startup
- catalog seeding for supported assets/pairs
- DB-backed planner lease
- planner task generation
- DB-backed task enqueue/claim/complete/fail
- worker loop
- per-pair advisory lock
- raw candle persistence
- hourly consolidation
- daily consolidation from hourly only
- realtime Kafka task and market event plumbing
- DB-backed REST API

### Ingestion Sources

- Binance API adapter
  - cooldown after rate-limit responses
- Kraken API adapter
- Binance Vision archive adapter
  - monthly kline zip archives
  - daily archive fallback if monthly archive missing
  - timestamp normalization for Binance’s newer microsecond archives
  - only used for windows closed before the current UTC day
- Kraken improvements
  - dynamic `AssetPairs` resolution
  - cooldown after rate-limit responses
  - cooldown also applies when `AssetPairs` lookup is rate-limited
- registry improvements
  - empty source responses now fall through to the next supporting source
  - no-data conditions surface as task failures instead of silent success
- backfill coverage validation
  - historical hourly backfill now fails when a source response leaves gaps inside the requested window

### Frontend

- basic React/Vite scaffold is present
- API consumer scaffold exists
- not a focus yet

### Local Infra

- Docker Compose stack for:
  - backend
  - frontend
  - Kafka `apache/kafka:4.1.2`
  - Timescale `timescale/timescaledb:2.26.1-pg18`
- one-shot Kafka topic bootstrap service:
  - `kafka-init`

## Important Migrations Added Locally

These were added in the runtime stabilization work:
- `backend/migrations/000004_interval_sync_state.up.sql`
- `backend/migrations/000004_interval_sync_state.down.sql`
- `backend/migrations/000005_reconcile_sync_state.up.sql`
- `backend/migrations/000005_reconcile_sync_state.down.sql`

Purpose:
- split sync progress into hourly vs daily state
- reconcile bad interval state created by earlier scheduler behavior

## Key Runtime Fixes Already Done Locally

### Scheduler/Sync Fixes

- task IDs now include interval
- daily backfill is not scheduled until hourly backfill is complete
- sync status tracks hourly and daily progress separately
- inconsistent historical sync flags are reconciled on migration/read
- failed tasks are no longer retried immediately in a tight loop

### Kafka/Compose Fixes

- Kafka topics are created by Compose via `deploy/kafka/init-topics.sh`
- backend no longer hard-fails startup if app-level topic bootstrap gets `EOF`
- local stack is healthy even if Kafka admin API is noisy at app boot

### Docker/Platform Fixes

- backend image build works for `linux/arm64`
- backend Dockerfile now respects `TARGETARCH`

## What Has Been Verified

Verified locally in this workspace:

- `go test ./...` passes
- lifecycle integration tests now cover:
  - hourly backfill scheduling + execution + sync advancement
  - daily promotion + realtime eligibility
- planner runner tests now cover:
  - lease gating
  - missing sync-state seeding
  - enqueue + publish behavior
- Kafka consumer tests now cover:
  - invalid task payload handling
  - task handler invocation/error swallowing
  - market event decoding and forwarding
- Kafka admin tests now cover:
  - retry behavior
  - topic config normalization
  - broker fallback
  - already-exists tolerance
- live source tests now cover:
  - Binance cooldown after rate-limit responses
  - Kraken cooldown after rate-limit responses
- backfill executor tests now cover:
  - missing source registry
  - source coverage gaps
- Docker Compose stack starts
- backend health endpoint returns healthy status
- Kafka topics exist:
  - `exchangely.tasks`
  - `exchangely.market.ticks`
- system sync status endpoint works

Useful verification commands:

```sh
go test ./...
curl -sS http://127.0.0.1:8080/api/v1/health
curl -sS http://127.0.0.1:8080/api/v1/system/sync-status
docker compose ps
docker exec exchangely-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list
```

## Current Live Behavior

Last observed backend state:
- API health was `ok`
- Kafka health was `ok`
- Timescale health was `ok`
- hourly backfill had progressed to complete across tracked pairs
- planner moved into daily phase after scheduler fixes

## Known Gaps / Risks

1. Source coverage is still incomplete and tasks can fail when free remote sources do not provide reliable coverage.
   - synthetic fallback candles have been removed from the executor to protect data quality
   - source reliability and coverage still need to improve

2. Realtime ingestion is still basic.
   - more robust polling/streaming and fallback logic is still needed

3. There is not yet full Compose-level automated integration coverage.

4. Data-source strategy is still mixed between archive and live API.
   - source selection and rate-limit handling still need to improve over time

## Current Uncommitted Files

Refresh this section before handoff with `git status --short`.

Important:
- `backend/server` is a local artifact and should not be committed.

## Operational Gotchas

### Docker platform

The shell session previously inherited:
- `DOCKER_DEFAULT_PLATFORM=linux/amd64`

That caused Docker/Buildx to pass `TARGETARCH=amd64` into the Go build.

For backend rebuilds in this environment, use:

```sh
DOCKER_DEFAULT_PLATFORM=linux/arm64 docker compose up --build -d backend
```

For direct infra-only commands where platform auto-selection is safer, prefer:

```sh
env -u DOCKER_DEFAULT_PLATFORM docker compose up -d kafka-init backend
```

### Restart window

Immediately after backend container recreation, `curl` to `:8080` may briefly return:
- `Recv failure: Connection reset by peer`

This was transient during startup; retry after a few seconds.

## Recommended Next Steps

### Next priority

1. Continue hardening backfill sources.
   - reduce task failures caused by public-source gaps
   - prefer archive-backed coverage where possible
   - add better source selection and retry/cooldown behavior

2. Add fuller Compose-level integration coverage.
   - planner lease under the Compose stack
   - task enqueue/consume against Kafka + DB together
   - startup/bootstrap checks around topic init and API readiness

2. Improve backfill source strategy.
   - prefer archive sources first for historical windows
   - reduce live API usage during heavy backfill
   - add source-level throttling / cooldown beyond Kraken
   - detect and classify partial historical coverage earlier when provider APIs underfill a request

3. Improve realtime mode.
   - better source selection
   - more deterministic 24h ticker variation
   - clearer boundary between live ingestion and consolidation

### Good candidate tasks for parallel agents

- Agent 1: commit hygiene + repo cleanup
  - review uncommitted files
  - remove/ignore local artifacts like `backend/server`
  - produce a clean checkpoint commit

- Agent 2: integration tests
  - Docker Compose startup checks
  - planner/worker/Kafka flow validation

- Agent 3: integration tests
  - hourly to daily materialization checks
  - idempotency and retry coverage

- Agent 4: source strategy hardening
  - backfill source priority rules
  - rate-limit handling and source cooldown policy

## Files Another Agent Should Read First

- `Plan.md`
- `docker-compose.yml`
- `backend/internal/app/app.go`
- `backend/internal/planner/scheduler.go`
- `backend/internal/storage/postgres/sync_repository.go`
- `backend/internal/worker/backfill_executor.go`
- `backend/internal/ingest/binancevision/client.go`
- `backend/internal/ingest/kraken/client.go`
- `backend/migrations/000004_interval_sync_state.up.sql`
- `backend/migrations/000005_reconcile_sync_state.up.sql`

## Short Handoff Summary

The system is past the pure scaffold stage.
The main local stack works.
The planner/worker flow is DB-backed and running.
Static Binance Vision backfill is integrated.
The biggest remaining work is deeper integration coverage, data-quality hardening, and reducing fallback/generated candles.
