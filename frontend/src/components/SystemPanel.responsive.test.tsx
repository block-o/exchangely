import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
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

describe("SystemPanel responsive behavior", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    MockEventSource.instances = [];
    globalThis.EventSource = MockEventSource as any;
    window.history.replaceState({}, "", "/");

    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        const method = typeof input === "string" ? "GET" : (input as Request).method || "GET";
        if (url.includes("/system/version")) return mockResponse({ api_version: "v1.0.0" });
        if (url.includes("/system/warnings")) {
          if (method === "POST") return Promise.resolve({ ok: true, json: async () => ({}) });
          return mockResponse([]);
        }
        if (url.includes("/system/sync-status")) return mockResponse([]);
        if (url.includes("/pairs") && !url.includes("/pairs/"))
          return mockResponse({ data: [] });
        if (url.includes("/tickers") && !url.includes("/tickers/stream"))
          return mockResponse({ data: [] });
        if (url.includes("/system/tasks") && !url.includes("/system/tasks/stream"))
          return mockResponse({
            upcoming: [], upcomingTotal: 0, upcomingLimit: 10, upcomingPage: 1,
            recent: [], recentTotal: 0, recentLimit: 10, recentPage: 1,
          });
        return Promise.reject(new Error(`Unhandled fetch URL: ${url}`));
      })
    );
  });

  it("tab bar uses toggle-group class for consistent pill styling", () => {
    const { container } = render(<SystemPanel />);
    const tabBar = container.querySelector("[role='tablist']");
    expect(tabBar).toHaveClass("toggle-group");
  });

  it("active tab button has the active class", async () => {
    render(<SystemPanel />);

    await waitFor(() => {
      const overviewTab = screen.getByRole("tab", { name: "Overview" });
      expect(overviewTab).toHaveClass("active");
    });
  });

  it("inactive tab buttons do not have the active class", () => {
    render(<SystemPanel />);
    const coverageTab = screen.getByRole("tab", { name: "Coverage" });
    const auditTab = screen.getByRole("tab", { name: "Audit" });
    expect(coverageTab).not.toHaveClass("active");
    expect(auditTab).not.toHaveClass("active");
  });

  it("switching tabs updates the active class", async () => {
    render(<SystemPanel />);

    const coverageTab = screen.getByRole("tab", { name: "Coverage" });
    fireEvent.click(coverageTab);

    await waitFor(() => {
      expect(coverageTab).toHaveClass("active");
      expect(screen.getByRole("tab", { name: "Overview" })).not.toHaveClass("active");
      expect(screen.getByRole("tab", { name: "Audit" })).not.toHaveClass("active");
    });
  });

  it("tab buttons do not have inline background or border styles", () => {
    render(<SystemPanel />);
    const tabs = screen.getAllByRole("tab");
    tabs.forEach((tab) => {
      // Inline styles should not override the CSS toggle-group styling
      expect((tab as HTMLElement).style.background).toBe("");
      expect((tab as HTMLElement).style.border).toBe("");
      expect((tab as HTMLElement).style.borderBottom).toBe("");
    });
  });

  it("tab bar does not have inline display or borderBottom styles", () => {
    const { container } = render(<SystemPanel />);
    const tabBar = container.querySelector("[role='tablist']") as HTMLElement;
    // Only marginTop and marginBottom should be inline
    expect(tabBar.style.display).toBe("");
    expect(tabBar.style.borderBottom).toBe("");
    expect(tabBar.style.paddingBottom).toBe("");
  });
});
