import type { ReactElement } from 'react';
import './Sparkline.css';

export type SparklineCandle = {
  timestamp: number;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
};

export type SparklineProps = {
  candles: SparklineCandle[];
  width?: number;
  height?: number;
};

const MIN_TREND_SCALE_PCT = 0.5;
const BAR_COUNT = 24;

export function Sparkline({ candles, width = 120, height = 40 }: SparklineProps): ReactElement {
  const latestCandleUnix =
    candles.length > 0
      ? candles[candles.length - 1].timestamp
      : Math.floor(Date.now() / 1000);
  const currentHourUnix = Math.floor(latestCandleUnix / 3600) * 3600;

  const plotted = Array.from({ length: BAR_COUNT }).map((_, i) => {
    const bucketUnix = currentHourUnix - (BAR_COUNT - 1 - i) * 3600;
    return candles.find(
      (c) => c.timestamp >= bucketUnix && c.timestamp < bucketUnix + 3600,
    );
  });

  const validCandles = plotted.filter((c): c is SparklineCandle => !!c);
  const referenceClose = validCandles.length > 0 ? validCandles[0].close : 0;

  let trendScale = MIN_TREND_SCALE_PCT;
  if (referenceClose > 0 && validCandles.length > 1) {
    const maxPct = validCandles.reduce(
      (mx, cc) =>
        Math.max(mx, Math.abs(((cc.close - referenceClose) / referenceClose) * 100)),
      0,
    );
    if (maxPct > trendScale) trendScale = maxPct * 1.1;
  }

  return (
    <div
      className="chart-placeholder"
      style={{ width: `${width}px`, height: `${height}px` }}
    >
      {plotted.map((c, i) => {
        if (!c) {
          return (
            <div
              key={`empty-${i}`}
              className="chart-bar empty"
              style={{ height: '30%', backgroundColor: '#374151', opacity: 1 }}
              title="Data unavailable"
            />
          );
        }

        const isUp = c.close >= c.open;
        const pctChange =
          referenceClose > 0
            ? ((c.close - referenceClose) / referenceClose) * 100
            : 0;
        const boundedPctChange = Math.max(
          -trendScale,
          Math.min(trendScale, pctChange),
        );
        const heightPct =
          ((boundedPctChange + trendScale) / (trendScale * 2)) * 100;

        return (
          <div
            key={`bar-${i}`}
            className={`chart-bar ${isUp ? 'up' : 'down'}`}
            style={{ height: `${Math.max(8, heightPct)}%` }}
          />
        );
      })}
    </div>
  );
}
