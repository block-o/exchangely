import { useEffect, useState } from "react";
import { fetchPairs } from "../api/pairs";
import { fetchTicker } from "../api/system";
import { fetchHistorical } from "../api/historical";
import { useApi } from "../hooks/useApi";
import { useSettings } from "../app/settings";
import { formatNumber, formatUnix } from "../lib/format";
import type { Ticker, Candle, Pair } from "../types/api";

export function MarketPage() {
  const { quoteCurrency } = useSettings();
  const { data: pairsData, error: pairsError, loading: pairsLoading } = useApi(fetchPairs);
  const [tickers, setTickers] = useState<Record<string, Ticker>>({});
  const [candles, setCandles] = useState<Record<string, Candle[]>>({});
  const [loadingExtras, setLoadingExtras] = useState(false);

  useEffect(() => {
    if (!pairsData?.data) return;

    let active = true;
    setLoadingExtras(true);

    const loadExtras = async () => {
      // Filter pairs matching the selected quote currency
      const items = pairsData.data.filter((p: Pair) => p.quote === quoteCurrency);
      const newTickers: Record<string, Ticker> = {};
      const newCandles: Record<string, Candle[]> = {};

      for (const pair of items) {
        try {
          const tickerRes = await fetchTicker(pair.symbol);
          newTickers[pair.symbol] = tickerRes;
        } catch (e) {
          console.warn("Failed to fetch ticker for", pair.symbol, e);
        }

        try {
          const histRes = await fetchHistorical(pair.symbol, "1h");
          if (histRes?.data) {
            newCandles[pair.symbol] = histRes.data.slice(0, 12).reverse();
          }
        } catch (e) {
          console.warn("Failed to fetch historical for", pair.symbol, e);
        }
      }

      if (active) {
        setTickers(newTickers);
        setCandles(newCandles);
        setLoadingExtras(false);
      }
    };

    loadExtras();

    return () => { active = false; };
  }, [pairsData]);

  return (
    <section id="market" className="panel">
      <div className="panel-header">
        <h2>Market Overview</h2>
        <p>Realtime ticker prices and 24h variation across supported pairs.</p>
      </div>

      {pairsLoading && <p>Loading pairs...</p>}
      {pairsError && <p className="error">{pairsError}</p>}
      
      {pairsData?.data ? (
        <div className="data-table-wrapper">
          <table className="data-table">
            <thead>
              <tr>
                <th>Asset</th>
                <th>Price ({quoteCurrency})</th>
                <th>24h Chg</th>
                <th>Source</th>
                <th>Updated</th>
                <th>Trend (12h)</th>
              </tr>
            </thead>
            <tbody style={{ opacity: loadingExtras ? 0.5 : 1 }}>
              {pairsData.data
                .filter((p: Pair) => p.quote === quoteCurrency)
                .map((pair) => {
                  const tk = tickers[pair.symbol];
                  const hist = candles[pair.symbol] || [];
                  const var24h = tk?.variation_24h || 0;
                  
                  return (
                    <tr key={pair.symbol}>
                      <td className="symbol">{pair.base}</td>
                      <td className="price">
                      {tk ? formatNumber(tk.price) : "-"}
                    </td>
                    <td className={var24h >= 0 ? "text-up" : "text-down"}>
                      {tk ? `${var24h >= 0 ? "+" : ""}${formatNumber(var24h)}%` : "-"}
                    </td>
                    <td className="text-muted">{tk ? tk.source : "-"}</td>
                    <td className="text-muted">
                      {tk ? formatUnix(tk.last_update_unix).split(", ")[1] : "-"}
                    </td>
                    <td>
                      <div className="chart-placeholder" style={{ width: '60px' }}>
                        {hist.map((c, i) => {
                          const isUp = c.close >= c.open;
                          const minLow = Math.min(...hist.map((x) => x.low));
                          const maxHigh = Math.max(...hist.map((x) => x.high));
                          const heightPct = maxHigh === minLow ? 50 : ((c.close - minLow) / (maxHigh - minLow)) * 100;
                          return (
                            <div 
                              key={i} 
                              className={`chart-bar ${isUp ? 'up' : 'down'}`}
                              style={{ height: `${Math.max(10, heightPct)}%` }}
                              title={`C: ${c.close}`}
                            />
                          );
                        })}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  );
}
