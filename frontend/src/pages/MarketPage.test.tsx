import { render, screen, waitFor, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { MarketPage } from "./MarketPage";
import { SettingsProvider } from "../app/settings";
import * as pairsApi from "../api/pairs";
import * as historicalApi from "../api/historical";
import * as newsApi from "../api/news";

vi.mock("../api/pairs");
vi.mock("../api/system");
vi.mock("../api/news", () => ({ getNews: vi.fn() }));
vi.mock("../api/historical", () => ({ fetchHistorical: vi.fn() }));

class MockEventSource {
  static instances: MockEventSource[] = [];

  url: string;
  close = vi.fn();
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }
}

describe("MarketPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    MockEventSource.instances = [];
    vi.mocked(historicalApi.fetchHistorical).mockResolvedValue({ data: [] });
    vi.mocked(newsApi.getNews).mockResolvedValue([]);
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        if (url.includes("/assets")) {
          return Promise.resolve({
            ok: true,
            json: async () => ({
              data: [
                { symbol: "BTC", name: "Bitcoin", type: "crypto" },
                { symbol: "ETH", name: "Ethereum", type: "crypto" },
                { symbol: "EUR", name: "Euro", type: "fiat" },
                { symbol: "USD", name: "US Dollar", type: "fiat" },
              ],
            }),
          });
        }
        return Promise.resolve({
          ok: true,
          json: async () => ({ data: [] }),
        });
      })
    );
    
    // Mock EventSource to avoid real HTTP requests and unhandled message loops which cause unmounted act() errors
    globalThis.EventSource = MockEventSource as any;
  });

  it("filters pairs by quote currency and displays asset names with currency symbols", async () => {
    vi.mocked(pairsApi.fetchPairs).mockResolvedValue({
      data: [
        { symbol: "ETHEUR", base: "ETH", quote: "EUR" },
        { symbol: "BTCEUR", base: "BTC", quote: "EUR" },
        { symbol: "BTCUSD", base: "BTC", quote: "USD" },
      ]
    });
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        if (url.includes("/assets")) {
          return Promise.resolve({
            ok: true,
            json: async () => ({
              data: [
                { symbol: "BTC", name: "Bitcoin", type: "crypto" },
                { symbol: "ETH", name: "Ethereum", type: "crypto" },
                { symbol: "EUR", name: "Euro", type: "fiat" },
              ],
            }),
          });
        }
        return Promise.resolve({
          ok: true,
          json: async () => ({
            data: [
              {
                pair: "ETHEUR",
                 price: 2500,
                 market_cap: 301_750_000_000,
                 variation_1h: 0.2,
                 variation_24h: 0.8,
                 variation_7d: 5.0,
                 volume_24h: 1500000,
                 high_24h: 2550,
                 low_24h: 2450,
                 source: "mock",
                 last_update_unix: 1711929600,
               },
               {
                 pair: "BTCEUR",
                 price: 50000,
                 market_cap: 992_500_000_000,
                 variation_1h: -0.5,
                 variation_24h: 1.5,
                 variation_7d: -2.0,
                 volume_24h: 1200000,
                 high_24h: 51000,
                 low_24h: 49000,
                 source: "mock",
                 last_update_unix: 1711929600,
               },
            ],
          }),
        });
      })
    );

    const { container } = render(
      <SettingsProvider>
        <MarketPage />
      </SettingsProvider>
    );

     // Assert EUR headers and base symbols output
     await waitFor(() => {
       expect(screen.getByText("Market Cap")).toBeInTheDocument();
       expect(screen.getByText("Price")).toBeInTheDocument();
       expect(screen.getByText("1h %")).toBeInTheDocument();
       expect(screen.getByText("7d %")).toBeInTheDocument();
       expect(screen.getByText(/24h Vol/)).toBeInTheDocument();
       expect(screen.getByText("Bitcoin")).toBeInTheDocument();
       expect(screen.getByText("Ethereum")).toBeInTheDocument();
       expect(screen.getByText("BTC")).toBeInTheDocument();
       expect(screen.getByText("ETH")).toBeInTheDocument();
       expect(screen.getByText("€992.5B")).toBeInTheDocument();
       expect(screen.getByText("€301.8B")).toBeInTheDocument();
       expect(screen.getByText("€50,000")).toBeInTheDocument();
       expect(screen.getByText("+0.20%")).toBeInTheDocument();
       expect(screen.getByText("-0.50%")).toBeInTheDocument();
       expect(screen.getByText("€1.2M")).toBeInTheDocument();
       expect(screen.getByText("€1.5M")).toBeInTheDocument();
     });

    const assetCells = Array.from(container.querySelectorAll("tbody tr td.symbol")).map((cell) => cell.textContent);
    expect(assetCells).toEqual(["BitcoinBTC", "EthereumETH"]);

    // USD pair should not be displayed while EUR is selected
    expect(screen.queryByText("BTCUSD")).not.toBeInTheDocument();
    
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
      expect(screen.getByText("Bitcoin")).toBeInTheDocument();
    });

    const chartPlaceholder = container.querySelector('.chart-placeholder');
    expect(chartPlaceholder).toBeInTheDocument();
    const bars = chartPlaceholder?.querySelectorAll('.chart-bar');
    expect(bars?.length).toBe(24);
    
    // verify they are empty bars
    const emptyBars = chartPlaceholder?.querySelectorAll('.chart-bar.empty');
    expect(emptyBars?.length).toBe(24);
  });

  it("accepts ticker SSE payloads wrapped in the backend tickers envelope", async () => {
    vi.mocked(pairsApi.fetchPairs).mockResolvedValue({
      data: [{ symbol: "BTCEUR", base: "BTC", quote: "EUR" }]
    });
    vi.mocked(historicalApi.fetchHistorical).mockResolvedValue({ data: [] });
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        if (url.includes("/assets")) {
          return Promise.resolve({
            ok: true,
            json: async () => ({
              data: [{ symbol: "BTC", name: "Bitcoin", type: "crypto" }],
            }),
          });
        }
        return Promise.resolve({
          ok: true,
          json: async () => ({
            data: [{
              pair: "BTCEUR",
              price: 50000,
              market_cap: 992_500_000_000,
              variation_24h: 1.5,
              high_24h: 51000,
              low_24h: 49000,
              source: "mock",
              last_update_unix: 1711929600,
            }],
          }),
        });
      })
    );

    render(
      <SettingsProvider>
        <MarketPage />
      </SettingsProvider>
    );

    await waitFor(() => {
      expect(screen.getByText("€50,000")).toBeInTheDocument();
      expect(screen.getByText(/Live/i)).toBeInTheDocument();
      expect(screen.getByText(/Last Updated: -/i)).toBeInTheDocument();
    });

    const stream = MockEventSource.instances.find(i => i.url.includes("/tickers/stream"));
    if (!stream?.onmessage) {
      throw new Error("expected EventSource onmessage handler to be registered");
    }

    const nowSpy = vi.spyOn(Date, "now").mockReturnValue(1711933200000);

    await act(async () => {
      stream.onmessage!(
        new MessageEvent("message", {
          data: JSON.stringify({
            tickers: [{
              pair: "BTCEUR",
              price: 50100,
              market_cap: 994_485_000_000,
              variation_1h: -0.4,
              variation_24h: 1.8,
              variation_7d: -1.9,
              volume_24h: 1250000,
              high_24h: 51100,
              low_24h: 49100,
              source: "stream",
              last_update_unix: 1711933200,
            }],
          }),
        })
      );
    });

    await waitFor(() => {
      expect(screen.getByText("€50,100")).toBeInTheDocument();
      expect(screen.getByText("€1.3M")).toBeInTheDocument();
      expect(screen.getByText(/Last Updated:/i)).toBeInTheDocument();
      expect(screen.queryByText(/Last Updated: -/i)).not.toBeInTheDocument();
      expect(screen.queryByText(/Source Updated:/i)).not.toBeInTheDocument();
    });

    nowSpy.mockRestore();
  });

  it("updates footer stream status across connect, disconnect, and reconnect events", async () => {
    vi.mocked(pairsApi.fetchPairs).mockResolvedValue({
      data: [{ symbol: "BTCEUR", base: "BTC", quote: "EUR" }]
    });
    vi.mocked(historicalApi.fetchHistorical).mockResolvedValue({ data: [] });
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        if (url.includes("/assets")) {
          return Promise.resolve({
            ok: true,
            json: async () => ({
              data: [{ symbol: "BTC", name: "Bitcoin", type: "crypto" }],
            }),
          });
        }
        return Promise.resolve({
          ok: true,
          json: async () => ({
            data: [{
              pair: "BTCEUR",
              price: 50000,
              market_cap: 992_500_000_000,
              variation_24h: 1.5,
              high_24h: 51000,
              low_24h: 49000,
              source: "mock",
              last_update_unix: 1711929600,
            }],
          }),
        });
      })
    );

    const { container } = render(
      <SettingsProvider>
        <MarketPage />
      </SettingsProvider>
    );

    await waitFor(() => {
      expect(screen.getByText("€50,000")).toBeInTheDocument();
    });

    const stream = MockEventSource.instances.find(i => i.url.includes("/tickers/stream"));
    const status = () => container.querySelector(".market-stream-status");
    if (!stream?.onopen || !stream.onerror) {
      throw new Error("expected EventSource handlers to be registered");
    }

    expect(status()).toHaveClass("is-offline");

    await act(async () => {
      stream.onopen!(new Event("open"));
    });

    await waitFor(() => {
      expect(status()).toHaveClass("is-live");
    });

    await act(async () => {
      stream.onerror!(new Event("error"));
    });

    await waitFor(() => {
      expect(status()).toHaveClass("is-offline");
    });

    await act(async () => {
      stream.onopen!(new Event("open"));
    });

    await waitFor(() => {
      expect(status()).toHaveClass("is-live");
    });
  });

  it("refreshes sparkline candles on a periodic interval", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });

    vi.mocked(pairsApi.fetchPairs).mockResolvedValue({
      data: [{ symbol: "BTCEUR", base: "BTC", quote: "EUR" }],
    });

    let fetchCount = 0;
    vi.mocked(historicalApi.fetchHistorical).mockImplementation(async () => {
      fetchCount++;
      return { data: [] };
    });

    render(
      <SettingsProvider>
        <MarketPage />
      </SettingsProvider>
    );

    // Wait for initial load (first fetchHistorical call)
    await waitFor(() => {
      expect(fetchCount).toBeGreaterThanOrEqual(1);
    });

    const initialCount = fetchCount;

    // Advance past the 5-minute refresh interval
    await act(async () => {
      vi.advanceTimersByTime(5 * 60 * 1000 + 100);
    });

    await waitFor(() => {
      expect(fetchCount).toBeGreaterThan(initialCount);
    });

    vi.useRealTimers();
  });

  it("uses dynamic scaling for sparkline bars in the table view", async () => {
    const now = Math.floor(Date.now() / 1000);
    const currentHour = Math.floor(now / 3600) * 3600;
    // Build 24 candles with a clear upward trend (60000 → 61150)
    const sparkCandles = Array.from({ length: 24 }, (_, i) => ({
      pair: "BTCEUR",
      interval: "1h",
      timestamp: currentHour - (23 - i) * 3600,
      open: 60000 + i * 50 - 10,
      high: 60000 + i * 50 + 50,
      low: 60000 + i * 50 - 50,
      close: 60000 + i * 50,
      volume: 100,
      source: "binance",
      finalized: true,
    }));

    vi.mocked(pairsApi.fetchPairs).mockResolvedValue({
      data: [{ symbol: "BTCEUR", base: "BTC", quote: "EUR" }],
    });
    vi.mocked(historicalApi.fetchHistorical).mockResolvedValue({ data: sparkCandles });
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        if (url.includes("/assets")) {
          return Promise.resolve({
            ok: true,
            json: async () => ({
              data: [{ symbol: "BTC", name: "Bitcoin", type: "crypto" }],
            }),
          });
        }
        return Promise.resolve({
          ok: true,
          json: async () => ({
            data: [{
              pair: "BTCEUR",
              price: 61150,
              market_cap: 1_200_000_000_000,
              variation_1h: 0.1,
              variation_24h: 1.9,
              variation_7d: 3.0,
              volume_24h: 500000,
              high_24h: 61200,
              low_24h: 60000,
              source: "mock",
              last_update_unix: now,
            }],
          }),
        });
      })
    );

    const { container } = render(
      <SettingsProvider>
        <MarketPage />
      </SettingsProvider>
    );

    await waitFor(() => {
      expect(screen.getByText("Bitcoin")).toBeInTheDocument();
    });

    // Wait for candles to load and bars to render
    await waitFor(() => {
      const bars = container.querySelectorAll<HTMLElement>(".chart-bar:not(.empty)");
      expect(bars.length).toBe(24);
    });

    const bars = container.querySelectorAll<HTMLElement>(".chart-bar:not(.empty)");
    const heights = Array.from(bars).map((b) => parseFloat(b.style.height));
    const first = heights[0];
    const last = heights[heights.length - 1];
    // Dynamic scaling should produce visible height differences for a trending market
    expect(last).toBeGreaterThan(first + 10);
  });

  it("renders the news ticker above the market panel", async () => {
    vi.mocked(pairsApi.fetchPairs).mockResolvedValue({
      data: [{ symbol: "BTCEUR", base: "BTC", quote: "EUR" }],
    });
    vi.mocked(newsApi.getNews).mockResolvedValue([
      {
        id: "news-1",
        title: "Bitcoin surges past 100k",
        link: "https://example.com/1",
        source: "CoinDesk",
        published_at: new Date().toISOString(),
      },
    ]);

    const { container } = render(
      <SettingsProvider>
        <MarketPage />
      </SettingsProvider>
    );

    await waitFor(() => {
      expect(screen.getByText("Latest News")).toBeInTheDocument();
      expect(screen.getAllByText("Bitcoin surges past 100k")[0]).toBeInTheDocument();
    });

    // News ticker container must clip its scrolling content
    const tickerContainer = container.querySelector(".news-ticker-container");
    expect(tickerContainer).toBeInTheDocument();

    // The news ticker should appear before the market panel in DOM order
    const marketPanel = container.querySelector("#market");
    expect(marketPanel).toBeInTheDocument();
    if (tickerContainer && marketPanel) {
      const position = tickerContainer.compareDocumentPosition(marketPanel);
      // Node.DOCUMENT_POSITION_FOLLOWING = 4 means marketPanel comes after tickerContainer
      expect(position & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    }
  });
});
