import { apiGet, authGet, authPost } from "./client";
import type { ActiveWarning, SyncPairStatus, Ticker } from "../types/api";

export function fetchSyncStatus() {
  return authGet<SyncPairStatus[]>("/system/sync-status");
}

export function fetchWarnings() {
  return authGet<ActiveWarning[]>("/system/warnings");
}

export function dismissWarning(id: string, fingerprint: string) {
  return authPost("/system/warnings", { id, fingerprint });
}

// Ticker endpoints expose the freshest persisted realtime point for a pair.
export function fetchTicker(pair: string) {
  return apiGet<Ticker>(`/ticker/${pair}`);
}
