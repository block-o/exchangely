import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { describe, expect, it } from "vitest";
import { MarketCard, MarketCardProps } from "./MarketCard";
import type { Pair, Ticker, Candle } from "../types/api";

/* ── Fixtures ─────────────────────────────────────────────── */

const pair: Pair = { base: "BTC", quote: "EUR", symbol: "BTCEUR" };

const ticker: Ticker = {
  pair: "BTCEUR",
  price: 67432,
  market_cap: 1_300_000_000_000,
  variation_1h: 0.3,
  variation_24h: 2.4,
  variation_7d: 5.1,
  volume_24h: 28_500_000_000,
  high_24h: 68100,
  low_24h: 66200,
  last_update_unix: 1711929600,
  source: "binance",
};

const now = Math.floor(Date.now() / 1000);
const candles: Candle[] = Array.from({ length: 3 }, (_, i) => ({
  pair: "BTCEUR",
  interval: "1h",
  timestamp: now - (2 - i) * 3600,
  open: 67000 + i * 100,
  high: 67500 + i * 100,
  low: 66800 + i * 100,
  close: 67200 + i * 100,
  volume: 500 + i * 10,
  source: "binance",
  finalized: true,
}));

const defaultProps: MarketCardProps = {
  pair,
  ticker,
  assetName: "Bitcoin",
  candles,
  flashState: undefined,
  quoteCurrency: "EUR",
};

function renderCard(overrides: Partial<MarketCardProps> = {}) {
  return render(<MarketCard {...defaultProps} {...overrides} />);
}

/* ── Tests ─────────────────────────────────────────────────── */

describe("MarketCard", () => {
  describe("renders all required fields (Req 3.3, 3.4)", () => {
    it("renders asset name and code", () => {
      renderCard();
      expect(screen.getByText("Bitcoin")).toBeInTheDocument();
      expect(screen.getByText("BTC")).toBeInTheDocument();
    });

    it("renders formatted price", () => {
      renderCard();
      expect(screen.getByText("€67,432")).toBeInTheDocument();
    });

    it("renders 24h variation", () => {
      renderCard();
      expect(screen.getByText("+2.40%")).toBeInTheDocument();
    });

    it("renders 1h variation", () => {
      renderCard();
      expect(screen.getByText("+0.30%")).toBeInTheDocument();
    });

    it("renders 7d variation", () => {
      renderCard();
      expect(screen.getByText("+5.10%")).toBeInTheDocument();
    });

    it("renders 24h volume", () => {
      renderCard();
      expect(screen.getByText("€28.5B")).toBeInTheDocument();
    });

    it("renders market cap", () => {
      renderCard();
      expect(screen.getByText("€1.3T")).toBeInTheDocument();
    });

    it("renders 24h high", () => {
      renderCard();
      expect(screen.getByText("€68,100")).toBeInTheDocument();
    });

    it("renders 24h low", () => {
      renderCard();
      expect(screen.getByText("€66,200")).toBeInTheDocument();
    });

    it("renders dash when ticker is undefined", () => {
      const { container } = renderCard({ ticker: undefined });
      // Price, variations, volume, high, low should all show "-"
      const dashes = container.querySelectorAll(".value, .price");
      const dashTexts = Array.from(dashes)
        .map((el) => el.textContent?.trim())
        .filter((t) => t === "-");
      expect(dashTexts.length).toBeGreaterThanOrEqual(1);
    });
  });

  describe("flash class application (Req 3.6)", () => {
    it("applies flash-up class when flashState is up", () => {
      const { container } = renderCard({ flashState: "up" });
      expect(container.querySelector(".market-card")).toHaveClass("flash-up");
    });

    it("applies flash-down class when flashState is down", () => {
      const { container } = renderCard({ flashState: "down" });
      expect(container.querySelector(".market-card")).toHaveClass("flash-down");
    });

    it("applies no flash class when flashState is undefined", () => {
      const { container } = renderCard({ flashState: undefined });
      const card = container.querySelector(".market-card")!;
      expect(card).not.toHaveClass("flash-up");
      expect(card).not.toHaveClass("flash-down");
    });
  });

  describe("variation color coding (Req 3.5)", () => {
    it("applies text-up class for positive variations", () => {
      const { container } = renderCard({
        ticker: { ...ticker, variation_1h: 1.5, variation_24h: 3.2, variation_7d: 7.0 },
      });
      const upElements = container.querySelectorAll(".text-up");
      // 24h change row + 1h + 7d in secondary grid = 3
      expect(upElements.length).toBe(3);
    });

    it("applies text-down class for negative variations", () => {
      const { container } = renderCard({
        ticker: { ...ticker, variation_1h: -1.5, variation_24h: -3.2, variation_7d: -7.0 },
      });
      const downElements = container.querySelectorAll(".text-down");
      expect(downElements.length).toBe(3);
    });

    it("applies text-up for zero variation (>= 0 is positive)", () => {
      const { container } = renderCard({
        ticker: { ...ticker, variation_1h: 0, variation_24h: 0, variation_7d: 0 },
      });
      const upElements = container.querySelectorAll(".text-up");
      expect(upElements.length).toBe(3);
      const downElements = container.querySelectorAll(".text-down");
      expect(downElements.length).toBe(0);
    });

    it("applies mixed classes for mixed positive/negative variations", () => {
      const { container } = renderCard({
        ticker: { ...ticker, variation_1h: 0.5, variation_24h: -1.2, variation_7d: 3.0 },
      });
      const upElements = container.querySelectorAll(".text-up");
      const downElements = container.querySelectorAll(".text-down");
      // 1h up, 24h down, 7d up → 2 up, 1 down
      expect(upElements.length).toBe(2);
      expect(downElements.length).toBe(1);
    });
  });

  describe("sparkline dynamic scaling", () => {
    /** Build 24 consecutive hourly candles ending at `anchorUnix`. */
    function buildCandles(anchorUnix: number, closes: number[]): Candle[] {
      const currentHour = Math.floor(anchorUnix / 3600) * 3600;
      return closes.map((close, i) => ({
        pair: "BTCEUR",
        interval: "1h",
        timestamp: currentHour - (closes.length - 1 - i) * 3600,
        open: close - 10,
        high: close + 50,
        low: close - 50,
        close,
        volume: 100,
        source: "binance",
        finalized: true,
      }));
    }

    it("produces bars with varying heights when candle closes differ", () => {
      const anchor = Math.floor(Date.now() / 1000);
      // Simulate a clear upward trend: 60000 → 61200 over 24 hours (2% move)
      const closes = Array.from({ length: 24 }, (_, i) => 60000 + i * 50);
      const { container } = renderCard({ candles: buildCandles(anchor, closes) });

      const bars = container.querySelectorAll<HTMLElement>(".chart-bar:not(.empty)");
      expect(bars.length).toBe(24);

      const heights = Array.from(bars).map((b) => parseFloat(b.style.height));
      const first = heights[0];
      const last = heights[heights.length - 1];
      // With dynamic scaling the last bar should be noticeably taller than the first
      expect(last).toBeGreaterThan(first + 10);
    });

    it("renders all bars at similar height when closes are identical (flat market)", () => {
      const anchor = Math.floor(Date.now() / 1000);
      const closes = Array.from({ length: 24 }, () => 60000);
      const { container } = renderCard({ candles: buildCandles(anchor, closes) });

      const bars = container.querySelectorAll<HTMLElement>(".chart-bar:not(.empty)");
      expect(bars.length).toBe(24);

      const heights = Array.from(bars).map((b) => parseFloat(b.style.height));
      const unique = new Set(heights.map((h) => Math.round(h)));
      // All bars should be roughly the same height (50 % midpoint)
      expect(unique.size).toBeLessThanOrEqual(2);
    });

    it("renders empty placeholder bars when no candles are provided", () => {
      const { container } = renderCard({ candles: [] });
      const emptyBars = container.querySelectorAll(".chart-bar.empty");
      expect(emptyBars.length).toBe(24);
    });
  });
});
