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

  close = vi.fn();
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;

  constructor(_url: string) {
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
       expect(screen.getByText("24h Vol")).toBeInTheDocument();
       expect(screen.getByText("Bitcoin")).toBeInTheDocument();
       expect(screen.getByText("Ethereum")).toBeInTheDocument();
       expect(screen.getByText("BTC")).toBeInTheDocument();
       expect(screen.getByText("ETH")).toBeInTheDocument();
       expect(screen.getByText("€992.5B")).toBeInTheDocument();
       expect(screen.getByText("€301.8B")).toBeInTheDocument();
       expect(screen.getByText("€50,000")).toBeInTheDocument();
       expect(screen.getByText("+0.2%")).toBeInTheDocument();
       expect(screen.getByText("-0.5%")).toBeInTheDocument();
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

    const stream = MockEventSource.instances[0];
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

    const stream = MockEventSource.instances[0];
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
});
