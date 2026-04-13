# Exchangely

> [!NOTE]
> This project is being built for educational purposes only. It is not intended for use in any production environment.


Started as a "poor man's CoinGecko" for historical data availability Exchangely is an event-driven crypto market data platform focused on historical OHLCV coverage for curated crypto/fiat and crypto/stablecoin pairs.

<table>
  <tr>
    <td>Market Overview</td>
     <td>Portfolio Tracker</td>
  </tr>
  <tr>
    <td><img src="./market.png" width=640 height=480></td>
    <td><img src="./portfolio.png" width=640 height=480></td>
  </tr>
 </table>


## Features

- **Real-time Market Dashboard** — Live prices, 1h/24h/7d variation, 24h volume, high/low, market cap, and embedded 24h sparkline candles for all tracked pairs. Updates via SSE with no polling.
- **Historical OHLCV Data** — Automated backwards backfill from multiple providers with hourly and daily resolution. REST API with interval, start/end time filtering.
- **Multi-Source Aggregation** — Five data providers (Binance, Binance Vision, Kraken, CoinGecko, CryptoDataDownload) with automatic cross-source consolidation and integrity checks.
- **Market News Feed** — Horizontal scrolling ticker with curated crypto news from CoinDesk, Cointelegraph, and TheBlock RSS feeds, refreshed every 15 minutes.
- **Authentication & Access Control** — Opt-in auth via `BACKEND_AUTH_MODE` supporting Google OAuth 2.0, local email/password, or both. JWT sessions with refresh tokens, role-based access (admin/user), rate limiting with progressive IP lockout, and password change flow. See [Authentication documentation](./docs/authentication.md).
- **API Tokens & Rate Limiting** — Per-user `exly_`-prefixed API tokens for programmatic access. Tiered rate limits (user/premium/admin) backed by PostgreSQL sliding window counters, per-IP abuse prevention, and a frontend API key management page. See [API documentation](./docs/api.md).
- **Portfolio Tracker** — Track crypto holdings from manual entries, exchange API syncs (Binance, Kraken, Coinbase), on-chain wallet addresses (Ethereum/ERC-20, Solana/SPL, Bitcoin), and Ledger Live imports. Live portfolio valuation, allocation breakdown, P&L, and historical value charts powered by Exchangely's own price data. All sensitive data encrypted at rest with per-user keys. See [Portfolio documentation](./docs/portfolio.md).
- **Admin User Management** — List, search, and filter users. Change roles (user/premium/admin), disable/enable accounts with automatic session invalidation, and force password resets. All operations are admin-only and audit-logged.
- **Operations Center** — Three-tab admin panel (gated to admin role when auth is enabled): system health warnings, coin-grouped coverage view (live feed health + backfill status per base asset in collapsible cards), and task audit log. All SSE-driven.
- **Event-Driven Task Engine** — Planner/worker architecture with Kafka-distributed tasks, DB-backed leader election, per-pair advisory locks, and configurable throughput controls. See [Task Lifecycle](./docs/lifecycle.md).
- **Data Integrity** — Gap validation, cross-source integrity checks, daily backfill probes, and automatic task cleanup with configurable retention.

## Shared Component Library

Reusable UI components live in `frontend/src/components/ui/` with a barrel export:

```tsx
import { Button, Badge, Card, Input, Modal, Alert } from '../components/ui';
```

Available components: Button, Badge, Card, Input, Table (TableHead, TableBody, TableRow, TableCell), Modal, ToggleGroup, StatusDot, Spinner, LogViewer, Alert, Sparkline, EmptyState.

## Architecture



```mermaid
flowchart LR
    User((User))
    Bot((API Consumer))
    User --> UI[React UI]
    UI -->|REST + Bearer JWT| API
    Bot -->|REST + API Token| API

    API[API Role]
    API -->|OAuth redirect/callback| Google[[Google]]

    API -.->|SSE: tickers| UI
    API -.->|SSE: news| UI
    API -.->|SSE: tasks| UI
    API -.->|SSE: portfolio| UI

    API -->|exchange balance sync| HotWallets
    API -->|wallet balance sync| ColdWallets

    Planner[Planner Role]
    Planner -->|lease + sync state| TS
    Planner -->|store tasks| TS
    Planner -->|publish task refs| Kafka

    Kafka[(Kafka)]
    Kafka -->|consume tasks| Worker

    Worker[Worker Role]
    Worker -->|claim tasks + pair locks| TS
    Worker -->|write candles + sync state| TS
    Worker -->|notify completion| Kafka

    Worker -->|historical fetch| Historical
    Worker -->|realtime poll| RealtimeSrc
    Worker -->|news fetch| News

    subgraph Historical[Historical Providers]
        BinanceVision[[Binance Vision]]
        BinanceHist[[Binance Historical API]]
        KrakenHist[[Kraken Historical API]]
        CDD[[CryptoDataDownload]]
    end

    subgraph RealtimeSrc[Realtime Providers]
        BinanceRT[[Binance Tickers API]]
        KrakenRT[[Kraken Tickers API]]
        CoinGecko[[CoinGecko Tickers API]]
    end

    subgraph News[News Sources]
        CoinDesk[[CoinDesk RSS]]
        CoinTelegraph[[CoinTelegraph RSS]]
        TheBlock[[TheBlock RSS]]
    end

    subgraph HotWallets[Hot Wallets]
        Binance[[Binance]]
        Kraken[[Kraken]]
        Coinbase[[Coinbase]]
    end

    subgraph ColdWallets[Cold Wallets]
        LedgerLive[[Ledger Live]]
        Blockstream[[Blockstream]]        
        Etherscan[[Etherscan]]
        SolanaRPC[[Solana RPC]]
    end

    API -->|read candles + tickers| TS
    API -->|users + sessions + tokens| TS
    API -->|rate limit counters| TS
    API -->|portfolio holdings + valuation| TS
    TS[(TimescaleDB)]
```

### Data Flow

Historical backfill and live ticker are intentionally decoupled:

- **Historical backfill** walks backwards from yesterday into the past (no fixed start date), so charts are useful immediately. A daily probe extends each pair one hour further to discover newly available upstream data.
- **Live ticker** starts immediately per pair with at most one task in the queue at a time. Once a worker completes a ticker poll, the next planner tick re-enqueues it.
- **Consolidation** aggregates raw samples into hourly candles, then hourly into daily.

### Data Providers

#### Crypto

| Provider | Historical | Realtime | Method |
|----------|:----------:|:--------:|--------|
| Binance | ✓ | ✓ | REST OHLC + `/ticker/24hr` |
| Binance Vision | ✓ | | Bulk CSV archives |
| Kraken | ✓ | ✓ | REST OHLC + `/Ticker` |
| CoinGecko | | ✓ * | `/simple/price` |
| CryptoDataDownload | ✓ | | Hourly/daily CSV |

\* Requires separate API key for the provider

#### News 

News are ingested from CoinDesk, Cointelegraph, and TheBlock RSS feeds.


## Running Exchangely

> [!NOTE]
> There is no pre-built image to run the platform at this point. You must run build your own docker files.

1. Copy `.env.example` to `.env` and adjust values if needed
2. Run `docker compose up --build`.
3. Open the frontend at `http://localhost:5173`.
4. Open the backend API at `http://localhost:8080/api/v1/health`.

### Configuration

All settings are controlled via environment variables. Override them in `.env` or `docker-compose.yml`.

#### Core

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKEND_ENV` | Runtime environment (`development`, `production`) | `development` |
| `BACKEND_HTTP_ADDR` | HTTP listen address | `:8080` |
| `BACKEND_ROLE` | Comma-separated roles: `api`, `planner`, `worker`, or `all` | `all` |
| `BACKEND_LOG_LEVEL` | Log verbosity (`debug`, `info`, `warn`, `error`) | `info` |
| `BACKEND_CORS_ALLOWED_ORIGINS` | Comma-separated allowed CORS origins | `localhost:5173` (dev) |

#### Database & Messaging

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKEND_DATABASE_URL` | TimescaleDB/PostgreSQL connection string | _(required)_ |
| `BACKEND_KAFKA_BROKERS` | Comma-separated Kafka broker addresses | _(required)_ |
| `BACKEND_KAFKA_TOPIC_TASKS` | Kafka topic for task distribution | `exchangely.tasks` |
| `BACKEND_KAFKA_TOPIC_MARKET_TICKS` | Kafka topic for market events | `exchangely.market.ticks` |
| `BACKEND_KAFKA_CONSUMER_GROUP` | Kafka consumer group name | `exchangely-workers` |

#### Planner

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKEND_PLANNER_LEASE_NAME` | DB lease name for leader election | `planner_leader` |
| `BACKEND_PLANNER_LEASE_TTL` | Leader lease time-to-live | `15s` |
| `BACKEND_PLANNER_TICK` | Planner scheduling loop interval | `10s` |
| `BACKEND_PLANNER_BACKFILL_BATCH_PERCENT` | % of worker batch size allocated to backfill per planner tick | `50` |
| `BACKEND_REALTIME_POLL_INTERVAL` | How often the planner emits realtime ticker tasks per pair | `5s` |

#### Worker

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKEND_WORKER_POLL_INTERVAL` | Worker task polling interval | `5s` |
| `BACKEND_WORKER_BATCH_SIZE` | Max tasks claimed per worker poll | `100` |
| `BACKEND_WORKER_CONCURRENCY` | Parallel task workers within a batch | `4` |
| `BACKEND_WORKER_BACKFILL_BATCH_PERCENT` | % of worker batch size allocated to backfill per poll | `50` |

#### Data Providers

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKEND_ENABLE_BINANCE` | Enable Binance provider | `true` |
| `BACKEND_ENABLE_BINANCE_VISION` | Enable Binance Vision CSV provider | `true` |
| `BACKEND_ENABLE_KRAKEN` | Enable Kraken provider | `true` |
| `BACKEND_ENABLE_COINGECKO` | Enable CoinGecko provider | `true` |
| `BACKEND_ENABLE_CRYPTODATADOWNLOAD` | Enable CryptoDataDownload CSV provider | `true` |
| `BACKEND_COINGECKO_API_KEY` | CoinGecko API key (optional, for higher rate limits) | _(empty)_ |
| `BACKEND_CDD_AVAILABILITY_BASE_URL` | CryptoDataDownload availability endpoint override | _(empty)_ |
| `BACKEND_DEFAULT_QUOTE_ASSETS` | Comma-separated quote currencies to track | `EUR,USD` |

#### Data Integrity

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKEND_INTEGRITY_MIN_SOURCES` | Minimum sources required for cross-source validation | `2` |
| `BACKEND_INTEGRITY_MAX_DIVERGENCE_PCT` | Max allowed price divergence % between sources | `0.5` |

#### Caching

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKEND_TICKER_CACHE_SIZE` | Max individual tickers cached in memory | `100` |
| `BACKEND_TICKERS_CACHE_TTL` | TTL for the global tickers snapshot cache | `30s` |

#### Maintenance

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKEND_TASK_RETENTION_PERIOD` | How long completed/failed tasks are kept before pruning | `24h` |
| `BACKEND_TASK_MAX_LOG_COUNT` | Max completed/failed tasks kept per cleanup cycle | `1000` |
| `BACKEND_NEWS_FETCH_INTERVAL` | How often the worker fetches news from RSS feeds | `15m` |
| `BACKEND_INTEGRITY_CHECK_INTERVAL` | How often integrity check sweeps are scheduled per pair | `24h` |
| `BACKEND_GAP_VALIDATION_INTERVAL` | How often gap validation sweeps are scheduled per pair | `24h` |

#### Frontend

| Variable | Description | Default |
|----------|-------------|---------|
| `API_BASE_URL` | Backend API base URL (shared by frontend build and backend OpenAPI spec) | `http://localhost:8080/api/v1` |

#### Authentication

> [!NOTE]
> See the [authentication guide](./docs/authentication.md) for setup instructions and the [API documentation](./docs/api.md) for token and rate limiting details.

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKEND_AUTH_MODE` | Auth mode: `local`, `sso`, or `local,sso`. Empty disables all auth. | _(empty)_ |
| `BACKEND_JWT_SECRET` | HMAC-SHA256 secret for signing access tokens. Required when auth mode is set. | _(empty)_ |
| `BACKEND_JWT_EXPIRY` | Access token lifetime | `15m` |
| `BACKEND_REFRESH_TOKEN_EXPIRY` | Refresh token lifetime | `168h` (7 days) |
| `BACKEND_GOOGLE_CLIENT_ID` | Google OAuth 2.0 client ID. Required when auth mode includes `sso`. | _(empty)_ |
| `BACKEND_GOOGLE_CLIENT_SECRET` | Google OAuth 2.0 client secret. Required when auth mode includes `sso`. | _(empty)_ |
| `BACKEND_GOOGLE_REDIRECT_URI` | OAuth callback URL | `http://localhost:8080/api/v1/auth/google/callback` |
| `BACKEND_ADMIN_EMAIL` | Email for the local admin account. Required when auth mode includes `local`. | _(empty)_ |
| `BACKEND_BCRYPT_COST` | Bcrypt hashing cost factor for password storage | `12` |
| `BACKEND_TRUSTED_PROXIES` | Comma-separated proxy CIDRs/IPs for trusting `X-Forwarded-For` / `X-Real-IP` headers | _(empty)_ |

#### API Rate Limiting

> [!NOTE]
> API tokens and rate limiting require authentication to be enabled. See the [API documentation](./docs/api.md) for usage details.

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKEND_RATELIMIT_USER` | Max requests per window for `user` role | `100` |
| `BACKEND_RATELIMIT_PREMIUM` | Max requests per window for `premium` role | `500` |
| `BACKEND_RATELIMIT_ADMIN` | Max requests per window for `admin` role | `1000` |
| `BACKEND_RATELIMIT_IP` | Max requests per window per IP address (across all tokens) | `200` |
| `BACKEND_RATELIMIT_WINDOW` | Sliding window duration for rate limit counters | `1m` |

#### Portfolio Tracker

> [!NOTE]
> Portfolio features require authentication to be enabled. See the [Portfolio documentation](./docs/portfolio.md) for setup details.

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKEND_PORTFOLIO_ENABLED` | Feature flag to enable portfolio endpoints and encryption key validation at startup | `false` |
| `BACKEND_PORTFOLIO_ENCRYPTION_KEY` | Hex-encoded 32-byte AES-256 master key for encrypting credentials and sensitive metadata. Required when portfolio is enabled. | _(empty)_ |
| `BACKEND_ETHERSCAN_API_KEY` | Etherscan API key for Ethereum balance lookups | _(empty)_ |
| `BACKEND_SOLANA_RPC_URL` | Solana RPC endpoint for balance queries | `https://api.mainnet-beta.solana.com` |
| `BACKEND_BITCOIN_API_URL` | Bitcoin API endpoint for balance queries | `https://blockstream.info/api` |
