# Exchangely

Exchangely is an event-driven crypto market data platform focused on historical OHLCV availability for a curated set of crypto/fiat and crypto/stablecoin pairs.

![Market Dashboard](./docs/ui/market_dashboard.png)

## Disclaimer

This project is being built for educational purposes only. It is not intended for use in any production environment.

## Architecture

```mermaid
flowchart LR
    UI[React Frontend]
    API[Go API]
    Planner[Planner Role]
    Worker[Worker Role]
    Kafka[(Kafka)]
    TS[(TimescaleDB)]
    BinanceVision[Binance Vision]
    Binance[Binance API]
    Kraken[Kraken API]

    UI -->|REST| API
    API -.->|SSE: Tickers & Tasks| UI

    Planner -->|lease + sync state| TS
    Planner -->|enqueue tasks| TS
    Planner -->|publish task refs| Kafka

    Worker -->|claim tasks + locks| TS
    Worker -->|consume tasks| Kafka
    Worker -->|write raw + consolidated candles| TS
    Worker -->|publish realtime market events| Kafka

    API -->|consume market events| Kafka
    API -->|read historical| TS

    Worker -->|historical backfill| BinanceVision
    Worker -->|recent USDT windows| Binance
    Worker -->|EUR windows| Kraken
```

## Quick Start

1. Copy `.env.example` to `.env` and adjust values if needed.
2. Run `docker compose up --build`.
3. Open the frontend at `http://localhost:5173`.
4. Open the backend API at `http://localhost:8080/api/v1/health`.

