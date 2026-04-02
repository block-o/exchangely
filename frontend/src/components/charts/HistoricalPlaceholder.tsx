import type { Candle } from "../../types/api";
import { formatNumber, formatUnix } from "../../lib/format";

type Props = {
  candles: Candle[];
};

export function HistoricalPlaceholder({ candles }: Props) {
  return (
    <div className="chart-placeholder">
      {candles.slice(0, 8).map((candle) => (
        <div key={`${candle.pair}-${candle.timestamp}`} className="chart-row">
          <span>{formatUnix(candle.timestamp)}</span>
          <span>O {formatNumber(candle.open)}</span>
          <span>H {formatNumber(candle.high)}</span>
          <span>L {formatNumber(candle.low)}</span>
          <span>C {formatNumber(candle.close)}</span>
        </div>
      ))}
    </div>
  );
}
