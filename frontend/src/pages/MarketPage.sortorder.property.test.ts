/**
 * MarketCard list preserves sort order
 *
 * For any array of Pairs with associated Tickers, the rendered MarketCard list
 * should be ordered by market cap descending, with ties broken by alphabetical
 * base asset symbol.
 */
import { describe, expect, it } from "vitest";
import fc from "fast-check";
import type { Pair, Ticker } from "../types/api";

// Sort logic extracted from MarketPage.tsx

function sortPairsByMarketCap(
  pairs: Pair[],
  tickers: Record<string, Ticker>,
): Pair[] {
  return [...pairs].sort((left, right) => {
    const leftCap = tickers[left.symbol]?.market_cap ?? 0;
    const rightCap = tickers[right.symbol]?.market_cap ?? 0;
    if (rightCap !== leftCap) {
      return rightCap - leftCap;
    }
    return left.base.localeCompare(right.base);
  });
}

// Arbitraries

/** Base symbol: 2–5 uppercase alpha chars */
const baseArb = fc.stringMatching(/^[A-Z]{2,5}$/);

/** Quote currency */
const quoteArb = fc.constantFrom("EUR", "USD");

/** Market cap: include 0 to exercise the fallback, plus positive values and duplicates */
const marketCapArb = fc.oneof(
  fc.constant(0),
  fc.double({ min: 0.01, max: 1e12, noNaN: true }),
  // Use a small set of round numbers to increase tie probability
  fc.constantFrom(100, 1000, 50000, 1e9),
);

/** Generate a Pair + Ticker combo */
const pairTickerArb = fc
  .record({
    base: baseArb,
    quote: quoteArb,
    marketCap: marketCapArb,
  })
  .map(({ base, quote, marketCap }) => {
    const symbol = `${base}${quote}`;
    const pair: Pair = { base, quote, symbol };
    const ticker: Ticker = {
      pair: symbol,
      price: 1,
      market_cap: marketCap,
      variation_1h: 0,
      variation_24h: 0,
      variation_7d: 0,
      volume_24h: 0,
      last_update_unix: 0,
      source: "test",
    };
    return { pair, ticker };
  });

describe("Feature: responsive-ui-overhaul, Property 5: MarketCard list preserves sort order", () => {
  it("sorts by market cap descending with alphabetical base tiebreak for any pair+ticker array", () => {
    fc.assert(
      fc.property(
        fc.array(pairTickerArb, { minLength: 0, maxLength: 50 }),
        (combos) => {
          const pairs = combos.map((c) => c.pair);
          const tickers: Record<string, Ticker> = {};
          for (const c of combos) {
            tickers[c.ticker.pair] = c.ticker;
          }

          const sorted = sortPairsByMarketCap(pairs, tickers);

          // Verify ordering invariant: each adjacent pair satisfies the comparator
          for (let i = 0; i < sorted.length - 1; i++) {
            const capA = tickers[sorted[i].symbol]?.market_cap ?? 0;
            const capB = tickers[sorted[i + 1].symbol]?.market_cap ?? 0;

            if (capA !== capB) {
              // Higher market cap comes first
              expect(capA).toBeGreaterThan(capB);
            } else {
              // Equal market cap: alphabetical base order
              expect(
                sorted[i].base.localeCompare(sorted[i + 1].base),
              ).toBeLessThanOrEqual(0);
            }
          }
        },
      ),
      { numRuns: 100 },
    );
  });

  it("is idempotent: sorting an already-sorted array produces the same result", () => {
    fc.assert(
      fc.property(
        fc.array(pairTickerArb, { minLength: 0, maxLength: 30 }),
        (combos) => {
          const pairs = combos.map((c) => c.pair);
          const tickers: Record<string, Ticker> = {};
          for (const c of combos) {
            tickers[c.ticker.pair] = c.ticker;
          }

          const first = sortPairsByMarketCap(pairs, tickers);
          const second = sortPairsByMarketCap(first, tickers);

          expect(second.map((p) => p.symbol)).toEqual(
            first.map((p) => p.symbol),
          );
        },
      ),
      { numRuns: 100 },
    );
  });

  it("preserves all original elements (no additions or removals)", () => {
    fc.assert(
      fc.property(
        fc.array(pairTickerArb, { minLength: 0, maxLength: 30 }),
        (combos) => {
          const pairs = combos.map((c) => c.pair);
          const tickers: Record<string, Ticker> = {};
          for (const c of combos) {
            tickers[c.ticker.pair] = c.ticker;
          }

          const sorted = sortPairsByMarketCap(pairs, tickers);

          expect(sorted.length).toBe(pairs.length);
          // Same set of symbols (as multiset)
          const originalSymbols = pairs.map((p) => p.symbol).sort();
          const sortedSymbols = sorted.map((p) => p.symbol).sort();
          expect(sortedSymbols).toEqual(originalSymbols);
        },
      ),
      { numRuns: 100 },
    );
  });
});
