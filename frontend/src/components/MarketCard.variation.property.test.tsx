/**
 * Feature: responsive-ui-overhaul, Property 4: MarketCard variation color coding matches sign
 *
 * Validates: Requirements 3.5
 *
 * For any Ticker with variation_24h, variation_1h, and variation_7d values,
 * the MarketCard should apply the `text-up` CSS class when the value >= 0
 * and the `text-down` CSS class when the value < 0.
 */
import { render, cleanup } from "@testing-library/react";
import "@testing-library/jest-dom";
import { describe, expect, it } from "vitest";
import fc from "fast-check";
import { MarketCard } from "./MarketCard";
import type { Ticker, Pair } from "../types/api";

/** Variation value: any finite double including zero, positive, and negative */
const variationArb = fc.double({ min: -99, max: 99, noNaN: true });

/** Build a Ticker with the given variation values */
function makeTicker(v1h: number, v24h: number, v7d: number): Ticker {
  return {
    pair: "BTCEUR",
    price: 50000,
    market_cap: 1e12,
    variation_1h: v1h,
    variation_24h: v24h,
    variation_7d: v7d,
    volume_24h: 1e9,
    high_24h: 51000,
    low_24h: 49000,
    last_update_unix: Math.floor(Date.now() / 1000),
    source: "test",
  };
}

const defaultPair: Pair = { base: "BTC", quote: "EUR", symbol: "BTCEUR" };

function expectedClass(value: number): string {
  return value >= 0 ? "text-up" : "text-down";
}

function oppositeClass(value: number): string {
  return value >= 0 ? "text-down" : "text-up";
}

describe("Feature: responsive-ui-overhaul, Property 4: MarketCard variation color coding matches sign", () => {
  it("applies text-up for non-negative and text-down for negative variations across all three variation fields", () => {
    fc.assert(
      fc.property(
        variationArb,
        variationArb,
        variationArb,
        (v1h, v24h, v7d) => {
          cleanup();

          const ticker = makeTicker(v1h, v24h, v7d);

          const { container } = render(
            <MarketCard
              pair={defaultPair}
              ticker={ticker}
              assetName="Bitcoin"
              candles={[]}
              flashState={undefined}
              quoteCurrency="EUR"
            />,
          );

          // 24h variation: the <span> inside .market-card-change
          const changeSection = container.querySelector(".market-card-change");
          const v24hEl = changeSection?.querySelector("span");
          expect(v24hEl?.classList.contains(expectedClass(v24h))).toBe(true);
          expect(v24hEl?.classList.contains(oppositeClass(v24h))).toBe(false);

          // 1h and 7d variations: .value elements inside .market-card-secondary
          const secondary = container.querySelector(".market-card-secondary");
          const valueEls = secondary?.querySelectorAll(".value");

          // 1h% is the second grid item (index 1)
          const v1hEl = valueEls?.[1];
          expect(v1hEl?.classList.contains(expectedClass(v1h))).toBe(true);
          expect(v1hEl?.classList.contains(oppositeClass(v1h))).toBe(false);

          // 7d% is the third grid item (index 2)
          const v7dEl = valueEls?.[2];
          expect(v7dEl?.classList.contains(expectedClass(v7d))).toBe(true);
          expect(v7dEl?.classList.contains(oppositeClass(v7d))).toBe(false);
        },
      ),
      { numRuns: 100 },
    );
  });
});
