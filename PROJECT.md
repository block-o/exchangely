# System Role & Objective

Act as an expert Staff Software Engineer. Your task is to architect and implement a new project called **Exchangely**.
Exchangely is a highly available "poor man's CoinGecko" focused specifically on solving industry gaps in historical crypto data availability.

## 1. Tech Stack Requirements

- **Backend API:** Golang
- **Frontend UI:** React (Just basic scaffolding for now to consume the API)
- **Event Bus/Message Broker:** Kafka (for task distribution and state handling)
- **Persistence:** TimescaleDB (PostgreSQL extension optimized for time-series data like OHLCV financial tickers)
- **API Documentation:** Swagger/OpenAPI
- **Infrastructure:** Docker & Docker Compose

## 2. Architecture & Roles

The system is event-driven and highly available. It relies on three specific roles to manage state and execution:

- **Coordinator:** Manages leader election. It determines which instance acts as the Planner. If no coordinator exists, the first instance assumes the role.
- **Planner:** Reviews system state and schedules tasks (e.g., initial syncs, 60-minute consolidation, daily consolidation). It must ensure tasks are independent. **Constraint:** Implement strict partitioning/locking so workers can process in parallel, but no two workers can modify the same trading pair simultaneously (e.g., BTCEUR is locked during its update, but BTCUSD can run concurrently).
- **Worker:** Executes the tasks scheduled by the Planner.

## 3. Core Features

- **Supported Pairs:** Initially EUR (Fiat) and USDT (Stablecoin) against top cryptocurrencies. The system must be easily extensible to new coins.
- **Initial Sync (Backfill):** Runs shortly after service startup. A scheduled process that backfills historical data for the top 25 coins. It runs continuously until it catches up to the current date, guaranteeing zero data gaps. Resolution targets: Hourly and Daily buckets.
- **Realtime Mode:** Once the initial sync is complete, live feeds start. The system must fetch ticker data from free smart contracts (e.g., Chainlink) and public exchanges (Binance, Kraken).
- **Consolidation:** Every 60 minutes, realtime data must be aggregated into an hourly bucket (OHLCV format) and subsequently into daily buckets.

## 4. Core API Endpoints

The Golang backend must expose at least the following RESTful endpoints, documented via Swagger:

- `GET /api/v1/health`
  - Returns the health status of the API, TimescaleDB connection, and Kafka connection.
- `GET /api/v1/assets`
  - Returns a list of supported base coins and quote currencies (e.g., BTC, ETH, EUR, USDT).
- `GET /api/v1/pairs`
  - Returns the available trading pairs currently tracked by the system (e.g., BTCEUR, ETHUSDT).
- `GET /api/v1/historical/{pair}`
  - The core endpoint for historical data.
  - **Query Parameters:** \* `interval` (required): Time bucket resolution (`1h`, `1d`).
    - `start_time` (optional): Unix timestamp.
    - `end_time` (optional): Unix timestamp.
  - **Response:** Array of OHLCV (Open, High, Low, Close, Volume) data objects.
- `GET /api/v1/ticker/{pair}`
  - Returns the latest consolidated price and 24h variation for the requested pair.
- `GET /api/v1/system/sync-status`
  - Returns the current state of the backfill process (e.g., which pairs are still catching up, last synced timestamp).

## 5. Testing & Deployment

- Provide robust unit and integration tests, particularly for the Planner's scheduling logic and Worker idempotency.
- Generate `Dockerfiles` for the Go backend and React frontend. Include .dockerignore to prevent dust
- Generate a comprehensive `docker-compose.yml` that spins up the App, Kafka, Zookeeper/KRaft, and TimescaleDB together.

## 6. Output Instructions

Do not write the entire codebase in one single message. Please follow these steps sequentially:

- **Step 1:** Review this architecture. Acknowledge the requirements, challenge any architectural flaws (e.g., how we handle leader election with Kafka vs. DB locks), and propose a final Technical Design.
- **Step 2:** Provide a detailed folder structure for the Golang backend and the React scaffolding.
- **Step 3:** Wait for my approval. Once approved, we will proceed to generate the code file by file or domain by domain.
