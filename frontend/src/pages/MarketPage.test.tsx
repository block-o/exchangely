import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { MarketPage } from "./MarketPage";
import { SettingsProvider } from "../app/settings";
import * as pairsApi from "../api/pairs";
import * as systemApi from "../api/system";
import * as historicalApi from "../api/historical";

vi.mock("../api/pairs");
vi.mock("../api/system");
vi.mock("../api/historical", () => ({ fetchHistorical: vi.fn() }));

describe("MarketPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    vi.mocked(historicalApi.fetchHistorical).mockResolvedValue({ data: [] });
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ data: [] }),
      })
    );
    
    // Mock EventSource to avoid real HTTP requests and unhandled message loops which cause unmounted act() errors
    class MockEventSource {
      close = vi.fn();
      onmessage: ((event: MessageEvent) => void) | null = null;
      onerror: ((event: Event) => void) | null = null;
    }
    globalThis.EventSource = MockEventSource as any;
  });

  it("filters pairs by quote currency and displays base", async () => {
    vi.mocked(pairsApi.fetchPairs).mockResolvedValue({
      data: [
        { symbol: "BTCEUR", base: "BTC", quote: "EUR" },
        { symbol: "BTCUSDT", base: "BTC", quote: "USDT" },
        { symbol: "ETHEUR", base: "ETH", quote: "EUR" },
      ]
    });
    vi.mocked(systemApi.fetchTicker).mockResolvedValue({
      pair: "BTCEUR", price: 50000, variation_24h: 1.5, source: "mock", last_update_unix: Date.now() / 1000
    });

    const { container } = render(
      <SettingsProvider>
        <MarketPage />
      </SettingsProvider>
    );

    // Assert EUR headers and base symbols output
    await waitFor(() => {
      expect(screen.getByText("Price (EUR)")).toBeInTheDocument();
      expect(screen.getByText("BTC")).toBeInTheDocument();
      expect(screen.getByText("ETH")).toBeInTheDocument();
    });

    // USDT pair should not be displayed
    expect(screen.queryByText("BTCUSDT")).not.toBeInTheDocument();
    
    // Wait for the chart to render 24-item arrays so the background async fetchHistorical resolves
    // before the test tears down (which prevents the React act() "Should not already be working" error)
    await waitFor(() => {
      expect(container.querySelectorAll('.chart-bar').length).toBeGreaterThan(0);
    });
  });

  it("renders 24 trend bars per asset including empty placeholders", async () => {
    vi.mocked(pairsApi.fetchPairs).mockResolvedValue({
      data: [{ symbol: "BTCEUR", base: "BTC", quote: "EUR" }]
    });
    
    // fetchHistorical is mocked to return `{ data: [] }`, so all 24 should be empty
    const { container } = render(
      <SettingsProvider>
        <MarketPage />
      </SettingsProvider>
    );

    await waitFor(() => {
      expect(screen.getByText("BTC")).toBeInTheDocument();
    });

    const chartPlaceholder = container.querySelector('.chart-placeholder');
    expect(chartPlaceholder).toBeInTheDocument();
    const bars = chartPlaceholder?.querySelectorAll('.chart-bar');
    expect(bars?.length).toBe(24);
    
    // verify they are empty bars
    const emptyBars = chartPlaceholder?.querySelectorAll('.chart-bar.empty');
    expect(emptyBars?.length).toBe(24);
  });
});
