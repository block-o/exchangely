/** Response from GET /api/v1/health — service readiness and dependency checks. */
export type HealthResponse = {
  /** Overall service status (e.g. "ok", "degraded"). */
  status: string;
  /** Per-dependency health results keyed by name (e.g. "postgres", "kafka"). */
  checks: Record<string, string>;
  /** Unix epoch seconds when the check was performed. */
  timestamp: number;
};

/** Per-pair backfill progress returned by GET /api/v1/system/sync-status. */
export type SyncPairStatus = {
  /** Trading pair symbol (e.g. BTCEUR). */
  pair: string;
  /** Whether the hourly backfill has reached the earliest available provider data. */
  backfill_completed: boolean;
  /** Unix epoch of the most recently synced hourly candle. */
  last_synced_unix: number;
  /** Unix epoch of the next hourly window the planner will target. */
  next_target_unix: number;
  /** Whether hourly-resolution backfill is fully caught up. */
  hourly_backfill_completed: boolean;
  /** Whether daily-resolution consolidation is fully caught up. */
  daily_backfill_completed: boolean;
  /** Unix epoch of the most recently synced hourly candle. */
  hourly_synced_unix: number;
  /** Unix epoch of the most recently synced daily candle. */
  daily_synced_unix: number;
  /** Unix epoch of the next hourly backfill target. */
  next_hourly_target_unix: number;
  /** Unix epoch of the next daily consolidation target. */
  next_daily_target_unix: number;
};

/** An active system warning surfaced in the Operations panel. */
export type ActiveWarning = {
  /** Unique warning identifier. */
  id: string;
  /** Severity level. */
  level: "warning" | "error";
  /** Short human-readable title. */
  title: string;
  /** Extended description with context. */
  detail: string;
  /** Stable fingerprint used for dismissal deduplication. */
  fingerprint: string;
  /** Unix epoch when the warning was raised. */
  timestamp?: number;
};

/** A supported asset from the catalog (GET /api/v1/assets). */
export type Asset = {
  /** Ticker symbol (e.g. BTC, EUR). */
  symbol: string;
  /** Human-readable name (e.g. Bitcoin, Euro). */
  name: string;
  /** Asset class — "crypto" or "fiat". */
  type: string;
};

/** A supported trading pair from the catalog (GET /api/v1/pairs). */
export type Pair = {
  /** Base asset symbol (e.g. BTC). */
  base: string;
  /** Quote asset symbol (e.g. EUR). */
  quote: string;
  /** Concatenated pair symbol used as the unique key (e.g. BTCEUR). */
  symbol: string;
};

/** A single OHLCV candle returned by GET /api/v1/historical/{pair}. */
export type Candle = {
  /** Trading pair symbol. */
  pair: string;
  /** Candle resolution — "1h" or "1d". */
  interval: string;
  /** Bucket start as unix epoch seconds. */
  timestamp: number;
  /** Opening price for the bucket. */
  open: number;
  /** Highest price during the bucket. */
  high: number;
  /** Lowest price during the bucket. */
  low: number;
  /** Closing price for the bucket. */
  close: number;
  /** Traded volume during the bucket. */
  volume: number;
  /** Data provider that produced this candle. */
  source: string;
  /** Whether the candle covers a completed time window. */
  finalized: boolean;
};

/**
 * Lightweight OHLCV point for sparkline rendering, embedded in the /tickers
 * response. Omits pair, interval, source, and finalized to keep the payload small.
 */
export interface SparklinePoint {
  /** Bucket start as unix epoch seconds. */
  timestamp: number;
  /** Opening price. */
  open: number;
  /** Highest price. */
  high: number;
  /** Lowest price. */
  low: number;
  /** Closing price. */
  close: number;
  /** Traded volume. */
  volume: number;
}

/**
 * Represents a point-in-time snapshot of market data for a specific trading pair.
 */
export interface Ticker {
  /** The unique symbol of the pair (e.g., BTCEUR) */
  pair: string;
  /** Current market price from the latest source */
  price: number;
  /** Estimated market capitalization (Price * CirculatingSupply) */
  market_cap: number;
  /** Percentage change over the last hour */
  variation_1h: number;
  /** Percentage change over the last 24 hours */
  variation_24h: number;
  /** Percentage change over the last 7 days */
  variation_7d: number;
  /** Sum of traded volume over the last 24 hours */
  volume_24h: number;
  /** Highest price recorded in the last 24 hours */
  high_24h?: number;
  /** Lowest price recorded in the last 24 hours */
  low_24h?: number;
  /** Effective unix timestamp of the latest sample */
  last_update_unix: number;
  /** Name of the data source provider (e.g., binance, kraken) */
  source: string;
  /** Last 24 hourly candle points for sparkline rendering (included in /tickers response) */
  sparkline?: SparklinePoint[];
}

/** Generic envelope used by all list endpoints (assets, pairs, tickers, etc.). */
export type ListResponse<T> = {
  data: T[];
};
