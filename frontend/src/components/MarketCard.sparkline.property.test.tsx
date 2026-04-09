/**
 * Feature: responsive-ui-overhaul, Property 6: Sparkline bars maintain minimum width
 *
 * Validates: Requirements 6.3
 *
 * For any array of candle data (1–24 candles), each rendered sparkline bar
 * element should have a CSS min-width of at least 2px.
 */
import { render, cleanup } from "@testing-library/react";
import "@testing-library/jest-dom";
import { describe, expect, it } from "vitest";
import fc from "fast-check";
import { MarketCard } from "./MarketCard";
import type { Candle, Pair, Ticker } from "../types/api";

/* ── Arbitraries ──────────────────────────────────────────── */

/** Positive float for OHLCV values */
const priceArb = fc.double({ min: 0.01, max: 1e6, noNaN: true });

/** Generate a single Candle with a given timestamp */
function candleArb(timestamp: number): fc.Arbitrary<Candle> {
  return fc
    .record({
      open: priceArb,
      high: priceArb,
      low: priceArb,
      close: priceArb,
      volume: fc.double({ min: 0, max: 1e9, noNaN: true }),
    })
    .map((c) => ({
      pair: "BTCEUR",
      interval: "1h",
      timestamp,
      open: c.open,
      high: Math.max(c.high, c.open, c.close),
      low: Math.min(c.low, c.open, c.close),
      close: c.close,
      volume: c.volume,
      source: "test",
      finalized: true,
    }));
}

/**
 * Generate an array of 1–24 candles with sequential hourly timestamps.
 * Timestamps are aligned to hour boundaries so the Sparkline bucketing logic
 * maps them correctly.
 */
const candlesArb = fc
  .integer({ min: 1, max: 24 })
  .chain((count) => {
    const baseHour = Math.floor(Date.now() / 1000 / 3600) * 3600;
    const arbs = Array.from({ length: count }, (_, i) =>
      candleArb(baseHour - (count - 1 - i) * 3600),
    );
    return fc.tuple(...(arbs as [fc.Arbitrary<Candle>, ...fc.Arbitrary<Candle>[]]));
  })
  .map((tuple) => [...tuple]);

/* ── Fixtures ─────────────────────────────────────────────── */

const fixedPair: Pair = { base: "BTC", quote: "EUR", symbol: "BTCEUR" };

const fixedTicker: Ticker = {
  pair: "BTCEUR",
  price: 50000,
  market_cap: 1e12,
  variation_1h: 0.5,
  variation_24h: 1.2,
  variation_7d: 3.0,
  volume_24h: 1e9,
  high_24h: 51000,
  low_24h: 49000,
  last_update_unix: Math.floor(Date.now() / 1000),
  source: "test",
};

/* ── Property Test ────────────────────────────────────────── */

describe("Feature: responsive-ui-overhaul, Property 6: Sparkline bars maintain minimum width", () => {
  it("every chart-bar has minWidth >= 2px for any candle array of 1–24 items", () => {
    fc.assert(
      fc.property(candlesArb, (candles) => {
        cleanup();

        const { container } = render(
          <MarketCard
            pair={fixedPair}
            ticker={fixedTicker}
            assetName="Bitcoin"
            candles={candles}
            flashState={undefined}
            quoteCurrency="EUR"
          />,
        );

        const bars = container.querySelectorAll(".chart-bar");

        // Sparkline always renders 24 buckets
        expect(bars.length).toBe(24);

        bars.forEach((bar) => {
          const style = (bar as HTMLElement).style;
          const minWidth = style.minWidth;
          expect(minWidth).toBe("2px");
        });
      }),
      { numRuns: 100 },
    );
  });
});
