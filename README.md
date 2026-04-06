# Exchangely

Exchangely is an event-driven crypto market data platform focused on historical OHLCV availability for a curated set of crypto/fiat pairs.

![Market Dashboard](./docs/ui/market_dashboard.png)

## Disclaimer

This project is being built for educational purposes only. It is not intended for use in any production environment.

## Architecture

Historical series and live ticker are now intentionally decoupled. Historical backfill remains
canonical and gap-focused, while live ticker starts immediately per pair and establishes a live
cutover hour; hourly backfill then catches up only up to that cutover, and newer windows are
owned by the realtime feed plus consolidation.

```mermaid
flowchart LR
    User((User))
    User --> UI[UI]
    UI -->|REST| API

    API[API]
    API -->|read historical candles + latest ticker| TS
    API -.->|SSE: realtime tickers| UI
    API -.->|SSE: news| UI
    API -.->|SSE: tasks| UI

    Planner[Planner Role]
    Planner -->|lease + sync state| TS
    Planner -->|store tasks| TS
    Planner -->|publish tasks refs| Kafka

    Worker[Worker Role]
    Worker -->|claim tasks + locks| TS
    Worker -->|downsample resolution for historical data| TS
    Worker -->|fetch recent crypto news| News
    Worker -->|notifies task completion| Kafka
    Worker -->|realtime ticker polling| Realtime
    Worker -->|historical backfill| Historical

    subgraph Historical
        BinanceData[[Binance Data API]]
        CryptoDataDownload[[CryptoDataDownload CSV]]
    end
    
    CryptoDataDownload --> TS
    BinanceData --> TS

    subgraph Realtime
        Binance[[Binance API]]
        Kraken[[Kraken API]]
        CoinGecko[[CoinGecko API]]
    end

    subgraph News
        CoinDesk[[CoinDesk RSS]]
        CoinTelegraph[[CoinTelegraph RSS]]
        TheBlock[[TheBlock RSS]]
    end 

    Binance --> TS
    Kraken --> TS
    CoinGecko --> TS

    CoinDesk --> TS
    CoinTelegraph --> TS
    TheBlock --> TS

    Kafka[(Kafka)]
    Kafka -->|consume tasks| Worker

    TS[(TimescaleDB)]
```

## Features

- **Historical OHLCV**: Automated backfill and gap management for hourly and daily resolutions.
- **Real-time Dashboards**: Live ticker updates via SSE (Server-Sent Events).
- **Market News**: Curated news feed from major industry sources (CoinDesk, Cointelegraph, TheBlock).
- **Operations Center**: Real-time task monitoring and system health visibility.

## Configuration

The following environment variables can be used to tune the system:

- `BACKEND_ROLE`: Comma-separated list of roles (`api,planner,worker`). Default: `api,planner,worker`.
- `BACKEND_NEWS_FETCH_INTERVAL`: Frequency of news updates (e.g., `5m`, `1h`). Default: `5m`.
- `DATABASE_URL`: TimescaleDB connection string.
- `KAFKA_BROKERS`: List of Kafka broker addresses.

## Quick Start

1. Copy `.env.example` to `.env` and adjust values if needed.
2. Run `docker compose up --build`.
3. Open the frontend at `http://localhost:5173`.
4. Open the backend API at `http://localhost:8080/api/v1/health`.
