import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  setAccessToken,
  getAccessToken,
  authFetch,
  refreshAccessToken,
} from "./client";

describe("API client auth", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAccessToken(null);
  });

  /**
   * Validates: Requirements 9.1
   * Access token is stored in memory, not localStorage.
   */
  describe("token management", () => {
    it("stores and retrieves access token in memory", () => {
      expect(getAccessToken()).toBeNull();
      setAccessToken("test-token");
      expect(getAccessToken()).toBe("test-token");
    });

    it("clears access token", () => {
      setAccessToken("test-token");
      setAccessToken(null);
      expect(getAccessToken()).toBeNull();
    });
  });

  /**
   * Validates: Requirements 9.4
   * authFetch injects Bearer header when token is present.
   */
  describe("Bearer injection", () => {
    it("injects Authorization header when token is set", async () => {
      setAccessToken("my-jwt");

      const mockFetch = vi.fn().mockResolvedValue({ status: 200 });
      vi.stubGlobal("fetch", mockFetch);

      await authFetch("http://localhost:8080/api/v1/auth/me");

      const [, options] = mockFetch.mock.calls[0];
      const headers = new Headers(options.headers);
      expect(headers.get("Authorization")).toBe("Bearer my-jwt");
    });

    it("does not inject Authorization header when no token", async () => {
      const mockFetch = vi.fn().mockResolvedValue({ status: 200 });
      vi.stubGlobal("fetch", mockFetch);

      await authFetch("http://localhost:8080/api/v1/auth/me");

      const [, options] = mockFetch.mock.calls[0];
      const headers = new Headers(options.headers);
      expect(headers.get("Authorization")).toBeNull();
    });
  });

  /**
   * Validates: Requirements 9.3
   * On 401, authFetch attempts one silent refresh then retries.
   */
  describe("401 → refresh → retry", () => {
    it("retries with new token after successful refresh", async () => {
      setAccessToken("expired-token");

      let callCount = 0;
      const mockFetch = vi.fn().mockImplementation(
        (url: string, options?: RequestInit) => {
          const urlStr = String(url);

          // Refresh endpoint
          if (urlStr.includes("/auth/refresh")) {
            return Promise.resolve({
              ok: true,
              status: 200,
              json: async () => ({ access_token: "fresh-token" }),
            });
          }

          // First call returns 401, retry should succeed
          callCount++;
          if (callCount === 1) {
            return Promise.resolve({ status: 401 });
          }
          return Promise.resolve({ status: 200 });
        },
      );
      vi.stubGlobal("fetch", mockFetch);

      const response = await authFetch("http://localhost:8080/api/v1/auth/me");

      expect(response.status).toBe(200);
      // Should have called: original request, refresh, retry
      expect(mockFetch).toHaveBeenCalledTimes(3);

      // After refresh, the token should be updated
      expect(getAccessToken()).toBe("fresh-token");
    });

    it("returns 401 when refresh fails", async () => {
      setAccessToken("expired-token");

      const mockFetch = vi.fn().mockImplementation(
        (url: string) => {
          const urlStr = String(url);

          if (urlStr.includes("/auth/refresh")) {
            return Promise.resolve({
              ok: false,
              status: 401,
            });
          }

          return Promise.resolve({ status: 401 });
        },
      );
      vi.stubGlobal("fetch", mockFetch);

      const response = await authFetch("http://localhost:8080/api/v1/auth/me");

      expect(response.status).toBe(401);
      // Token should be cleared after failed refresh
      expect(getAccessToken()).toBeNull();
    });
  });

  /**
   * Validates: Requirements 12.11
   * Auth state cleanup: setAccessToken(null) clears the token.
   */
  describe("auth state cleanup on logout", () => {
    it("clears token state completely", () => {
      setAccessToken("session-token");
      expect(getAccessToken()).toBe("session-token");

      // Simulate logout cleanup
      setAccessToken(null);
      expect(getAccessToken()).toBeNull();
    });
  });

  /**
   * Validates: Requirements 9.3
   * Concurrent refresh calls share a single in-flight promise.
   */
  describe("refresh deduplication", () => {
    it("deduplicates concurrent refresh calls", async () => {
      let refreshCallCount = 0;

      const mockFetch = vi.fn().mockImplementation((url: string) => {
        const urlStr = String(url);
        if (urlStr.includes("/auth/refresh")) {
          refreshCallCount++;
          return Promise.resolve({
            ok: true,
            status: 200,
            json: async () => ({ access_token: "deduped-token" }),
          });
        }
        return Promise.resolve({ status: 200 });
      });
      vi.stubGlobal("fetch", mockFetch);

      // Fire two concurrent refreshes
      const [result1, result2] = await Promise.all([
        refreshAccessToken(),
        refreshAccessToken(),
      ]);

      // Both should get the same token
      expect(result1).toBe("deduped-token");
      expect(result2).toBe("deduped-token");
      // Only one actual fetch to /auth/refresh
      expect(refreshCallCount).toBe(1);
    });
  });
});
