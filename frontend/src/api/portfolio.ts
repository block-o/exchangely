import {
  API_BASE_URL,
  authGet,
  authPost,
  authFetch,
  authEventSource,
} from "./client";

// Types matching backend JSON responses

export interface Holding {
  id: string;
  user_id: string;
  asset_symbol: string;
  quantity: number;
  avg_buy_price?: number;
  quote_currency: string;
  source: string;
  source_ref?: string;
  notes?: string;
  created_at: string;
  updated_at: string;
}

export interface ExchangeCredential {
  id: string;
  user_id: string;
  exchange: string;
  api_key_prefix: string;
  status: string;
  error_reason?: string;
  last_sync_at?: string;
  created_at: string;
  updated_at: string;
}

export interface WalletAddress {
  id: string;
  user_id: string;
  chain: string;
  address_prefix: string;
  label?: string;
  last_sync_at?: string;
  created_at: string;
  updated_at: string;
}

export interface LedgerCredential {
  id: string;
  user_id: string;
  last_sync_at?: string;
  created_at: string;
  updated_at: string;
}

export interface AssetValuation {
  asset_symbol: string;
  quantity: number;
  current_price: number;
  current_value: number;
  allocation_pct: number;
  avg_buy_price?: number;
  unrealized_pnl?: number;
  unrealized_pnl_pct?: number;
  priced: boolean;
  source: string;
}

export interface Valuation {
  total_value: number;
  quote_currency: string;
  assets: AssetValuation[];
  updated_at: string;
}

export interface HistoricalPoint {
  timestamp: number;
  value: number;
}

interface ListResponse<T> {
  data: T[];
}

export interface CreateHoldingRequest {
  asset_symbol: string;
  quantity: number;
  avg_buy_price?: number;
  quote_currency?: string;
  notes?: string;
}

export interface UpdateHoldingRequest {
  asset_symbol: string;
  quantity: number;
  avg_buy_price?: number;
  quote_currency?: string;
  notes?: string;
}

export interface CreateCredentialRequest {
  exchange: string;
  api_key: string;
  api_secret: string;
}

export interface CreateWalletRequest {
  chain: string;
  address: string;
  label?: string;
}

export interface ConnectLedgerRequest {
  token: string;
}

// Holdings

export async function getHoldings(): Promise<Holding[]> {
  const res = await authGet<ListResponse<Holding>>("/portfolio/holdings");
  return res.data;
}

export function createHolding(req: CreateHoldingRequest): Promise<Holding> {
  return authPost<Holding>("/portfolio/holdings", req);
}

export async function updateHolding(id: string, req: UpdateHoldingRequest): Promise<Holding> {
  const response = await authFetch(`${API_BASE_URL}/portfolio/holdings/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: JSON.stringify(req),
  });
  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
  return response.json() as Promise<Holding>;
}

export async function deleteHolding(id: string): Promise<void> {
  const response = await authFetch(
    `${API_BASE_URL}/portfolio/holdings/${id}`,
    { method: "DELETE", credentials: "include" },
  );
  if (!response.ok && response.status !== 204) {
    throw new Error(`request failed: ${response.status}`);
  }
}

// Valuation

export function getValuation(quote?: string): Promise<Valuation> {
  const params = quote ? `?quote=${encodeURIComponent(quote)}` : "";
  return authGet<Valuation>(`/portfolio/valuation${params}`);
}

export async function getHistory(range?: string, quote?: string): Promise<HistoricalPoint[]> {
  const params = new URLSearchParams();
  if (range) params.set("range", range);
  if (quote) params.set("quote", quote);
  const qs = params.toString();
  const res = await authGet<ListResponse<HistoricalPoint>>(
    `/portfolio/history${qs ? `?${qs}` : ""}`,
  );
  return res.data;
}

// Exchange credentials

export async function getCredentials(): Promise<ExchangeCredential[]> {
  const res = await authGet<ListResponse<ExchangeCredential>>("/portfolio/credentials");
  return res.data;
}

export async function createCredential(req: CreateCredentialRequest): Promise<ExchangeCredential> {
  const response = await authFetch(`${API_BASE_URL}/portfolio/credentials`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: JSON.stringify(req),
  });
  if (!response.ok) {
    if (response.status === 409) {
      throw new Error(`A ${req.exchange} connection already exists. Remove the existing one first.`);
    }
    throw new Error(`request failed: ${response.status}`);
  }
  return response.json() as Promise<ExchangeCredential>;
}

export async function deleteCredential(id: string): Promise<void> {
  const response = await authFetch(
    `${API_BASE_URL}/portfolio/credentials/${id}`,
    { method: "DELETE", credentials: "include" },
  );
  if (!response.ok && response.status !== 204) {
    throw new Error(`request failed: ${response.status}`);
  }
}

export function syncCredential(id: string): Promise<void> {
  return authPost(`/portfolio/credentials/${id}/sync`);
}

// Wallets

export async function getWallets(): Promise<WalletAddress[]> {
  const res = await authGet<ListResponse<WalletAddress>>("/portfolio/wallets");
  return res.data;
}

export function createWallet(req: CreateWalletRequest): Promise<WalletAddress> {
  return authPost<WalletAddress>("/portfolio/wallets", req);
}

export async function deleteWallet(id: string): Promise<void> {
  const response = await authFetch(
    `${API_BASE_URL}/portfolio/wallets/${id}`,
    { method: "DELETE", credentials: "include" },
  );
  if (!response.ok && response.status !== 204) {
    throw new Error(`request failed: ${response.status}`);
  }
}

export function syncWallet(id: string): Promise<void> {
  return authPost(`/portfolio/wallets/${id}/sync`);
}

// Ledger

export async function uploadLedgerExport(file: File): Promise<{ imported: number }> {
  const formData = new FormData();
  formData.append("file", file);
  const response = await authFetch(`${API_BASE_URL}/portfolio/ledger/connect`, {
    method: "POST",
    credentials: "include",
    body: formData,
  });
  if (!response.ok) throw new Error(`request failed: ${response.status}`);
  return response.json();
}

export async function disconnectLedger(): Promise<void> {
  const response = await authFetch(
    `${API_BASE_URL}/portfolio/ledger`,
    { method: "DELETE", credentials: "include" },
  );
  if (!response.ok && response.status !== 204) {
    throw new Error(`request failed: ${response.status}`);
  }
}

// SSE stream

export function subscribePortfolioStream(): EventSource {
  return authEventSource(`${API_BASE_URL}/portfolio/stream`);
}

// Sync all sources

export interface SyncAllResult {
  exchanges_synced: number;
  wallets_synced: number;
  errors: string[];
}

export async function syncAll(): Promise<SyncAllResult> {
  return authPost<SyncAllResult>("/portfolio/sync-all");
}
