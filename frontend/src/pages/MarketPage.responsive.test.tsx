import { render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { MarketPage } from "./MarketPage";
import { SettingsProvider } from "../app/settings";
import * as pairsApi from "../api/pairs";
import * as historicalApi from "../api/historical";
import * as newsApi from "../api/news";

vi.mock("../api/pairs");
vi.mock("../api/system");
vi.mock("../api/news", () => ({ getNews: vi.fn() }));
vi.mock("../api/historical", () => ({ fetchHistorical: vi.fn() }));

type ChangeListener = (ev: MediaQueryListEvent) => void;

function createMockMediaQueryList(matches: boolean) {
  const listeners: ChangeListener[] = [];
  return {
    matches,
    media: "",
    onchange: null as ((ev: MediaQueryListEvent) => void) | null,
    addEventListener: vi.fn((_event: string, cb: ChangeListener) => {
      listeners.push(cb);
    }),
    removeEventListener: vi.fn((_event: string, cb: ChangeListener) => {
      const idx = listeners.indexOf(cb);
      if (idx >= 0) listeners.splice(idx, 1);
    }),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  };
}

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

function setupMatchMedia(mobileMatches: boolean, tabletMatches: boolean) {
  const mobileMql = createMockMediaQueryList(mobileMatches);
  const tabletMql = createMockMediaQueryList(tabletMatches);

  window.matchMedia = vi.fn((query: string) => {
    if (query === "(max-width: 639px)") return mobileMql as unknown as MediaQueryList;
    if (query === "(min-width: 640px) and (max-width: 1023px)") return tabletMql as unknown as MediaQueryList;
    return createMockMediaQueryList(false) as unknown as MediaQueryList;
  });
}

function setupMocks() {
  vi.mocked(pairsApi.fetchPairs).mockResolvedValue({
    data: [
      { symbol: "BTCEUR", base: "BTC", quote: "EUR" },
      { symbol: "ETHEUR", base: "ETH", quote: "EUR" },
    ],
  });
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
            ],
          }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({
          data: [
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
          ],
        }),
      });
    })
  );
}

function renderMarket() {
  return render(
    <SettingsProvider>
      <MarketPage />
    </SettingsProvider>
  );
}

describe("MarketPage responsive behavior", () => {
  const originalMatchMedia = window.matchMedia;

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    MockEventSource.instances = [];
    globalThis.EventSource = MockEventSource as any;
    setupMocks();
  });

  afterEach(() => {
    window.matchMedia = originalMatchMedia;
    vi.restoreAllMocks();
  });

  describe("at mobile breakpoint", () => {
    beforeEach(() => setupMatchMedia(true, false));

    it("renders MarketCard list instead of a table", async () => {
      const { container } = renderMarket();

      await waitFor(() => {
        expect(screen.getByText("Bitcoin")).toBeInTheDocument();
      });

      expect(container.querySelector(".market-card-list")).toBeInTheDocument();
      expect(container.querySelector(".data-table")).not.toBeInTheDocument();
    });

    it("renders one MarketCard per visible pair", async () => {
      const { container } = renderMarket();

      await waitFor(() => {
        expect(screen.getByText("Bitcoin")).toBeInTheDocument();
        expect(screen.getByText("Ethereum")).toBeInTheDocument();
      });

      const cards = container.querySelectorAll(".market-card");
      expect(cards.length).toBe(2);
    });

    it("MarketCard shows asset name, code, and price", async () => {
      renderMarket();

      await waitFor(() => {
        expect(screen.getByText("Bitcoin")).toBeInTheDocument();
        expect(screen.getByText("BTC")).toBeInTheDocument();
        expect(screen.getByText("€50,000")).toBeInTheDocument();
      });
    });

    it("renders sparklines at 80px width inside cards", async () => {
      const { container } = renderMarket();

      await waitFor(() => {
        expect(screen.getByText("Bitcoin")).toBeInTheDocument();
      });

      const sparklines = container.querySelectorAll(".market-card .chart-placeholder");
      sparklines.forEach((el) => {
        expect((el as HTMLElement).style.width).toBe("80px");
        expect((el as HTMLElement).style.height).toBe("32px");
      });
    });
  });

  describe("at tablet breakpoint", () => {
    beforeEach(() => setupMatchMedia(false, true));

    it("renders a table (not cards)", async () => {
      const { container } = renderMarket();

      await waitFor(() => {
        expect(screen.getByText("Bitcoin")).toBeInTheDocument();
      });

      expect(container.querySelector(".data-table")).toBeInTheDocument();
      expect(container.querySelector(".market-card-list")).not.toBeInTheDocument();
    });

    it("applies tablet-market-table class to hide columns", async () => {
      const { container } = renderMarket();

      await waitFor(() => {
        expect(screen.getByText("Bitcoin")).toBeInTheDocument();
      });

      const table = container.querySelector(".data-table");
      expect(table).toHaveClass("tablet-market-table");
    });

    it("table has col-1h, col-7d, col-high classes for CSS hiding", async () => {
      const { container } = renderMarket();

      await waitFor(() => {
        expect(screen.getByText("Bitcoin")).toBeInTheDocument();
      });

      expect(container.querySelector("th.col-1h")).toBeInTheDocument();
      expect(container.querySelector("th.col-7d")).toBeInTheDocument();
      expect(container.querySelector("th.col-high")).toBeInTheDocument();
    });
  });

  describe("at desktop breakpoint", () => {
    beforeEach(() => setupMatchMedia(false, false));

    it("renders the full table without tablet class", async () => {
      const { container } = renderMarket();

      await waitFor(() => {
        expect(screen.getByText("Bitcoin")).toBeInTheDocument();
      });

      const table = container.querySelector(".data-table");
      expect(table).toBeInTheDocument();
      expect(table).not.toHaveClass("tablet-market-table");
      expect(container.querySelector(".market-card-list")).not.toBeInTheDocument();
    });

    it("renders sparklines at 120px width in the table", async () => {
      const { container } = renderMarket();

      await waitFor(() => {
        expect(screen.getByText("Bitcoin")).toBeInTheDocument();
      });

      const sparklines = container.querySelectorAll(".chart-placeholder");
      sparklines.forEach((el) => {
        expect((el as HTMLElement).style.width).toBe("120px");
        expect((el as HTMLElement).style.height).toBe("40px");
      });
    });

    it("shows all table columns including 1h%, 7d%, High/Low", async () => {
      renderMarket();

      await waitFor(() => {
        expect(screen.getByText("1h %")).toBeInTheDocument();
        expect(screen.getByText("7d %")).toBeInTheDocument();
        expect(screen.getByText("24h High/Low")).toBeInTheDocument();
      });
    });
  });
});
