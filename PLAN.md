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
- **Data Consistency**: Implemented advisory pair-level locking in PostgreSQL during worker execution to prevent concurrent mutations of the same trading pair and ensure strictly sequential candle processing.
- **Scheduled Maintenance**: Added `task_cleanup` executor for automatic, scheduled pruning of completed and failed task logs to maintain bounded database growth.
- **Operational Tuning**: Made task log retention configurable through `BACKEND_TASK_RETENTION_PERIOD` and `BACKEND_TASK_MAX_LOG_COUNT`, supporting both duration-based and volume-based (count) pruning.
- **Operations Visibility**: Restored and improved backend-side filtering and pagination support for the operations dashboard, ensuring reliable visibility into large task logs.
- **Performance Optimization**: Implemented a multi-layered caching system for ticker endpoints (per-ticker invalidation + time-based global cache) to significantly reduce database load during high concurrency.
- **Testing Infrastructure**: Added a dedicated Go-based load testing suite (`make load-test`) integrated into CI, ensuring performance stability for ticker read models under heavy request volume.

## Roadmap & Missing Features
- [x] Add Active Warnings area on top of the task status panel so current platform risks such as degraded health, pending backfills, and recent task failures are visible without digging through task history.
- [x] Add **CryptoDataDownload** as a dedicated backfill provider for historical hourly/daily CSV fallback alongside the existing exchange adapters.
- [x] Add **CoinGecko** as an additional realtime ticker provider, using live market-chart samples for supported realtime quote windows.
- [x] **Refactored Ingest**: Split the `ingest` module cleanly into two distinct submodules: `backfill` and `realtime`.
- [/] **Extend Market view**: Circulating supply (from realtime) and 24h variation stats are implemented in the read model; still need to calculate 1h% and 7d% change, 24h volume variation, and expose these new fields in the Dashboard UI.
- [ ] Add Chainlink for realtime historical data providers. Review if we could call the smartcontract for free for each coin ensuring we have the most accurate data possible for free.
- [/] Implement a **News feed**: Add a recent news feed implementation from trusted RSS feeds. This should be displayed as a horizontal scrolling feed in the main page. The feed should be updated every 5 minutes. 
- [/] **Extend the supported coins to be configurable in the backend**: Reconciliation logic is implemented (unused coins/pairs are pruned from DB on startup), but the coin list is currently hardcoded in `CatalogService`.
- [ ] Add scheduled **month/year rollup buckets** derived from hourly/daily canonical candles rather than provider-native month archives for recent data. Only override historical data if the realtime data bucket is complete for the interval we are overriding. So for example, if we have one month data of realtime data for a coin, we should override the 30 day bucket with the realtime data, that is wipped out after the consolidation happen, ensuring only recent data is derived from realtime data and keeping data size small.
- [ ] **Historical backfill with day and month resolution** for all coin historical prices. For this feature add a minimum date to pull data from (ie, 2016) configured with a Variable in backend. From this date, you should use the oldest date available for each coin considering the date it was listed in relevant exchanges (ie, Kraken/Binance). Decide smartly this feature
- [ ] Design a way to graphically visualize gaps in data resolution in operations panel
- [ ] **Fiat/Forex Pairs**: Begin tracking currency-to-currency pairs (e.g., EURUSD, EURGBP).
- [ ] Implement robust source load-balancing and rate-limit back-off (circuit breakers for `429 Too Many Requests`).
- [x] **Caching Layer**: Implemented multi-layered ticker caching with per-ticker invalidation and time-based global snapshots.
- [ ] Implement a **Redis-based cache layer**: Transition news feed and historical price reads to a Redis backend for sub-millisecond response times and reduced primary database load.
- [ ] Implement api call examples in swagger
- [ ] Add **Yahoo Finance (Yfinance)** as a ticker provider.

## Current Focus
**News Feed Implementation & Market View Enhancements**
Implementing a real-time RSS news ticker for the dashboard and completing the 1h/7d market metrics in the ticker read model.

Operational rule updates:
- Historical source fetch granularity must never be coarser than **1 day**, and **1 hour** remains the preferred canonical backfill resolution.
- Provider-native monthly archives should not drive historical sweeps; larger buckets such as month/year must be built later by scheduled consolidation from canonical stored candles.
