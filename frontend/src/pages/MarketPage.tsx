import { useEffect, useState, useRef, useMemo } from "react";
import { fetchPairs } from "../api/pairs";
import { fetchTicker } from "../api/system";
import { fetchHistorical } from "../api/historical";
import { useApi } from "../hooks/useApi";
import { useSettings } from "../app/settings";
import { formatNumber, formatUnix, getBrowserTimezone } from "../lib/format";
import type { Ticker, Candle, Pair } from "../types/api";

export function MarketPage() {
  const { quoteCurrency } = useSettings();
  const { data: pairsData, error: pairsError, loading: pairsLoading } = useApi(fetchPairs);
  const [tickers, setTickers] = useState<Record<string, Ticker>>({});
  const [candles, setCandles] = useState<Record<string, Candle[]>>({});
  const [loadingExtras, setLoadingExtras] = useState(false);
  const [flashState, setFlashState] = useState<Record<string, 'up' | 'down'>>({});
  
  // Track previous tickers to compute flashes
  const tickersRef = useRef(tickers);
  useEffect(() => {
    tickersRef.current = tickers;
  }, [tickers]);

  useEffect(() => {
    if (!pairsData?.data) return;

    let active = true;
    setLoadingExtras(true);

    const loadExtras = async () => {
      const items = pairsData.data.filter((p: Pair) => p.quote === quoteCurrency);
      const newCandles: Record<string, Candle[]> = {};

      // 1. Fetch initial global state
      try {
        const res = await fetch(import.meta.env.VITE_API_BASE_URL + "/tickers");
        if (res.ok) {
          const json = await res.json();
          const tMap: Record<string, Ticker> = {};
          if (json.data) {
            json.data.forEach((t: Ticker) => tMap[t.pair] = t);
          }
          if (active) setTickers(tMap);
        }
      } catch (e) {
        console.error("Failed to fetch initial tickers", e);
      }

      // 2. Fetch historical candles for sparklines
      for (const pair of items) {
        try {
          const histRes = await fetchHistorical(pair.symbol, "1h");
          if (histRes?.data) {
            newCandles[pair.symbol] = histRes.data.slice(0, 24).reverse();
          }
        } catch (e) {
          console.warn("Failed to fetch historical for", pair.symbol, e);
        }
      }

      if (active) {
        setCandles(newCandles);
        setLoadingExtras(false);
      }
    };

    loadExtras();

    // 3. Establish SSE Stream for realtime reactive webhooks
    const es = new EventSource(import.meta.env.VITE_API_BASE_URL + "/tickers/stream");
    es.onmessage = (event) => {
      try {
        const incoming: Ticker[] = JSON.parse(event.data) || [];
        const prev = tickersRef.current;
        const next = { ...prev };
        const updates: Record<string, 'up' | 'down'> = {};

        incoming.forEach(t => {
          if (prev[t.pair] && prev[t.pair].price !== t.price) {
            updates[t.pair] = t.price > prev[t.pair].price ? 'up' : 'down';
          }
          next[t.pair] = t;
        });

        if (active) {
          setTickers(next);
          if (Object.keys(updates).length > 0) {
            setFlashState(f => ({ ...f, ...updates }));
            setTimeout(() => {
              if (active) {
                setFlashState(f => {
                  const sf = { ...f };
                  Object.keys(updates).forEach(k => delete sf[k]);
                  return sf;
                });
              }
            }, 1000);
          }
        }
      } catch (e) {
        console.error("SSE parse error:", e);
      }
    };

    return () => { 
      active = false;
      es.close();
    };
  }, [pairsData, quoteCurrency]);

  const maxUpdatedUnix = useMemo(() => {
    let max = 0;
    Object.values(tickers).forEach(t => {
      if (t.last_update_unix > max) max = t.last_update_unix;
    });
    return max;
  }, [tickers]);

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
                <th>24h High</th>
                <th>24h Low</th>
                <th>Trend (24h)</th>
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
                    <tr key={pair.symbol} className={flashState[pair.symbol] ? `flash-${flashState[pair.symbol]}` : ""}>
                      <td className="symbol">{pair.base}</td>
                      <td className="price">
                      {tk ? formatNumber(tk.price) : "-"}
                    </td>
                    <td className={var24h >= 0 ? "text-up" : "text-down"}>
                      {tk ? `${var24h >= 0 ? "+" : ""}${formatNumber(var24h)}%` : "-"}
                    </td>
                    <td className="text-muted">
                      {tk?.high_24h ? formatNumber(tk.high_24h) : "-"}
                    </td>
                    <td className="text-muted">
                      {tk?.low_24h ? formatNumber(tk.low_24h) : "-"}
                    </td>
                    <td>
                      <div className="chart-placeholder" style={{ width: '120px', margin: '0 auto', display: 'flex', alignItems: 'flex-end', gap: '2px' }}>
                        {(() => {
                          const latestCandleUnix = hist.length > 0 ? hist[hist.length - 1].timestamp : Math.floor(Date.now() / 1000);
                          const currentHourUnix = Math.floor(latestCandleUnix / 3600) * 3600;
                          const plotted = Array.from({ length: 24 }).map((_, i) => {
                            const bucketUnix = currentHourUnix - (23 - i) * 3600;
                            return hist.find(x => x.timestamp >= bucketUnix && x.timestamp < bucketUnix + 3600);
                          });
                          
                          const validCandles = plotted.filter(c => !!c) as typeof hist;
                          const minLow = validCandles.length > 0 ? Math.min(...validCandles.map((x) => x.low)) : 0;
                          const maxHigh = validCandles.length > 0 ? Math.max(...validCandles.map((x) => x.high)) : 0;

                          return plotted.map((c, i) => {
                            if (!c) {
                              return (
                                <div 
                                  key={`missing-${i}`} 
                                  className="chart-bar empty"
                                  style={{ height: '30%', backgroundColor: '#374151', opacity: 1 }}
                                  title="Data unavailable"
                                />
                              );
                            }

                            const isUp = c.close >= c.open;
                            const heightPct = maxHigh === minLow ? 50 : ((c.close - minLow) / (maxHigh - minLow)) * 100;
                            return (
                              <div 
                                key={`val-${i}`} 
                                className={`chart-bar ${isUp ? 'up' : 'down'}`}
                                style={{ height: `${Math.max(10, heightPct)}%` }}
                                title={`C: ${c.close}`}
                              />
                            );
                          });
                        })()}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          
          {maxUpdatedUnix > 0 && (
            <div className="text-muted" style={{ padding: '16px 20px', fontSize: '0.8em', textAlign: 'right', opacity: 0.7 }}>
              Last updated: {formatUnix(maxUpdatedUnix)} · All times shown in {getBrowserTimezone()}
            </div>
          )}
        </div>
      ) : null}
    </section>
  );
}
