import type { Pair, Ticker, Candle } from "../types/api";
import {
  formatCurrencyNumber,
  formatCompactCurrencyNumber,
  formatVariation,
} from "../lib/format";
import { Sparkline } from "./ui";

export interface MarketCardProps {
  pair: Pair;
  ticker: Ticker | undefined;
  assetName: string;
  candles: Candle[];
  flashState: "up" | "down" | undefined;
  quoteCurrency: string;
}

function variationClass(value: number | undefined): string {
  if (value === undefined) return "";
  return value >= 0 ? "text-up" : "text-down";
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
        <Sparkline candles={candles} width={80} height={32} />
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
