export type HealthResponse = {
  status: string;
  checks: Record<string, string>;
  timestamp: number;
};

export type SyncPairStatus = {
  pair: string;
  backfill_completed: boolean;
  last_synced_unix: number;
  next_target_unix: number;
  hourly_backfill_completed: boolean;
  daily_backfill_completed: boolean;
  hourly_synced_unix: number;
  daily_synced_unix: number;
  next_hourly_target_unix: number;
  next_daily_target_unix: number;
};

export type ActiveWarning = {
  id: string;
  level: "warning" | "error";
  title: string;
  detail: string;
  fingerprint: string;
};

export type Asset = {
  symbol: string;
  name: string;
  type: string;
};

export type Pair = {
  base: string;
  quote: string;
  symbol: string;
};

export type Candle = {
  pair: string;
  interval: string;
  timestamp: number;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
  source: string;
  finalized: boolean;
};

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
}

export type ListResponse<T> = {
  data: T[];
};
