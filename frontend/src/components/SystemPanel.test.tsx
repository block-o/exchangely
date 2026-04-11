import { fireEvent, render, screen, waitFor, act } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SystemPanel } from "./SystemPanel";

class MockEventSource {
  static instances: MockEventSource[] = [];

  url: string;
  close = vi.fn();
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }
}

function mockResponse(data: unknown) {
  return Promise.resolve({
    ok: true,
    json: async () => data,
  });
}

describe("SystemPanel", () => {
  let warningsResponse: Array<{
    id: string;
    level: "warning" | "error";
    title: string;
    detail: string;
    fingerprint: string;
  }>;

  beforeEach(() => {
    vi.clearAllMocks();
    vi.useRealTimers();
    localStorage.clear();
    MockEventSource.instances = [];
    globalThis.EventSource = MockEventSource as any;

    // Reset URL to avoid tab param leaking between tests
    window.history.replaceState({}, "", "/");

    warningsResponse = [
      {
        id: "system-health",
        level: "error",
        title: "System health degraded",
        detail: "Failing checks: kafka.",
        fingerprint: "warning-fingerprint-1",
      },
    ];

    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        const method = typeof input === "string" ? "GET" : (input as Request).method || "GET";

        if (url.includes("/config")) {
          return mockResponse({ auth_enabled: false, auth_methods: { google: false, local: false }, version: "v1.0.0" });
        }

        if (url.includes("/system/warnings")) {
          if (method === "POST") {
            return Promise.resolve({ ok: true, json: async () => ({}) });
          }
          return mockResponse(warningsResponse);
        }

        if (url.includes("/system/sync-status")) {
          return mockResponse([
            {
              pair: "BTCEUR",
              hourly_backfill_completed: true,
              daily_backfill_completed: false,
              hourly_synced_unix: 1700000000,
              daily_synced_unix: 0,
            },
            {
              pair: "BTCUSD",
              hourly_backfill_completed: true,
              daily_backfill_completed: true,
              hourly_synced_unix: 1700000000,
              daily_synced_unix: 1700000000,
            },
          ]);
        }

        if (url.includes("/pairs") && !url.includes("/pairs/")) {
          return mockResponse({
            data: [
              { base: "BTC", quote: "EUR", symbol: "BTCEUR" },
              { base: "BTC", quote: "USD", symbol: "BTCUSD" },
            ],
          });
        }

        if (url.includes("/tickers") && !url.includes("/tickers/stream")) {
          return mockResponse({
            data: [
              {
                pair: "BTCEUR",
                price: 82431,
                last_update_unix: Math.floor(Date.now() / 1000),
                source: "kraken",
              },
              {
                pair: "BTCUSD",
                price: 91204,
                last_update_unix: Math.floor(Date.now() / 1000),
                source: "binance",
              },
            ],
          });
        }

        if (url.includes("/system/tasks") && !url.includes("/system/tasks/stream")) {
          return mockResponse({
            upcoming: [],
            upcomingTotal: 0,
            upcomingLimit: 10,
            upcomingPage: 1,
            recent: [
              {
                id: "completed-task",
                type: "consolidation",
                pair: "ETHEUR",
                interval: "1d",
                window_start: "2026-04-01T00:00:00Z",
                window_end: "2026-04-02T00:00:00Z",
                status: "completed",
                completed_at: "2026-04-02T00:00:00Z",
              },
              {
                id: "failed-task",
                type: "integrity_check",
                pair: "BTCEUR",
                interval: "1h",
                window_start: "2026-04-02T00:00:00Z",
                window_end: "2026-04-02T01:00:00Z",
                status: "failed",
                last_error: "validator mismatch",
                completed_at: "2026-04-02T01:00:00Z",
              },
            ],
            recentTotal: 2,
            recentLimit: 10,
            recentPage: 1,
          });
        }

        return Promise.reject(new Error(`Unhandled fetch URL: ${url}`));
      })
    );
  });

  it("renders tab bar with three tabs and defaults to Overview", async () => {
    render(<SystemPanel />);

    expect(screen.getByRole("tab", { name: "Overview" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Coverage" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Audit" })).toBeInTheDocument();

    // Overview tab is active by default — warnings should load
    await waitFor(() => {
      expect(screen.getByText("System health degraded")).toBeInTheDocument();
    });
  });

  it("dismisses warnings on the Overview tab", async () => {
    render(<SystemPanel />);

    await waitFor(() => {
      expect(screen.getByText("System health degraded")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText("Dismiss warning System health degraded"));
    await waitFor(() => {
      expect(screen.queryByText("System health degraded")).not.toBeInTheDocument();
    });
  });

  it("renders warnings as a vertical list on Overview", async () => {
    warningsResponse = [
      {
        id: "system-health",
        level: "error",
        title: "System health degraded",
        detail: "Failing checks: kafka.",
        fingerprint: "warning-fingerprint-1",
      },
      {
        id: "hourly-backfill",
        level: "warning",
        title: "Hourly backfill pending",
        detail: "4 pairs are still filling hourly history.",
        fingerprint: "warning-fingerprint-2",
      },
      {
        id: "daily-backfill",
        level: "warning",
        title: "Daily backfill pending",
        detail: "2 pairs are not ready for consolidation yet.",
        fingerprint: "warning-fingerprint-3",
      },
    ];

    const { container } = render(<SystemPanel />);

    await waitFor(() => {
      expect(screen.getByText("3 active")).toBeInTheDocument();
    });

    const warningCards = container.querySelectorAll("article");
    expect(warningCards).toHaveLength(3);

    const warningRail = warningCards[0]?.parentElement;
    expect(warningRail).not.toBeNull();
    expect(warningRail).toHaveStyle({
      display: "flex",
      flexDirection: "column",
    });
  });

  it("switches to Coverage tab and shows coin-grouped data", async () => {
    render(<SystemPanel />);

    fireEvent.click(screen.getByRole("tab", { name: "Coverage" }));

    await waitFor(() => {
      // BTC group should appear with both EUR and USD quotes
      expect(screen.getByText("BTC")).toBeInTheDocument();
    });

    // The card should auto-expand and show quote rows
    await waitFor(() => {
      expect(screen.getByText("EUR")).toBeInTheDocument();
      expect(screen.getByText("USD")).toBeInTheDocument();
    });
  });

  it("switches to Audit tab and shows task log with failure details", async () => {
    render(<SystemPanel />);

    fireEvent.click(screen.getByRole("tab", { name: "Audit" }));

    await waitFor(() => {
      // The log viewer renders each task as a single line containing the pair and error
      const logViewer = screen.getByRole("log");
      expect(logViewer).toBeInTheDocument();
      expect(logViewer.textContent).toContain("ETHEUR");
      expect(logViewer.textContent).toContain("ERROR");
      expect(logViewer.textContent).toContain("validator mismatch");
    });
  });

  it("syncs active tab to URL query param", () => {
    render(<SystemPanel />);

    fireEvent.click(screen.getByRole("tab", { name: "Coverage" }));
    expect(window.location.search).toContain("tab=Coverage");

    fireEvent.click(screen.getByRole("tab", { name: "Audit" }));
    expect(window.location.search).toContain("tab=Audit");
  });

  it("shows feeds as live when bootstrap tickers have recent last_update_unix", async () => {
    render(<SystemPanel />);
    fireEvent.click(screen.getByRole("tab", { name: "Coverage" }));

    // Wait for auto-expand to reveal feed badges (first 5 coins auto-expand)
    await waitFor(() => {
      expect(screen.getByText("BTC")).toBeInTheDocument();
    });

    // Ensure the card is expanded (auto-expand should handle it, click if not)
    await waitFor(() => {
      const card = screen.getByLabelText("BTC coverage details");
      expect(card.getAttribute("aria-expanded")).toBe("true");
    });

    await waitFor(() => {
      // Both tickers have last_update_unix = now, so both should show Live
      const liveBadges = screen.getAllByText("● Live");
      expect(liveBadges.length).toBe(2);
      expect(screen.queryByText("● Stale")).not.toBeInTheDocument();
    });
  });

  it("shows feeds as stale when no updates arrive within threshold", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const bootstrapTime = Date.now();
    const oldUnix = Math.floor(bootstrapTime / 1000) - 600;

    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        const method = typeof input === "string" ? "GET" : (input as Request).method || "GET";
        if (url.includes("/config")) return mockResponse({ auth_enabled: false, auth_methods: { google: false, local: false }, version: "v1.0.0" });
        if (url.includes("/system/warnings")) {
          if (method === "POST") return Promise.resolve({ ok: true, json: async () => ({}) });
          return mockResponse([]);
        }
        if (url.includes("/system/sync-status")) return mockResponse([]);
        if (url.includes("/pairs") && !url.includes("/pairs/"))
          return mockResponse({ data: [{ base: "BTC", quote: "EUR", symbol: "BTCEUR" }] });
        if (url.includes("/tickers") && !url.includes("/tickers/stream"))
          return mockResponse({
            data: [{ pair: "BTCEUR", price: 82000, last_update_unix: oldUnix, source: "kraken" }],
          });
        if (url.includes("/system/tasks") && !url.includes("/system/tasks/stream"))
          return mockResponse({ upcoming: [], upcomingTotal: 0, upcomingLimit: 10, upcomingPage: 1, recent: [], recentTotal: 0, recentLimit: 10, recentPage: 1 });
        return Promise.reject(new Error(`Unhandled fetch URL: ${url}`));
      })
    );

    render(<SystemPanel />);
    fireEvent.click(screen.getByRole("tab", { name: "Coverage" }));

    await waitFor(() => {
      expect(screen.getByText("● Live")).toBeInTheDocument();
    });

    // Advance past the 5-minute threshold + trigger the 30s staleness re-evaluation timer
    await act(async () => {
      vi.advanceTimersByTime(310_000);
    });

    await waitFor(() => {
      expect(screen.getByText("● Stale")).toBeInTheDocument();
    });
    expect(screen.queryByText("● Live")).not.toBeInTheDocument();

    vi.useRealTimers();
  });

  it("transitions feed from stale to live when SSE delta arrives", async () => {
    const bootstrapTime = 1700000000000;
    const oldUnix = Math.floor(bootstrapTime / 1000) - 600;
    const nowSpy = vi.spyOn(Date, "now").mockReturnValue(bootstrapTime);

    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        const method = typeof input === "string" ? "GET" : (input as Request).method || "GET";
        if (url.includes("/config")) return mockResponse({ auth_enabled: false, auth_methods: { google: false, local: false }, version: "v1.0.0" });
        if (url.includes("/system/warnings")) {
          if (method === "POST") return Promise.resolve({ ok: true, json: async () => ({}) });
          return mockResponse([]);
        }
        if (url.includes("/system/sync-status")) return mockResponse([]);
        if (url.includes("/pairs") && !url.includes("/pairs/"))
          return mockResponse({ data: [{ base: "BTC", quote: "EUR", symbol: "BTCEUR" }] });
        if (url.includes("/tickers") && !url.includes("/tickers/stream"))
          return mockResponse({
            data: [{ pair: "BTCEUR", price: 82000, last_update_unix: oldUnix, source: "kraken" }],
          });
        if (url.includes("/system/tasks") && !url.includes("/system/tasks/stream"))
          return mockResponse({ upcoming: [], upcomingTotal: 0, upcomingLimit: 10, upcomingPage: 1, recent: [], recentTotal: 0, recentLimit: 10, recentPage: 1 });
        return Promise.reject(new Error(`Unhandled fetch URL: ${url}`));
      })
    );

    render(<SystemPanel />);
    fireEvent.click(screen.getByRole("tab", { name: "Coverage" }));

    await waitFor(() => {
      expect(screen.getByText("BTC")).toBeInTheDocument();
    });

    // Advance time so bootstrap lastSeenUnix becomes stale, send delta with old timestamp
    const futureTime = bootstrapTime + 310_000;
    nowSpy.mockReturnValue(futureTime);

    const tickerStream = MockEventSource.instances.find((i) => i.url.includes("/tickers/stream"));

    await act(async () => {
      tickerStream!.onmessage!(
        new MessageEvent("message", {
          data: JSON.stringify({
            tickers: [{ pair: "BTCEUR", price: 82001, last_update_unix: oldUnix, source: "kraken" }],
          }),
        })
      );
    });

    // The delta sets lastSeenUnix = futureTime, and isHealthy checks max(oldUnix, futureTime/1000).
    // futureTime/1000 - futureTime/1000 = 0, so it should be Live because the SSE delta just arrived.
    // This proves that lastSeenUnix (browser reception time) keeps the feed alive even with old backend timestamps.
    await waitFor(() => {
      expect(screen.getByText("● Live")).toBeInTheDocument();
    });

    // Now advance time again past the threshold without any new SSE
    nowSpy.mockReturnValue(futureTime + 310_000);

    // Send another delta to trigger re-render, but with old data
    await act(async () => {
      tickerStream!.onmessage!(
        new MessageEvent("message", {
          data: JSON.stringify({
            tickers: [{ pair: "BTCEUR", price: 82002, last_update_unix: oldUnix, source: "kraken" }],
          }),
        })
      );
    });

    // This delta sets lastSeenUnix = futureTime + 310_000, which is now() so delta = 0 → Live
    // The key insight: as long as SSE deltas keep arriving, the feed stays Live regardless of backend timestamp
    await waitFor(() => {
      expect(screen.getByText("● Live")).toBeInTheDocument();
    });

    nowSpy.mockRestore();
  });

  it("uses browser-side lastSeenUnix for health even when backend timestamp is old", async () => {
    // Bootstrap with old backend timestamp — but since bootstrap sets lastSeenUnix = now,
    // the feed should still show as Live
    const veryOldUnix = Math.floor(Date.now() / 1000) - 3600; // 1 hour ago
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        const method = typeof input === "string" ? "GET" : (input as Request).method || "GET";
        if (url.includes("/config")) return mockResponse({ auth_enabled: false, auth_methods: { google: false, local: false }, version: "v1.0.0" });
        if (url.includes("/system/warnings")) {
          if (method === "POST") return Promise.resolve({ ok: true, json: async () => ({}) });
          return mockResponse([]);
        }
        if (url.includes("/system/sync-status")) return mockResponse([]);
        if (url.includes("/pairs") && !url.includes("/pairs/"))
          return mockResponse({ data: [{ base: "BTC", quote: "EUR", symbol: "BTCEUR" }] });
        if (url.includes("/tickers") && !url.includes("/tickers/stream"))
          return mockResponse({
            data: [{ pair: "BTCEUR", price: 82000, last_update_unix: veryOldUnix, source: "kraken" }],
          });
        if (url.includes("/system/tasks") && !url.includes("/system/tasks/stream"))
          return mockResponse({ upcoming: [], upcomingTotal: 0, upcomingLimit: 10, upcomingPage: 1, recent: [], recentTotal: 0, recentLimit: 10, recentPage: 1 });
        return Promise.reject(new Error(`Unhandled fetch URL: ${url}`));
      })
    );

    render(<SystemPanel />);
    fireEvent.click(screen.getByRole("tab", { name: "Coverage" }));

    // Even though backend last_update_unix is 1 hour old, bootstrap sets lastSeenUnix = now
    // so the feed should show as Live
    await waitFor(() => {
      expect(screen.getByText("● Live")).toBeInTheDocument();
    });
    expect(screen.queryByText("● Stale")).not.toBeInTheDocument();
  });

  it("shows summary counts reflecting live and backfill status", async () => {
    render(<SystemPanel />);
    fireEvent.click(screen.getByRole("tab", { name: "Coverage" }));

    await waitFor(() => {
      expect(screen.getByText("2 pairs across 1 coins")).toBeInTheDocument();
      // "2/2 live" appears in both the summary bar and the BTC card header
      expect(screen.getAllByText("2/2 live")).toHaveLength(2);
      expect(screen.getByText("1/2 fully backfilled")).toBeInTheDocument();
    });
  });
});
