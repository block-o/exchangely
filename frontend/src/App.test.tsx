import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";

class MockEventSource {
  close = vi.fn();
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;

  constructor(_url: string) {}
}

function mockResponse(data: unknown) {
  return Promise.resolve({
    ok: true,
    json: async () => data,
  });
}

describe("App", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.location.hash = "";
    localStorage.clear();
    globalThis.EventSource = MockEventSource as any;

    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        if (url.includes("/pairs")) {
          return mockResponse({
            data: [{ symbol: "BTCEUR", base: "BTC", quote: "EUR" }],
          });
        }
        if (url.includes("/assets")) {
          return mockResponse({
            data: [
              { symbol: "BTC", name: "Bitcoin", type: "crypto" },
              { symbol: "EUR", name: "Euro", type: "fiat" },
            ],
          });
        }
        if (url.includes("/tickers")) {
          return mockResponse({
            data: [{
              pair: "BTCEUR",
              price: 50000,
              market_cap: 992500000000,
              variation_24h: 1.5,
              high_24h: 51000,
              low_24h: 49000,
              source: "mock",
              last_update_unix: 1711929600,
            }],
          });
        }
        if (url.includes("/historical/")) {
          return mockResponse({ data: [] });
        }
        if (url.includes("/system/version")) {
          return mockResponse({ api_version: "v1.0.0" });
        }
        if (url.includes("/system/sync-status")) {
          return mockResponse([]);
        }
        if (url.includes("/health")) {
          return mockResponse({
            status: "ok",
            checks: { api: "ok", db: "ok", kafka: "ok" },
            timestamp: 1711929600,
          });
        }
        if (url.includes("/system/tasks")) {
          return mockResponse({
            upcoming: [],
            upcomingTotal: 0,
            upcomingLimit: 10,
            upcomingPage: 1,
            recent: [],
            recentTotal: 0,
            recentLimit: 10,
            recentPage: 1,
          });
        }
        return Promise.reject(new Error(`Unhandled fetch URL: ${url}`));
      })
    );
  });

  it("renders the market page by default", async () => {
    render(<App />);

    await waitFor(() => {
      expect(screen.getByText("Market Overview")).toBeInTheDocument();
      expect(screen.getByText("Price")).toBeInTheDocument();
      expect(screen.getByText("Bitcoin")).toBeInTheDocument();
    });
  });

  it("renders the operations page when the hash targets system", async () => {
    window.location.hash = "#system";

    render(<App />);

    await waitFor(() => {
      expect(screen.getByText("System Operations")).toBeInTheDocument();
      expect(screen.getByText("Active Warnings")).toBeInTheDocument();
    });
  });
});
