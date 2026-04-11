import type { Pair, Ticker, Candle } from "../types/api";
import {
  formatCurrencyNumber,
  formatCompactCurrencyNumber,
  formatNumber,
  formatVariation,
} from "../lib/format";

export interface MarketCardProps {
  pair: Pair;
  ticker: Ticker | undefined;
  assetName: string;
  candles: Candle[];
  flashState: "up" | "down" | undefined;
  quoteCurrency: string;
}

const MIN_TREND_SCALE_PCT = 0.5;

function variationClass(value: number | undefined): string {
  if (value === undefined) return "";
  return value >= 0 ? "text-up" : "text-down";
}

function Sparkline({ candles }: { candles: Candle[] }) {
  const latestCandleUnix =
    candles.length > 0
      ? candles[candles.length - 1].timestamp
      : Math.floor(Date.now() / 1000);
  const currentHourUnix = Math.floor(latestCandleUnix / 3600) * 3600;

  const plotted = Array.from({ length: 24 }).map((_, i) => {
    const bucketUnix = currentHourUnix - (23 - i) * 3600;
    return candles.find(
      (x) => x.timestamp >= bucketUnix && x.timestamp < bucketUnix + 3600
    );
  });

  const validCandles = plotted.filter((c): c is Candle => !!c);
  const referenceClose = validCandles.length > 0 ? validCandles[0].close : 0;

  // Derive scale from actual data range so bars fill the chart.
  let trendScale = MIN_TREND_SCALE_PCT;
  if (referenceClose > 0 && validCandles.length > 1) {
    const maxPct = validCandles.reduce(
      (mx, cc) => Math.max(mx, Math.abs(((cc.close - referenceClose) / referenceClose) * 100)),
      0,
    );
    if (maxPct > trendScale) trendScale = maxPct * 1.1; // 10 % headroom
  }

  return (
    <div
      className="chart-placeholder"
      style={{
        width: "80px",
        maxWidth: "80px",
        flexShrink: 0,
        height: "32px",
        display: "flex",
        alignItems: "flex-end",
        gap: "1px",
        overflow: "hidden",
      }}
    >
      {plotted.map((c, i) => {
        if (!c) {
          return (
            <div
              key={`missing-${i}`}
              className="chart-bar empty"
              style={{
                height: "30%",
                backgroundColor: "#374151",
                opacity: 1,
              }}
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
          Math.min(trendScale, pctChange)
        );
        const heightPct =
          ((boundedPctChange + trendScale) / (trendScale * 2)) * 100;

        return (
          <div
            key={`val-${i}`}
            className={`chart-bar ${isUp ? "up" : "down"}`}
            style={{
              height: `${Math.max(8, heightPct)}%`,
            }}
            title={`C: ${formatNumber(c.close)} (${pctChange >= 0 ? "+" : ""}${formatNumber(pctChange)}%)`}
          />
        );
      })}
    </div>
  );
}

export function MarketCard({
  pair,
  ticker,
  assetName,
  candles,
  flashState,
  quoteCurrency,
}: MarketCardProps) {
  const flashClass = flashState ? `flash-${flashState}` : "";

  return (
    <div className={`market-card ${flashClass}`}>
      {/* Primary row: name, code, price */}
      <div className="market-card-primary">
        <div className="asset-cell">
          <span className="asset-name">{assetName}</span>
          <span className="asset-code">{pair.base}</span>
        </div>
        <span className="price">
          {ticker ? formatCurrencyNumber(ticker.price, quoteCurrency) : "-"}
        </span>
      </div>

      {/* 24h change + sparkline */}
      <div className="market-card-change">
        <span className={variationClass(ticker?.variation_24h)}>
          {formatVariation(ticker?.variation_24h)}
        </span>
        <Sparkline candles={candles} />
      </div>

      {/* Secondary info grid */}
      <div className="market-card-secondary">
        <div>
          <div className="label">MCap</div>
          <div className="value">
            {formatCompactCurrencyNumber(ticker?.market_cap, quoteCurrency)}
          </div>
        </div>
        <div>
          <div className="label">1h %</div>
          <div className={`value ${variationClass(ticker?.variation_1h)}`}>
            {formatVariation(ticker?.variation_1h)}
          </div>
        </div>
        <div>
          <div className="label">7d %</div>
          <div className={`value ${variationClass(ticker?.variation_7d)}`}>
            {formatVariation(ticker?.variation_7d)}
          </div>
        </div>
        <div>
          <div className="label">24h Vol</div>
          <div className="value">
            {formatCompactCurrencyNumber(ticker?.volume_24h, quoteCurrency)}
          </div>
        </div>
        <div>
          <div className="label">24h High</div>
          <div className="value">
            {ticker?.high_24h !== undefined
              ? formatCurrencyNumber(ticker.high_24h, quoteCurrency)
              : "-"}
          </div>
        </div>
        <div>
          <div className="label">24h Low</div>
          <div className="value">
            {ticker?.low_24h !== undefined
              ? formatCurrencyNumber(ticker.low_24h, quoteCurrency)
              : "-"}
          </div>
        </div>
      </div>
    </div>
  );
}
