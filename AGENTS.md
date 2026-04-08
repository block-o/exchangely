# Exchangely Agent Guide

This file is the primary operating guide for future agent work in this repository.

## Document Roles

- `AGENTS.md`: durable project-specific execution guide. Future work should start here.
- `PLAN.md`: shared scratchpad. Keep it current, lightweight, and disposable. Use it for active tasks, temporary notes, checkpoints, and decisions in progress. Do not turn it into a permanent spec.
- `PROJECT.md`: original project brief and historical scope. Keep it as source context, but do not treat it as the day-to-day operating contract once this file exists.
- `README.md`: high-level overview and quick-start entrypoint for humans.

If these documents disagree:

1. Current code and tests define actual behavior.
2. `AGENTS.md` defines how agents should work in this repo.
3. `PLAN.md` reflects current intent and active priorities, but it may lag and must be refreshed during work.
4. `PROJECT.md` explains the original ask and enduring product direction.
5. `README.md` is overview material, not the detailed implementation contract.

## Project Snapshot

Exchangely is an educational event-driven crypto market data platform focused on historical OHLCV coverage for curated crypto/fiat and crypto/stablecoin pairs.

Current implementation shape:

- Backend: Go service with a single runtime that can enable API, planner, and worker roles through `BACKEND_ROLE`.
- Frontend: React + Vite dashboard consuming REST plus SSE streams.
- Persistence: TimescaleDB/PostgreSQL is the authoritative source of truth.
- Transport: Kafka distributes task and market events, but it is not the coordination authority.
- Local topology: Docker Compose runs TimescaleDB, Kafka, topic init, backend, and frontend.

Important current sources/providers:

- Historical/realtime adapters currently include Binance, Binance Vision, Kraken, CryptoDataDownload, and CoinGecko.
- Default quote assets are `EUR` and `USD`.

## Non-Negotiable System Invariants

These rules are easy to break accidentally. Preserve them unless the user explicitly changes the architecture.

- PostgreSQL/Timescale owns system state. Do not move scheduling truth, sync state, or coordination responsibility into Kafka.
- Planner leadership is lease-based and DB-backed. Keep leadership and scheduling resilient to multiple instances.
- Workers must never mutate the same trading pair concurrently. Pair-level locking is a hard requirement.
- Historical backfill walks backwards from yesterday toward the past. There is no fixed start date; the system keeps walking back until providers return no data. The per-tick limit (`BACKEND_PLANNER_BACKFILL_BATCH_PERCENT`) caps how many backfill tasks are emitted per planner cycle.
- A daily backfill probe task extends each pair one hour further into the past, ensuring new providers or newly available upstream data is discovered even after regular backfill stops finding data.
- `1h` is the preferred canonical historical resolution. Historical fetch granularity must never be coarser than `1d`.
- Month/year rollups, if added, must be derived from canonical stored candles, not provider-native month archives.
- Realtime tasks use a stable ID per pair (`live_ticker:{PAIR}:realtime`). The Enqueue logic guarantees at most one pending/running realtime task per pair. Do not add poll-window timestamps to realtime task IDs.
- Prefer SSE for live UI updates when a stream already exists. Do not replace stream-driven flows with aggressive polling.
- The backend is intentionally one binary with role gating. Preserve that unless a deliberate architecture change is requested.
- Keep the project educational and local-first. Do not quietly harden it toward production assumptions without discussing the tradeoff.

## Current Runtime Model

Key implementation files:

- `backend/internal/app/app.go`: application wiring, enabled sources, runtime roles.
- `backend/internal/planner/runtime.go`: leader lease renewal and task scheduling loop.
- `backend/internal/planner/scheduler.go`: task generation logic (backfill, realtime, probes, cleanup).
- `backend/internal/worker/processor.go`: worker claim/execute path.
- `backend/internal/httpapi/router/router.go`: REST and SSE endpoints.
- `backend/internal/storage/postgres/*`: persistence, task state, sync state, leases, locks.
- `frontend/src/components/SystemPanel.tsx`: operations dashboard, warnings, task/system views.
- `docker-compose.yml`: local topology and default environment wiring.

Current task types:

- `historical_backfill` — walks backwards from yesterday; also used for daily probes.
- `live_ticker` — stable ID per pair; at most one pending/running per pair at any time.
- `integrity_check`
- `consolidation`
- `task_cleanup`
- `news_fetch`
- `gap_validation`

Current notable API surface:

- `GET /api/v1/health`
- `GET /api/v1/assets`
- `GET /api/v1/pairs`
- `GET /api/v1/historical/{pair}`
- `GET /api/v1/ticker/{pair}`
- `GET /api/v1/tickers`
- `GET /api/v1/tickers/stream`
- `GET /api/v1/system/sync-status`
- `GET /api/v1/system/tasks`
- `GET /api/v1/system/tasks/stream`
- `GET /api/v1/system/version`
- `GET /api/v1/news`
- `GET /api/v1/news/stream`

When changing API behavior, update `docs/openapi/openapi.yaml` if the contract changes materially.

## Active Priorities From PLAN

`PLAN.md` is the active working memory. Read it before major work and update it when you materially change direction.

At the time this guide was written, the main near-term priorities are:

- Finish the health/data integrity validator flow by persisting findings and exposing them cleanly through the API/UI.
- Continue provider expansion, with Yahoo Finance still pending.
- Split ingestion responsibilities more cleanly into backfill vs realtime paths.
- Add month/year rollups from canonical candles.
- Expand into fiat/forex pairs later.
- Improve rate-limit handling, source balancing, and cache/circuit-breaker behavior.
- Upgrade chart rendering quality in the frontend.
- Add stronger integration coverage, likely with Testcontainers.

Completed work already reflected in the repo includes:

- Single Go binary runtime with planner/worker/API roles.
- Kafka task flow and Timescale-backed sync/task lifecycle.
- SSE-driven ticker and task updates for the frontend.
- Operations dashboard with active warnings.
- CryptoDataDownload and CoinGecko integrations.
- Real-time news ingestion from RSS sources (CoinDesk, Cointelegraph, TheBlock).
- Compose-based smoke/e2e coverage.
- Backwards backfill strategy (yesterday → past) with no fixed start date.
- Realtime dedup: at most one `live_ticker` task per pair in the queue.
- Daily backfill probe: one task per pair per day to extend coverage into the past.
- Ticker query uses `FULL OUTER JOIN` so pairs appear as soon as raw candles land (before hourly consolidation).
- `BACKEND_DEFAULT_BACKFILL_START` removed; `backfill_start_at` column dropped from `pairs` table.

## How To Use PLAN.md

Use `PLAN.md` as a scratchpad, not as permanent documentation.

Good uses:

- Active todo lists
- Investigation notes
- Decision checkpoints
- Work-in-progress rollout plans
- Temporary coordination between agents/models

Bad uses:

- Stable architecture rules
- Long-term onboarding instructions
- Canonical API or domain behavior
- Permanent project scope

When you finish meaningful work:

- Mark completed items.
- Add any newly discovered follow-up tasks.
- Remove stale or misleading notes.
- Keep the file readable by the next agent.

## Repo-Specific Working Rules

Before starting substantial changes:

- Read this file.
- Read the relevant section of `PLAN.md`.
- Inspect the actual implementation files you will touch.

When changing backend behavior:

- Respect the planner/worker separation even though they share one binary.
- Keep repository interfaces and domain boundaries coherent; avoid pushing transport or storage concerns deep into domain objects.
- Add or update tests close to the affected package.
- Preserve structured logging with `slog`.
- If you add a provider, wire support through config, source registration, and tests.

When changing frontend behavior:

- Preserve the existing dashboard-first UX and operations visibility.
- Prefer consuming existing SSE streams over introducing new polling loops.
- Keep test coverage in Vitest for non-trivial view logic.
- Maintain the current intentional UI quality; do not downgrade the interface into generic placeholder styling.

When changing configuration:

- Update `backend/internal/config/config.go`.
- Update `docker-compose.yml` if the local stack should expose the setting.
- Update example config files when relevant.
- Mention new operational knobs in `README.md` or `AGENTS.md` if they matter to future work.

When changing schema or persistence behavior:

- Add a migration in `backend/migrations`.
- Keep migrations forward/backward paired.
- Validate the affected repositories and any runtime assumptions tied to sync/task state.

## Validation Commands

Use the repo's existing commands instead of inventing one-off workflows.

- `make backend-fmt`
- `make backend-fmt-fix`
- `make backend-vet`
- `make backend-lint`
- `make backend-build`
- `make backend-test`
- `make frontend-deps`
- `make frontend-typecheck`
- `make frontend-build`
- `make frontend-test`
- `make check`
- `make test`
- `make e2e`
- `docker compose up --build`
- `docker compose down -v`

E2E notes:

- `make e2e` uses `scripts/compose-smoke.sh`.
- The smoke flow validates the live Compose stack plus Kafka topics/consumer groups.

## Useful File Map

- `backend/cmd/server/main.go`: backend entrypoint.
- `backend/cmd/migrate/main.go`: migration entrypoint.
- `backend/internal/service/*`: application services used by the HTTP layer.
- `backend/internal/ingest/*`: market data providers and registry.
- `backend/internal/messaging/kafka/*`: Kafka producers, consumers, health checks.
- `backend/tests/integration/*`: integration coverage.
- `backend/tests/e2e/*`: compose-backed end-to-end tests.
- `frontend/src/api/*`: frontend API clients.
- `frontend/src/pages/*`: top-level screens.
- `frontend/src/components/*`: UI building blocks.
- `docs/openapi/openapi.yaml`: API contract documentation.
- `docs/ui/market_dashboard.png`: current dashboard reference image.

## Guidance On PROJECT.md

Do not copy `PROJECT.md` wholesale into ongoing work. Its value is the original scope and product intent:

- crypto market history availability
- event-driven architecture
- planner/worker/coordinator mindset
- historical + realtime data flows
- Dockerized local stack

Those durable ideas are already distilled here. Keep `PROJECT.md` as historical context unless the user explicitly asks to retire or rewrite it.

## Practical Default For Future Agents

If you are unsure where to put new information:

- Put stable repo-specific rules in `AGENTS.md`.
- Put active work tracking and temporary notes in `PLAN.md`.
- Put user-facing setup or overview material in `README.md`.
- Leave `PROJECT.md` as the original brief unless explicitly asked to migrate or replace it.
