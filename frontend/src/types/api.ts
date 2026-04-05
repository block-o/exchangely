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

export interface Ticker {
  pair: string;
  price: number;
  market_cap: number;
  variation_24h: number;
  high_24h?: number;
  low_24h?: number;
  last_update_unix: number;
  source: string;
};

export type ListResponse<T> = {
  data: T[];
};
