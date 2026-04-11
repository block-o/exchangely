/**
 * Feature: responsive-ui-overhaul, Property 3: MarketCard renders all required fields
 *
 * Validates: Requirements 3.3, 3.4
 *
 * For any valid Ticker object with non-zero values, the MarketCard component
 * should render text content containing: the asset name, asset code, formatted
 * price, 24h percentage change, market cap, 1h percentage change, 7d percentage
 * change, 24h volume, 24h high, and 24h low.
 */
import { render, cleanup } from "@testing-library/react";
import "@testing-library/jest-dom";
import { describe, expect, it } from "vitest";
import fc from "fast-check";
import { MarketCard } from "./MarketCard";
import type { Ticker, Pair } from "../types/api";
import {
  formatCurrencyNumber,
  formatCompactCurrencyNumber,
  formatVariation,
} from "../lib/format";

/* ── Arbitraries ──────────────────────────────────────────── */

/** Asset name: 2–12 alpha chars starting with uppercase */
const assetNameArb = fc.stringMatching(/^[A-Z][a-z]{1,11}$/);

/** Asset code: 2–5 uppercase alpha chars */
const assetCodeArb = fc.stringMatching(/^[A-Z]{2,5}$/);

/** Quote currency picked from known symbols so formatting is predictable */
const quoteCurrencyArb = fc.constantFrom("EUR", "USD", "GBP");

/** Positive float for fields that must be > 0 (market_cap, volume, high, low, price) */
const posFloatArb = fc.double({ min: 0.01, max: 1e12, noNaN: true });

/** Non-zero float for variation fields (can be positive or negative) */
const variationArb = fc.oneof(
  fc.double({ min: 0.01, max: 99, noNaN: true }),
  fc.double({ min: -99, max: -0.01, noNaN: true }),
);


/** Generate a valid Ticker with all non-zero fields */
const tickerArb = fc
  .record({
    price: posFloatArb,
    market_cap: posFloatArb,
    variation_1h: variationArb,
    variation_24h: variationArb,
    variation_7d: variationArb,
    volume_24h: posFloatArb,
    high_24h: posFloatArb,
    low_24h: posFloatArb,
  })
  .map(
    (t): Ticker => ({
      pair: "TESTEUR",
      ...t,
      last_update_unix: Math.floor(Date.now() / 1000),
      source: "test",
    }),
  );

/* ── Property Test ────────────────────────────────────────── */

describe("Feature: responsive-ui-overhaul, Property 3: MarketCard renders all required fields", () => {
  it("renders asset name, code, price, all variations, market cap, volume, high, and low for any valid ticker", () => {
    fc.assert(
      fc.property(
        assetNameArb,
        assetCodeArb,
        quoteCurrencyArb,
        tickerArb,
        (assetName, assetCode, quoteCurrency, ticker) => {
          cleanup();

          const pair: Pair = {
            base: assetCode,
            quote: quoteCurrency,
            symbol: `${assetCode}${quoteCurrency}`,
          };

          const { container } = render(
            <MarketCard
              pair={pair}
              ticker={ticker}
              assetName={assetName}
              candles={[]}
              flashState={undefined}
              quoteCurrency={quoteCurrency}
            />,
          );

          const text = container.textContent ?? "";

          // Asset name and code
          expect(text).toContain(assetName);
          expect(text).toContain(assetCode);

          // Formatted price
          expect(text).toContain(
            formatCurrencyNumber(ticker.price, quoteCurrency),
          );

          // Variation percentages (24h, 1h, 7d)
          expect(text).toContain(formatVariation(ticker.variation_24h));
          expect(text).toContain(formatVariation(ticker.variation_1h));
          expect(text).toContain(formatVariation(ticker.variation_7d));

          // Market cap (compact format)
          expect(text).toContain(
            formatCompactCurrencyNumber(ticker.market_cap, quoteCurrency),
          );

          // 24h volume (compact format)
          expect(text).toContain(
            formatCompactCurrencyNumber(ticker.volume_24h, quoteCurrency),
          );

          // 24h high and low
          expect(text).toContain(
            formatCurrencyNumber(ticker.high_24h, quoteCurrency),
          );
          expect(text).toContain(
            formatCurrencyNumber(ticker.low_24h, quoteCurrency),
          );
        },
      ),
      { numRuns: 100 },
    );
  });
});
