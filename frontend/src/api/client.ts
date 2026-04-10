export const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api/v1";

// --- Existing unauthenticated helpers (unchanged) ---

export async function apiGet<T>(path: string): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`);
  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
  return response.json() as Promise<T>;
}

export async function apiPost(path: string, body?: unknown): Promise<void> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
}

// --- In-memory access token management ---

let accessToken: string | null = null;

export function setAccessToken(token: string | null): void {
  accessToken = token;
}

export function getAccessToken(): string | null {
  return accessToken;
}

// --- Silent refresh with single in-flight promise ---

let refreshPromise: Promise<string | null> | null = null;

async function silentRefresh(): Promise<string | null> {
  const response = await fetch(`${API_BASE_URL}/auth/refresh`, {
    method: "POST",
    credentials: "include",
  });
  if (!response.ok) {
    setAccessToken(null);
    return null;
  }
  const data = (await response.json()) as { access_token: string };
  setAccessToken(data.access_token);
  return data.access_token;
}

/**
 * Attempt a single in-flight token refresh. Concurrent callers share the same
 * promise so only one refresh request is made at a time.
 */
export function refreshAccessToken(): Promise<string | null> {
  if (!refreshPromise) {
    refreshPromise = silentRefresh().finally(() => {
      refreshPromise = null;
    });
  }
  return refreshPromise;
}

// --- Authenticated fetch wrapper ---

/**
 * Wrapper around `fetch` that:
 * 1. Injects `Authorization: Bearer <token>` when a token is held in memory.
 * 2. On a 401 response, attempts one silent refresh and retries the request.
 * 3. If the refresh fails, clears the token and returns the original 401 response.
 */
export async function authFetch(
  url: string,
  options: RequestInit = {},
): Promise<Response> {
  const doFetch = (token: string | null): Promise<Response> => {
    const headers = new Headers(options.headers);
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }
    return fetch(url, { ...options, headers });
  };

  const response = await doFetch(accessToken);

  if (response.status === 401) {
    const newToken = await refreshAccessToken();
    if (newToken) {
      return doFetch(newToken);
    }
    // Refresh failed — return the original 401 so callers can handle it.
    return response;
  }

  return response;
}

// --- Authenticated convenience helpers ---

export async function authGet<T>(path: string): Promise<T> {
  const response = await authFetch(`${API_BASE_URL}${path}`, {
    credentials: "include",
  });
  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
  return response.json() as Promise<T>;
}

export async function authPost<T = void>(
  path: string,
  body?: unknown,
): Promise<T> {
  const response = await authFetch(`${API_BASE_URL}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
  // Return undefined for void responses (empty body).
  const text = await response.text();
  if (!text) {
    return undefined as T;
  }
  return JSON.parse(text) as T;
}
