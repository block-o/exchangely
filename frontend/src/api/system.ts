import { apiGet } from "./client";
import type { SyncPairStatus, Ticker } from "../types/api";

export function fetchSyncStatus() {
  return apiGet<SyncPairStatus[]>("/system/sync-status");
}

export function fetchTicker(pair: string) {
  return apiGet<Ticker>(`/ticker/${pair}`);
}
