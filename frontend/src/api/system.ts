import { apiGet, apiPost } from "./client";
import type { ActiveWarning, SyncPairStatus, Ticker } from "../types/api";

export function fetchSyncStatus() {
  return apiGet<SyncPairStatus[]>("/system/sync-status");
}

export function fetchWarnings() {
  return apiGet<ActiveWarning[]>("/system/warnings");
}

export function dismissWarning(id: string, fingerprint: string) {
  return apiPost("/system/warnings", { id, fingerprint });
}

// Ticker endpoints expose the freshest persisted realtime point for a pair.
export function fetchTicker(pair: string) {
  return apiGet<Ticker>(`/ticker/${pair}`);
}
