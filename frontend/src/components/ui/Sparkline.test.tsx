import { render } from '@testing-library/react';
import fc from 'fast-check';
import { describe, it, expect } from 'vitest';
import { Sparkline, type SparklineCandle } from './Sparkline';

// Feature: frontend-design-system, Property 8: Sparkline bar direction matches candle close vs open
// Feature: frontend-design-system, Property 9: Sparkline auto-scales bar heights relative to data range

// Base timestamp for generating candles within a 24-hour window.
// Candles are spaced 3600s apart so each maps to a unique hourly bucket.
const BASE_TS = Math.floor(Date.now() / 1000);
const HOUR = 3600;

function makeCandleAtSlot(
  slot: number,
  open: number,
  close: number,
): SparklineCandle {
  return {
    timestamp: BASE_TS + slot * HOUR,
    open,
    high: Math.max(open, close) + 1,
    low: Math.min(open, close) - 1,
    close,
    volume: 100,
  };
}

// Arbitrary that produces 1-24 candles with unique hourly slots
const candleArrayArb = fc
  .array(
    fc.record({
      slot: fc.integer({ min: 0, max: 23 }),
      open: fc.double({ min: 1, max: 10000, noNaN: true }),
      close: fc.double({ min: 1, max: 10000, noNaN: true }),
    }),
    { minLength: 1, maxLength: 24 },
  )
  .filter((arr) => new Set(arr.map((c) => c.slot)).size === arr.length)
  .map((arr) =>
    arr
      .sort((a, b) => a.slot - b.slot)
      .map((c) => makeCandleAtSlot(c.slot, c.open, c.close)),
  );

describe('Sparkline', () => {
  // **Validates: Requirements 13.3**
  it('bar direction matches candle close vs open for any candle array', () => {
    fc.assert(
      fc.property(candleArrayArb, (candles) => {
        const { container } = render(<Sparkline candles={candles} />);

        const bars = container.querySelectorAll('.chart-bar:not(.empty)');
        expect(bars.length).toBe(candles.length);

        bars.forEach((bar, i) => {
          const c = candles[i];
          if (c.close >= c.open) {
            expect(bar.className).toContain('up');
            expect(bar.className).not.toContain('down');
          } else {
            expect(bar.className).toContain('down');
            expect(bar.className).not.toContain('up');
          }
        });
      }),
      { numRuns: 100 },
    );
  });

  // **Validates: Requirements 13.5**
  it('auto-scales bar heights so higher close produces taller bar', () => {
    fc.assert(
      fc.property(
        fc
          .double({ min: 10, max: 5000, noNaN: true })
          .chain((baseClose) =>
            fc
              .double({ min: 0.01, max: baseClose * 0.5, noNaN: true })
              .map((delta) => ({ baseClose, delta })),
          ),
        ({ baseClose, delta }) => {
          // Three candles: reference (slot 0), lower close (slot 1), higher close (slot 2)
          const refCandle = makeCandleAtSlot(0, baseClose, baseClose);
          const lowerCandle = makeCandleAtSlot(1, baseClose, baseClose - delta);
          const higherCandle = makeCandleAtSlot(2, baseClose, baseClose + delta);

          const { container } = render(
            <Sparkline candles={[refCandle, lowerCandle, higherCandle]} />,
          );

          const bars = container.querySelectorAll('.chart-bar:not(.empty)');
          expect(bars.length).toBe(3);

          const heightOf = (el: Element) =>
            parseFloat((el as HTMLElement).style.height);

          const lowerHeight = heightOf(bars[1]);
          const higherHeight = heightOf(bars[2]);

          expect(higherHeight).toBeGreaterThan(lowerHeight);
        },
      ),
      { numRuns: 100 },
    );
  });
});
