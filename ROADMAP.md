# Exchangely Roadmap

## Development Priorities (Suggested Order)

1. Responsive UI — Immediate value, no backend changes, unblocks mobile users
2. Auth + User Management — Unlocks Phase 2 entirely
3. API Rate Limiting + Tokens — Security prerequisite before going public
4. Portfolio Tracker — The killer feature; uses existing price data + exchange APIs
5. Price Alert Webhooks — High engagement feature; hooks into existing SSE infra
6. Interactive Charts — Visual upgrade that makes the historical data shine
7. Chainlink Provider — Accuracy improvement, free on-chain data
8. Circuit Breakers — Operational resilience as provider count grows
9. Everything else — Redis, forex, rollups, Yahoo Finance, gap viz

---

## Phase 1 — Platform Foundation

The jump from "data service" to "platform with users." These are the enablers for everything that follows.

### 1.1 Responsive & Mobile-First UI Overhaul
- [x] Redesign the frontend layout system for mobile/tablet/desktop breakpoints
- [x] Make the Market Overview table responsive (card layout on mobile, table on desktop)
- [x] Adapt the Operations panel tabs for touch-friendly navigation
- [x] Ensure the news ticker, charts, and sparklines render cleanly on small screens
- [x] Add a proper navigation drawer/hamburger menu for mobile
- [x] Maintain the current premium dark aesthetic across all breakpoints

### 1.2 Authentication & User Management
- [x] Add Google OAuth 2.0 login (primary auth method)
- [x] Implement JWT-based session management with refresh tokens
- [x] Create a `users` table with roles: `admin`, `user`
- [x] Build login/signup UI flow integrated into the existing layout
- [x] Admin role gates access to the Operations panel
- [x] User role gets Market view, Portfolio, and Alerts
- [x] Add user settings page (profile, preferences, connected accounts)
- [x] Add DB migration for users, sessions, and roles
- [x] Local email/password authentication with configurable `BACKEND_AUTH_MODE`
- [x] Rate limiting on login endpoints (per-email + IP-based progressive lockout)
- [x] Real IP resolution middleware for reverse proxy support (`BACKEND_TRUSTED_PROXIES`)
- [x] Password change flow with live complexity feedback
- [x] Unified `/api/v1/config` endpoint for frontend auth/version discovery

### 1.3 Frontend Design System
- [x] Extract shared UI components from repeated page-specific markup into `frontend/src/components/ui/`
- [x] Add design tokens (spacing, radius, transitions) to `globals.css`
- [x] Implement shared components: Button, Badge, Card, Input, Table, Modal, ToggleGroup, StatusDot, Spinner, LogViewer, Alert, Sparkline, EmptyState
- [x] Migrate consumer pages (Login, APIKeys, Market, Settings, Portfolio, Operations) to use shared components
- [x] Migrate remaining components (PasswordChange, AddHoldingModal, WalletManager, ExchangeCredentialManager, LedgerManager, SystemPanel)
- [x] Add barrel export for single-import access
- [x] Co-locate CSS into per-component and per-page files, reduce `globals.css` from 3000+ to ~250 lines (tokens, resets, utilities only)
- [ ] Migrate from custom CSS to Tailwind CSS for better theming support and utility-first styling

### 1.4 API Authentication & Rate Limiting
- [x] Generate per-user API tokens (`exly_`-prefixed, SHA-256 hashed) for programmatic access to historical data
- [x] Implement token-based auth middleware for `/api/v1/historical/*` and ticker endpoints
- [x] Add tiered rate limiting (100 req/min user, 500 premium, 1000 admin)
- [x] Rate limit by token + IP with sliding window counters in PostgreSQL
- [x] Return standard `429 Too Many Requests` with `Retry-After` headers and `X-RateLimit-*` response headers
- [x] Add API key management UI (create, revoke, view usage with rate limit progress)
- [x] Public endpoints (`/health`, `/assets`, `/pairs`, `/config`) remain unauthenticated
- [x] Admin user management: list, role change, disable/enable, force password reset via `/api/v1/system/users/*`

---

## Phase 2 — User-Facing Features

Require auth to be in place. Transform Exchangely from a dashboard into a tool people use daily.

### 2.1 Portfolio Tracker
- [ ] Design portfolio data model: manual holdings + exchange-linked positions
- [ ] Support manual portfolio entry (coin, quantity, avg buy price)
- [ ] Integrate read-only API keys for Kraken and Binance to pull live balances
- [ ] Calculate portfolio value over time using Exchangely's own historical price data
- [ ] Build Portfolio dashboard page: total value, allocation pie chart, P&L per asset
- [ ] Historical portfolio value chart (1d, 7d, 30d, 1y) using stored candle data
- [ ] SSE-driven live portfolio value updates (reuse ticker stream)
- [ ] Secure API key storage (encrypted at rest in PostgreSQL)

### 2.2 Price Alert Webhooks
- [ ] Design alert model: user, pair, condition (above/below/crosses), threshold, delivery method
- [ ] Support webhook delivery (POST to user-specified URL with signed payload)
- [ ] Support email delivery as secondary channel
- [ ] Alert evaluation engine that hooks into the existing SSE ticker stream
- [ ] Cooldown/snooze logic to prevent alert spam on volatile pairs
- [ ] Alert management UI: create, edit, delete, view history
- [ ] Alert history log showing trigger time, price at trigger, delivery status
- [ ] Webhook signature verification docs for consumers

### 2.3 Interactive Price Charts
- [ ] Replace sparkline trend indicators with full interactive candlestick charts
- [ ] Support multiple timeframes (1h, 1d, 1w, 1M) using existing historical API
- [ ] Add volume bars overlay
- [ ] Click-through from Market Overview row to detailed pair chart page
- [ ] Responsive chart rendering for mobile
- [ ] Consider lightweight charting library (e.g., lightweight-charts by TradingView)

---

## Phase 3 — Data & Provider Expansion

Broaden coverage, improve accuracy, and add new data dimensions.

### 3.1 Chainlink On-Chain Price Feeds
- [ ] Research available Chainlink price feed contracts per supported pair
- [ ] Implement an Ethereum RPC client for free `eth_call` reads (no gas cost)
- [ ] Register Chainlink as a realtime provider with `CapRealtime`
- [ ] Map Chainlink feed addresses to Exchangely pair symbols
- [ ] Handle RPC endpoint configuration (Infura/Alchemy free tier or public nodes)
- [ ] Add fallback logic if RPC calls fail (existing providers continue)

### 3.2 Yahoo Finance Provider
- [ ] Add Yahoo Finance as a ticker provider for fiat/forex and crypto pairs
- [ ] Implement rate-limit-aware fetching with backoff
- [ ] Register with `CapHistorical` + `CapRealtime` as appropriate

### 3.3 Fiat/Forex Pairs
- [ ] Extend the pair model to support currency-to-currency pairs (EURUSD, EURGBP, etc.)
- [ ] Identify free forex data sources compatible with the provider interface
- [ ] Add forex pairs to the asset catalog
- [ ] Ensure the UI handles non-crypto pairs gracefully (no market cap, different volume semantics)

### 3.4 Monthly & Yearly Rollup Consolidation
- [ ] Add `MonthlyFromDaily` and `YearlyFromMonthly` consolidation functions
- [ ] Emit `consolidation` tasks for `1M` and `1Y` intervals from the scheduler
- [ ] Only produce rollups when the underlying period is fully covered (completeness check)
- [ ] Store rollups with appropriate interval markers or a dedicated rollup table
- [ ] Expose `1M` and `1Y` intervals in the historical API
- [ ] Prune raw realtime data after consolidation into the coarser bucket

---

## Phase 4 — Operational Hardening

Make the platform resilient and observable at scale.

### 4.1 Provider Circuit Breakers & Rate Limit Backoff
- [ ] Implement circuit breaker pattern in the provider registry
- [ ] Detect `429 Too Many Requests` and back off with exponential delay
- [ ] Track per-provider health metrics (success rate, latency, error count)
- [ ] Automatic failover to alternative providers when primary is tripped
- [ ] Expose provider health in the Operations panel

### 4.2 Redis Cache Layer
- [ ] Add Redis to the Docker Compose topology
- [ ] Migrate ticker cache from in-memory to Redis for cross-instance consistency
- [ ] Cache news feed responses in Redis with TTL
- [ ] Cache historical data queries for popular pairs/intervals
- [ ] Keep PostgreSQL as fallback if Redis is unavailable

### 4.3 Data Gap Visualization
- [ ] Design a timeline/heatmap component showing data coverage per pair
- [ ] Color-code by resolution: green (hourly), yellow (daily), red (gap)
- [ ] Integrate into the Coverage tab in the Operations panel
- [ ] Use existing sync status data — no new backend endpoints needed

### 4.4 Enhanced API Documentation
- [ ] Add request/response examples to all OpenAPI endpoints
- [ ] Add authentication examples (API token usage)
- [ ] Document webhook payload format and signature verification
- [ ] Interactive Swagger UI with "Try it out" support
