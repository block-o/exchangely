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

    UI[UI]
    UI -->|REST| API

    API[API]
    API -->|read historical candles + latest ticker| TS
    API -.->|SSE: Tickers & Tasks| UI

    Planner[Planner Role]
    Planner -->|lease + sync state| TS
    Planner -->|store tasks| TS
    Planner -->|publish tasks refs| Kafka

    Worker[Worker Role]
    Worker -->|claim tasks + locks| TS
    Worker -->|downsample resolution for historical data| TS
    Worker -->|notifies task completion| Kafka
    Worker -->|realtime ticker polling| Kraken & CoinGecko & Binance
    Worker -->|historical backfill| BinanceData & CryptoDataDownload

    CryptoDataDownload[[CryptoDataDownload CSV]]
    CryptoDataDownload --> TS

    BinanceData[[Binance Data API]]
    BinanceData --> TS
    Binance[[Binance API]]
    Binance --> TS
    Kraken[[Kraken API]]
    Kraken --> TS
    CoinGecko[[CoinGecko API]]
    CoinGecko --> TS


    Kafka[(Kafka)]
    Kafka -->|consume tasks| Worker

    TS[(TimescaleDB)]

```

## Quick Start

1. Copy `.env.example` to `.env` and adjust values if needed.
2. Run `docker compose up --build`.
3. Open the frontend at `http://localhost:5173`.
4. Open the backend API at `http://localhost:8080/api/v1/health`.
