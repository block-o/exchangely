import { apiGet } from "./client";
import type { SyncStatus, Ticker } from "../types/api";

export function fetchSyncStatus() {
  return apiGet<SyncStatus>("/system/sync-status");
}

export function fetchTicker(pair: string) {
  return apiGet<Ticker>(`/ticker/${pair}`);
}
