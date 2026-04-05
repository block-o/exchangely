# Exchangely Plan

## Goal
Exchangely is a high-availability crypto historical-data service for curated Fiat and Stablecoin pairs.

## Core Stack
- Go backend + React frontend
- Kafka event bus + TimescaleDB/PostgreSQL persistence
- Docker Compose local topology

## Completed Milestones
- **Backend**: Single Go binary (API + planner + worker), DB-backed task & lease lifecycle, 24h/1h consolidated candles, realtime Kafka task flow, structured logging (`slog`).
- **Frontend**: Premium interface, SSE-driven realtime Market updates (bypassing heavy polling), Vitest testing stack, dynamic `__APP_VERSION__` injection, SVG settings gear.
- **Data Querying**: Zero-polling SSE updates at `/api/v1/tickers/stream`; optimized 3-CTE SQL endpoint for 24h variation stats.
- **Ticker Read Model**: `/api/v1/ticker/{pair}` and `/api/v1/tickers` now prefer the newest persisted realtime raw sample for price/last-update metadata instead of the current hour aggregate, while still exposing 24h stats.
- **Realtime Cutover**: live ticker scheduling now starts before hourly historical backfill completes; backfill stops at the per-pair live cutover hour and no longer shares the historical sync cursor with realtime execution.
- **Realtime Streaming**: Kafka market-event consumption now batches candles by pair/hour before ingesting them so SSE ticker pushes track realtime poll batches instead of noisy per-candle fanout.
- **DevOps**: GitHub Actions CI workflow, strict Node.js 24 + Go 1.24 build limits via Docker configurations.
- **Operations Dashboard**: SSE-powered `SystemPanel` accurately tracking and predicting future backfill and realtime poll gaps.
- **Documentation**: Substantial inline godoc standard for SSE/SQL methods mapping.
- **Task Runtime Extensions**: Scheduler now emits `integrity_check` tasks for caught-up pairs and workers execute cross-source validation passes.

## Runtime Notes
- **Data Backbone**: PostgreSQL/Timescale is the authoritative source for states. Kafka acts firmly as a transport mechanism, not a state-store coordination layer.
- **Container Strategy**: `npm ci --legacy-peer-deps` within a multi-stage Alpine Docker restricts OS leaks.

## Roadmap & Missing Features
- [ ] Finish the **health validator scheduled task**: task emission and worker execution already exist; configurable thresholds and failed-task surfacing are now wired, but findings still need persistence and dedicated API exposure.
- [x] Add Active Warnings area on top of the task status panel so current platform risks such as degraded health, pending backfills, and recent task failures are visible without digging through task history.
- [x] Add **CryptoDataDownload** as a dedicated backfill provider for historical hourly/daily CSV fallback alongside the existing exchange adapters.
- [x] Add **CoinGecko** as an additional realtime ticker provider, using live market-chart samples for supported realtime quote windows.
- [ ] Add **Yahoo Finance (Yfinance)** as a ticker provider.
- [ ] **Refactored Ingest**: Split the `ingest` module cleanly into two distinct submodules: `backfill` and `realtime`.
- [ ] Add scheduled **month/year rollup buckets** derived from hourly/daily canonical candles rather than provider-native month archives.
- [ ] **Fiat/Forex Pairs**: Begin tracking currency-to-currency pairs (e.g., EURUSD, EURGBP).
- [ ] Implement robust source load-balancing, rate-limit back-off (circuit breakers for `429 Too Many Requests`), and caching.
- [ ] Upgrade frontend sparklines to true SVG/Canvas line charts.
- [ ] Integrate Go Testcontainers for hard failure-mode testing against PostgreSQL/Kafka.

## Current Focus
**Data Integrity Validator & New Provider Integration**
The immediate phase aims at ensuring that fetched records match securely across exchanges prior to write, followed quickly by introducing new data sources (CryptoDataDownload, CoinGecko, Yfinance) and splitting up the ingestion mechanics into dedicated paths.

Operational rule updates:
- Historical source fetch granularity must never be coarser than **1 day**, and **1 hour** remains the preferred canonical backfill resolution.
- Provider-native monthly archives should not drive historical sweeps; larger buckets such as month/year must be built later by scheduled consolidation from canonical stored candles.

## Deferred TODOs
- Evaluate Go Testcontainers for isolated per-test Kafka/Timescale integration tests (Revisit only once the core feature stability allows it).

## Read First
Key system components to navigate:
- `docker-compose.yml`
- `backend/internal/app/app.go`
- `backend/internal/planner/runtime.go`
- `backend/internal/worker/backfill_executor.go`
- `frontend/src/components/SystemPanel.tsx`
