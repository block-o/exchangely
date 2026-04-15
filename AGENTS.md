# Exchangely Agent Guide

This file is the primary operating guide for future agent work in this repository. It describes what Exchangely is, how it works, and how to work in this codebase.

## Document Roles

- `AGENTS.md` (this file): Project bible. What Exchangely is, architecture, current state, working rules, validation commands. Start here.
- `ROADMAP.md`: Forward-looking phased roadmap with checkboxes. What we're building next.
- `README.md`: Human-facing overview and quick-start guide.

If these documents disagree:

1. Current code and tests define actual behavior.
2. `AGENTS.md` defines how agents should work in this repo and describes the current system.
3. `ROADMAP.md` defines what to build next.
4. `README.md` is overview material, not the detailed implementation contract.

---

## What Is Exchangely

Exchangely is an event-driven crypto market data platform focused on historical OHLCV coverage for curated crypto/fiat and crypto/stablecoin pairs. It started as a "poor man's CoinGecko" for historical data availability and is evolving into a full-featured exchange rate platform with user accounts, portfolio tracking, price alerts, and a public API.

## Core Stack

- **Backend**: Go single-binary (API + planner + worker roles via `BACKEND_ROLE`)
- **Frontend**: React + Vite dashboard with SSE-driven realtime updates
- **Persistence**: TimescaleDB/PostgreSQL (authoritative source of truth)
- **Transport**: Kafka (task distribution + market events, not coordination authority)
- **Infrastructure**: Docker Compose local topology, GitHub Actions CI

## Architecture Principles

These are non-negotiable unless the user explicitly changes the architecture.

- PostgreSQL/Timescale owns system state. Do not move scheduling truth, sync state, or coordination responsibility into Kafka.
- Planner leadership is lease-based and DB-backed. Keep leadership and scheduling resilient to multiple instances.
- Workers must never mutate the same trading pair concurrently. Pair-level locking is a hard requirement.
- Historical backfill walks backwards from yesterday toward the past. No fixed start date; the system keeps walking back until providers return no data. `BACKEND_PLANNER_BACKFILL_BATCH_PERCENT` caps backfill tasks per planner cycle.
- A daily backfill probe task extends each pair one hour further into the past, discovering new provider data even after regular backfill exhausts known sources.
- `1h` is the preferred canonical historical resolution. Historical fetch granularity must never be coarser than `1d`.
- Month/year rollups, if added, must be derived from canonical stored candles, not provider-native month archives.
- Realtime tasks use a stable ID per pair (`live_ticker:{PAIR}:realtime`). Enqueue logic guarantees at most one pending/running realtime task per pair. Do not add poll-window timestamps to realtime task IDs.
- News fetch tasks use a stable ID per RSS source (`news_fetch:{source}:periodic`). One pending/running task per source at a time. Do not add timestamps to news fetch task IDs.
- Integrity check tasks use a stable per-pair sweep ID (`integrity_check:{PAIR}:sweep`). The executor walks unverified days and marks results in the `integrity_coverage` table. Do not create per-hour windowed integrity tasks.
- Gap validation tasks use a stable per-pair sweep ID (`gap_validation:{PAIR}:sweep`). The executor walks uncovered days and marks complete days in the `data_coverage` table. Do not create per-day windowed gap tasks.
- Portfolio recompute tasks use a stable per-user ID (`portfolio_recompute:{USER_ID}:pending`). The `Interval` field carries an optional currency hint (e.g. "EUR") set by the handler on currency change; when empty, the executor queries distinct currencies from the user's transactions and computes P&L for each. Do not hardcode a single currency.
- P&L refresh tasks use a stable per-user ID (`pnl_refresh:{USER_ID}:periodic`). The executor queries distinct currencies from the user's transactions and refreshes P&L for all of them. If the currency lookup fails, it falls back to USD gracefully.
- Worker batch processing reserves slots for live_ticker tasks first, then fills remaining capacity with other task types, then backfill. This ensures realtime tasks always get worker slots regardless of queue depth.
- Prefer SSE for live UI updates when a stream already exists. Do not replace stream-driven flows with aggressive polling.
- The backend is intentionally one binary with role gating. Preserve that unless a deliberate architecture change is requested.
- Keep the project educational and local-first. Do not quietly harden it toward production assumptions without discussing the tradeoff.
- Auth is opt-in via `BACKEND_AUTH_MODE`. When empty, the auth middleware is bypassed and all endpoints are publicly accessible (matching pre-auth behavior). Valid modes are `local` (email/password only), `sso` (Google OAuth only), and `local,sso` (both). The backend validates that required variables are present for the chosen mode at startup and refuses to start if any are missing. When `BACKEND_AUTH_MODE` includes `sso` but `BACKEND_GOOGLE_CLIENT_ID` is empty, startup fails. When it includes `local` but `BACKEND_ADMIN_EMAIL` is empty, startup fails. `BACKEND_JWT_SECRET` is always required when any auth mode is set.
- When running behind a reverse proxy (nginx, Caddy, ALB, etc.), set `BACKEND_TRUSTED_PROXIES` to a comma-separated list of proxy CIDR ranges or IPs so that `X-Forwarded-For` / `X-Real-IP` headers are trusted for resolving the real client IP. This affects rate limiting, audit logging, and fail2ban-style IP blocking.

---

## Current Data Providers

| Provider            | Capabilities          | Notes                                      |
|---------------------|-----------------------|--------------------------------------------|
| Binance             | Historical + Realtime | OHLC + native `/ticker/24hr`               |
| Binance Vision      | Historical            | Bulk CSV archives                          |
| Kraken              | Historical + Realtime | OHLC + native `/Ticker`                    |
| CoinGecko           | Realtime              | `/simple/price` for live quotes            |
| CryptoDataDownload  | Historical            | Hourly/daily CSV fallback                  |

Default quote assets: `EUR` and `USD`.


## What's Built (Current State)

### Backend
- Single Go binary with planner/worker/API role gating
- DB-backed task & lease lifecycle with structured `slog` logging
- Backwards backfill strategy (yesterday → past, no fixed start date)
- Daily backfill probe: one task per pair per day extending coverage into the past
- Realtime dedup: stable per-pair task IDs, at most one live_ticker per pair
- Stable per-source news fetch tasks (one per RSS source, no duplicates)
- Stable per-pair integrity sweep with persistent verification tracking
- Stable per-pair gap validation sweep with persistent coverage tracking
- Worker batch reservation: live_ticker tasks always get priority slots
- Hourly + daily consolidation pipeline (`FromRaw` → `DailyFromHourly`)
- Advisory pair-level locking for concurrent worker safety
- Multi-layered ticker caching (per-ticker invalidation + time-based global)
- SSE delta streaming for tickers (only changed pairs emitted)
- Task cleanup executor with configurable retention (duration + count)
- Integrity check tasks for cross-source validation
- Gap validation tasks
- News feed ingestion from RSS sources (CoinDesk, Cointelegraph, TheBlock)
- Provider capability system (`CapHistorical`, `CapRealtime`) with registry pre-filtering
- Planner throughput control (backfill batch budget, realtime-first scheduling)
- Worker throughput control (historical-sweep cap per batch)
- Load testing suite integrated into CI
- Google OAuth 2.0 + local admin authentication with JWT sessions
- Role-based access control (admin/user) with Operations panel gating
- Auth middleware with graceful degradation (disabled when `BACKEND_JWT_SECRET` is empty)
- Per-user API tokens (`exly_`-prefixed) for programmatic access with SHA-256 hashed storage
- Tiered PostgreSQL-backed rate limiting (user/premium/admin) with sliding window counters and per-IP abuse prevention
- Admin user management: list, role change (user/premium/admin), disable/enable with session invalidation, force password reset
- Portfolio transactions: normalized transaction records from exchange syncs, wallet syncs, and Ledger imports with price resolution fallback chain (hourly → daily → unresolvable)
- FIFO P&L engine: realized/unrealized profit & loss computed from transactions using candle data, persisted as snapshots per user per currency
- Multi-currency P&L: executors query distinct currencies from user transactions and compute P&L for each; currency hint passed via task Interval field on recompute
- Debounced recompute: `portfolio_recompute` tasks use stable per-user IDs with configurable debounce window to batch rapid sync/import changes
- Periodic P&L refresh: planner schedules `pnl_refresh` tasks for users with active portfolios at configurable interval
- Admin P&L recompute: `POST /api/v1/system/users/{id}/recompute-pnl` triggers recompute for any user (admin only)
- Kraken trade history: `TradeHistoryConnector` interface with paginated `/0/private/TradesHistory` fetching, normalized into transactions during sync

### Frontend
- Premium dark-themed dashboard with SSE-driven realtime market updates
- Shared component library (`components/ui/`) with Button, Badge, Card, Input, Table, Modal, ToggleGroup, StatusDot, Spinner, LogViewer, Alert, Sparkline, EmptyState
- Market Overview with 1h%, 24h%, 7d%, 24h volume, high/low, trend sparklines
- Operations panel with three tabs: Overview (warnings + version), Coverage (coin-grouped), Audit (task history)
- Coverage tab: pairs grouped by base asset in collapsible cards with live feed health, backfill badges, earliest data
- Horizontal scrolling news ticker from RSS feeds
- API Key management page (create, list, revoke tokens with rate limit usage display)
- Portfolio page with four tabs: Holdings, P&L, Transactions, Sources
- Transactions tab: paginated list with resolution badges (~1h, ~1d, ⚠️ unresolvable), manual edit modal, source labels
- P&L tab: realized/unrealized/total summary cards, per-asset breakdown table, approximate values notice, excluded transaction count
- Sources tab: unified view merging Exchange Connections, Wallets, Ledger, and Manual Holdings managers
- Exchange capability badges: per-credential Balances + Transactions status indicators (green/red based on connection health and trade history support)
- Currency resync notification: inline warning banner and "recalculating…" tab badges during reference currency change, SSE-driven completion
- Vitest testing stack
- Dynamic `__APP_VERSION__` injection

### API Surface
- `GET /api/v1/health`
- `GET /api/v1/assets`
- `GET /api/v1/pairs`
- `GET /api/v1/historical/{pair}` (with `interval`, `start_time`, `end_time`)
- `GET /api/v1/ticker/{pair}`
- `GET /api/v1/tickers`
- `GET /api/v1/tickers/stream` (SSE)
- `GET /api/v1/system/sync-status`
- `GET /api/v1/system/tasks`
- `GET /api/v1/system/tasks/stream` (SSE)
- `GET /api/v1/news`
- `GET /api/v1/news/stream` (SSE)
- `GET /api/v1/auth/google/login` (OAuth redirect)
- `GET /api/v1/auth/google/callback` (OAuth callback)
- `POST /api/v1/auth/local/login` (email/password login)
- `POST /api/v1/auth/refresh` (token refresh via cookie)
- `POST /api/v1/auth/logout` (session invalidation)
- `GET /api/v1/auth/me` (authenticated user profile)
- `POST /api/v1/auth/local/change-password` (password update)
- `POST /api/v1/auth/api-tokens` (create API token, JWT session only)
- `GET /api/v1/auth/api-tokens` (list user's API tokens, JWT session only)
- `DELETE /api/v1/auth/api-tokens/{id}` (revoke API token, JWT session only)
- `GET /api/v1/config` (frontend-facing app configuration: auth state, version)
- `GET /api/v1/system/users` (list users with pagination/filters, admin only)
- `GET /api/v1/system/users/{id}` (get single user, admin only)
- `PATCH /api/v1/system/users/{id}/role` (update user role, admin only)
- `PATCH /api/v1/system/users/{id}/status` (enable/disable user, admin only)
- `POST /api/v1/system/users/{id}/force-password-reset` (force password reset, admin only)
- `POST /api/v1/system/users/{id}/recompute-pnl` (trigger P&L recalculation for user, admin only)
- `GET /api/v1/portfolio/holdings` (list user holdings)
- `POST /api/v1/portfolio/holdings` (create manual holding)
- `PUT /api/v1/portfolio/holdings/{id}` (update holding)
- `DELETE /api/v1/portfolio/holdings/{id}` (delete holding)
- `GET /api/v1/portfolio/valuation` (portfolio valuation with `?quote=`)
- `GET /api/v1/portfolio/history` (historical portfolio value)
- `POST /api/v1/portfolio/sync-all` (sync all exchange/wallet sources)
- `POST /api/v1/portfolio/recompute` (recompute transactions + P&L, accepts `?quote=` currency hint)
- `GET /api/v1/portfolio/credentials` (list exchange credentials)
- `POST /api/v1/portfolio/credentials` (add exchange credential)
- `DELETE /api/v1/portfolio/credentials/{id}` (remove credential)
- `POST /api/v1/portfolio/credentials/{id}/sync` (sync single credential)
- `GET /api/v1/portfolio/wallets` (list wallets)
- `POST /api/v1/portfolio/wallets` (add wallet)
- `DELETE /api/v1/portfolio/wallets/{id}` (remove wallet)
- `POST /api/v1/portfolio/wallets/{id}/sync` (sync single wallet)
- `POST /api/v1/portfolio/ledger/connect` (import Ledger Live export)
- `POST /api/v1/portfolio/ledger/sync` (re-sync ledger)
- `DELETE /api/v1/portfolio/ledger` (disconnect ledger)
- `GET /api/v1/portfolio/stream` (SSE: portfolio valuation + transaction/P&L update events)
- `GET /api/v1/portfolio/transactions` (paginated transaction list with asset/type/date filters)
- `PUT /api/v1/portfolio/transactions/{id}` (edit transaction value/notes, sets manually_edited flag)
- `GET /api/v1/portfolio/pnl` (P&L snapshot with `?quote=` currency)

When changing API behavior, update `router.go:defaultOpenAPIYAML()` if the contract changes materially.

### DevOps
- GitHub Actions CI (Go 1.26 + Node.js 24)
- Docker Compose topology (TimescaleDB, Kafka, backend, frontend)
- Compose-based smoke/e2e tests (`make e2e`)
- Configurable asset catalog via YAML
- Commits start with uppercase letter, ignoring the type of work done like "Improved performance in the api" 

### Task Types

| Type                  | Description                                                    |
|-----------------------|----------------------------------------------------------------|
| `historical_backfill` | Walks backwards from yesterday; also used for daily probes     |
| `live_ticker`         | Stable ID per pair; at most one pending/running per pair       |
| `integrity_check`     | Stable per-pair sweep; walks unverified days, marks results    |
| `consolidation`       | Hourly → daily aggregation (future: monthly, yearly)           |
| `task_cleanup`        | Scheduled pruning of completed/failed task logs                |
| `news_fetch`          | Stable per-source RSS fetch (coindesk, cointelegraph, theblock)|
| `gap_validation`      | Stable per-pair sweep; validates coverage, marks complete days |
| `portfolio_recompute` | Full transaction re-normalization + FIFO P&L recompute per user. Stable ID: `portfolio_recompute:{USER_ID}:pending`. Interval field carries currency hint when triggered by currency change. |
| `pnl_refresh`         | Periodic unrealized P&L refresh with current ticker prices per user. Stable ID: `pnl_refresh:{USER_ID}:periodic`. Queries distinct currencies from user's transactions. |

---

## Current Runtime Model

Key implementation files:

- `backend/cmd/server/main.go`: backend entrypoint.
- `backend/cmd/migrate/main.go`: migration entrypoint.
- `backend/internal/app/app.go`: application wiring, enabled sources, runtime roles.
- `backend/internal/planner/runtime.go`: leader lease renewal and task scheduling loop.
- `backend/internal/planner/scheduler.go`: task generation logic (backfill, realtime, probes, cleanup).
- `backend/internal/worker/processor.go`: worker claim/execute path.
- `backend/internal/httpapi/router/router.go`: REST and SSE endpoints.
- `backend/internal/storage/postgres/*`: persistence, task state, sync state, leases, locks.
- `backend/internal/storage/postgres/integrity_repository.go`: integrity coverage persistence (integrity checks).
- `frontend/src/components/SystemPanel.tsx`: operations dashboard with three tabs.
- `frontend/src/components/system/CoverageTab.tsx`: coin-grouped coverage view.
- `docker-compose.yml`: local topology and default environment wiring.

## Useful File Map

- `backend/internal/service/*`: application services used by the HTTP layer.
- `backend/internal/ingest/*`: market data providers and registry.
- `backend/internal/messaging/kafka/*`: Kafka producers, consumers, health checks.
- `backend/tests/integration/*`: integration coverage.
- `backend/tests/e2e/*`: compose-backed end-to-end tests.
- `frontend/src/api/*`: frontend API clients.
- `frontend/src/pages/*`: top-level screens.
- `frontend/src/components/ui/*`: shared design system components (Button, Badge, Card, Input, Table, Modal, ToggleGroup, StatusDot, Spinner, LogViewer, Alert, Sparkline, EmptyState). Barrel export at `components/ui/index.ts`.
- `frontend/src/components/*`: UI building blocks.
- `frontend/src/components/system/*`: system operations tab components (OverviewTab, CoverageTab, AuditTab, shared utilities).
- `backend/internal/auth/*`: auth service, JWT, validation, rate limiting.
- `backend/internal/auth/admin.go`: admin user management service (list, role, disable, force password reset).
- `backend/internal/auth/apitoken.go`: API token service (create, validate, revoke, list).
- `backend/internal/auth/apiratelimit.go`: PostgreSQL-backed tiered rate limiter.
- `backend/internal/httpapi/middleware/auth.go`: JWT auth middleware.
- `backend/internal/httpapi/middleware/apitoken.go`: API token auth middleware.
- `backend/internal/httpapi/middleware/ratelimit.go`: rate limit middleware.
- `backend/internal/httpapi/handlers/auth.go`: auth HTTP handlers (login, OAuth, token management).
- `backend/internal/httpapi/handlers/admin_users.go`: admin user management HTTP handlers.
- `backend/internal/storage/postgres/user_repository.go`: user persistence.
- `backend/internal/storage/postgres/session_repository.go`: session persistence.
- `backend/internal/storage/postgres/apitoken_repository.go`: API token persistence.
- `backend/internal/storage/postgres/ratelimit_repository.go`: rate limit counter persistence.
- `frontend/src/app/auth.tsx`: auth context provider.
- `frontend/src/pages/LoginPage.tsx`: login page.
- `frontend/src/pages/SettingsPage.tsx`: settings page.
- `frontend/src/pages/PasswordChangePage.tsx`: password change page.
- `frontend/src/pages/APIKeysPage.tsx`: API key management page.
- `docs/authentication.md`: authentication setup guide.
- `docs/api.md`: API documentation (endpoints, tokens, rate limiting).
- `docs/lifecycle.md`: task lifecycle documentation (planner, worker, coordinator interaction).
- `market_dashboard.png`: current dashboard reference image.
- `backend/internal/portfolio/*`: portfolio services (valuation, transactions, P&L, price resolution, encryption, notifier).
- `backend/internal/portfolio/exchange/*`: exchange connectors (Binance, Kraken, Coinbase) with balance and trade history fetching.
- `backend/internal/portfolio/wallet/*`: on-chain wallet connectors (Ethereum, Solana, Bitcoin).
- `backend/internal/portfolio/ledger/*`: Ledger Live export parser.
- `backend/internal/portfolio/transaction_service.go`: transaction normalization, manual edits, debounced recompute queuing.
- `backend/internal/portfolio/pnl_engine.go`: FIFO P&L computation and snapshot persistence.
- `backend/internal/portfolio/price_resolver.go`: candle-based price resolution with hourly → daily → unresolvable fallback.
- `backend/internal/portfolio/notifier.go`: SSE portfolio update broadcaster for recompute/refresh completion.
- `backend/internal/worker/recompute_executor.go`: portfolio_recompute task executor with multi-currency support.
- `backend/internal/worker/pnl_refresh_executor.go`: pnl_refresh task executor with multi-currency support.
- `backend/internal/httpapi/handlers/transactions.go`: transaction and P&L HTTP handlers.
- `backend/internal/httpapi/handlers/portfolio.go`: portfolio holdings, credentials, wallets, ledger, valuation, sync, recompute handlers.
- `backend/internal/storage/postgres/transaction_repository.go`: transaction persistence with DistinctCurrencies query.
- `backend/internal/storage/postgres/pnl_repository.go`: P&L snapshot persistence keyed by (user_id, reference_currency).
- `backend/internal/domain/portfolio/transaction.go`: Transaction, PnLSnapshot, AssetPnL, LedgerEntry domain types and repository interfaces.
- `frontend/src/components/portfolio/*`: portfolio UI components (TransactionsTab, PnLTab, ExchangeCredentialManager, WalletManager, LedgerManager, etc.).
- `frontend/src/components/system/shared.tsx`: task type filters, labels, icons, and descriptions for the operations panel.
- `frontend/src/pages/PortfolioPage.tsx`: portfolio page with Holdings/P&L/Transactions/Sources tabs.

---

## Repo-Specific Working Rules

Before starting substantial changes:

- Read this file.
- Check `ROADMAP.md` for current priorities.
- Inspect the actual implementation files you will touch.

Code comment style:

- Do not use decorative Unicode box-drawing characters in comments (e.g. `── Section ─────`). Use plain language section comments instead.
- In Go files, use standard `//` comments. For section headers use `// --- Section name ---`.
- In TypeScript/TSX files, use `//` for single-line comments. For section headers, a blank line is sufficient separation; add a short `//` comment only when the purpose is non-obvious.
- In CSS files, standard `/* Section */` comments are fine.
- Do not reference spec task IDs, requirement numbers, or internal tracking in code comments (e.g. `(Task 1.1)`, `(Req 3.5)`). Keep comments useful for the next developer reading the code, not for tracing back to a planning document.
- Keep comments concise and informative. If the code is self-explanatory, skip the comment.

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
- Use shared components from `frontend/src/components/ui/` before creating page-specific markup. Import via the barrel export: `import { Button, Badge } from '../components/ui'`.

When changing configuration:

- Update `backend/internal/config/config.go`.
- Update the Configuration table in `README.md` to reflect the new variable, default, and description.
- Update `docker-compose.yml` if the local stack should expose the setting (add the env var with `${VAR:-default}` syntax).
- Update `.env.example` if the variable is commonly overridden.
- Update example config files when relevant.
- Mention new operational knobs in `AGENTS.md` if they matter to future work.

When changing schema or persistence behavior:

- Add a migration in `backend/migrations`.
- Keep migrations forward/backward paired.
- Validate the affected repositories and any runtime assumptions tied to sync/task state.

## Validation Commands

Use the repo's existing commands instead of inventing one-off workflows.

- `make backend-fmt` / `make backend-fmt-fix`
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

E2E notes: `make e2e` uses `scripts/compose-smoke.sh`. The smoke flow validates the live Compose stack plus Kafka topics/consumer groups.

---

## Practical Default For Future Agents

If you are unsure where to put new information:

- Stable repo-specific rules and project state → `AGENTS.md`
- Roadmap items and feature plans → `ROADMAP.md`
- User-facing setup or overview → `README.md`
