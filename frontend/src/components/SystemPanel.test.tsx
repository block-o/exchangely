import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SystemPanel } from "./SystemPanel";

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
    localStorage.clear();
    globalThis.EventSource = MockEventSource as any;
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
        const method = typeof input === "string" ? "GET" : input.method || "GET";

        if (url.includes("/system/version")) {
          return mockResponse({ api_version: "v1.0.0" });
        }

        if (url.includes("/system/warnings")) {
          if (method === "POST") {
            return Promise.resolve({ ok: true, json: async () => ({}) });
          }
          return mockResponse(warningsResponse);
        }

        if (url.includes("/tickers") && !url.includes("/tickers/stream")) {
          return mockResponse({ data: [] });
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

  it("dismisses warnings via the API and shows failed-task details on hover", async () => {
    render(<SystemPanel />);

    await waitFor(() => {
      expect(screen.getByText("System health degraded")).toBeInTheDocument();
      expect(screen.getByText("ETHEUR")).toBeInTheDocument();
    });

    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();

    const failedStatus = screen.getByLabelText("Failed: validator mismatch");
    fireEvent.mouseEnter(failedStatus);

    expect(screen.getByRole("tooltip")).toHaveTextContent("validator mismatch");

    fireEvent.click(screen.getByLabelText("Dismiss warning System health degraded"));
    await waitFor(() => {
      expect(screen.queryByText("System health degraded")).not.toBeInTheDocument();
    });

    expect(screen.queryByLabelText("Dismiss completed task Consolidation ETHEUR")).not.toBeInTheDocument();
    expect(screen.queryByText("Actions")).not.toBeInTheDocument();
  });

  it("renders warnings as a vertical list", async () => {
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
});
