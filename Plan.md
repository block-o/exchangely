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
- **DevOps**: GitHub Actions CI workflow, strict Node.js 24 + Go 1.24 build limits via Docker configurations.
- **Operations Dashboard**: SSE-powered `SystemPanel` accurately tracking and predicting future backfill and realtime poll gaps.
- **Documentation**: Substantial inline godoc standard for SSE/SQL methods mapping.

## Runtime Notes
- **Data Backbone**: PostgreSQL/Timescale is the authoritative source for states. Kafka acts firmly as a transport mechanism, not a state-store coordination layer.
- **Container Strategy**: `npm ci --legacy-peer-deps` within a multi-stage Alpine Docker restricts OS leaks. 

## Roadmap & Missing Features
- [ ] Create a **health validator scheduled task** to cross-examine data sanity and consolidation accuracy across multiple providers (e.g. Binance vs Kraken divergence logs & gap detection).
- [ ] Add **CryptoDataDownload** as a dedicated backfill provider.
- [ ] Add **CoinGecko** as an additional realtime ticker provider.
- [ ] Add **Yahoo Finance (Yfinance)** as a ticker provider.
- [ ] **Refactored Ingest**: Split the `ingest` module cleanly into two distinct submodules: `backfill` and `realtime`.
- [ ] **Fiat/Forex Pairs**: Begin tracking currency-to-currency pairs (e.g., EURUSD, EURGBP).
- [ ] Implement robust source load-balancing, rate-limit back-off (circuit breakers for `429 Too Many Requests`), and caching.
- [ ] Upgrade frontend sparklines to true SVG/Canvas line charts.
- [ ] Integrate Go Testcontainers for hard failure-mode testing against PostgreSQL/Kafka.

## Current Focus
**Data Integrity Validator & New Provider Integration**
The immediate phase aims at ensuring that fetched records match securely across exchanges prior to write, followed quickly by introducing new data sources (CryptoDataDownload, CoinGecko, Yfinance) and splitting up the ingestion mechanics into dedicated paths.

## Deferred TODOs
- Evaluate Go Testcontainers for isolated per-test Kafka/Timescale integration tests (Revisit only once the core feature stability allows it).

## Read First
Key system components to navigate:
- `docker-compose.yml`
- `backend/internal/app/app.go`
- `backend/internal/planner/runtime.go`
- `backend/internal/worker/backfill_executor.go`
- `frontend/src/components/SystemPanel.tsx`
