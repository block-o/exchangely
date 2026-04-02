import { fetchHistorical } from "../api/historical";
import { fetchTicker } from "../api/system";
import { HistoricalPlaceholder } from "../components/charts/HistoricalPlaceholder";
import { useApi } from "../hooks/useApi";
import { formatNumber, formatUnix } from "../lib/format";

const DEFAULT_PAIR = "BTCEUR";

export function PairDetailPage() {
  const historical = useApi(() => fetchHistorical(DEFAULT_PAIR), []);
  const ticker = useApi(() => fetchTicker(DEFAULT_PAIR), []);

  return (
    <section id="pair-detail" className="panel">
      <div className="panel-header">
        <h2>{DEFAULT_PAIR}</h2>
        <p>Example pair detail wired to the historical and ticker endpoints.</p>
      </div>
      {ticker.data ? (
        <div className="ticker-strip">
          <span>Price: {formatNumber(ticker.data.price)}</span>
          <span>24h: {formatNumber(ticker.data.variation_24h)}%</span>
          <span>Updated: {formatUnix(ticker.data.last_update_unix)}</span>
        </div>
      ) : null}
      {historical.data ? <HistoricalPlaceholder candles={historical.data.data} /> : null}
    </section>
  );
}
