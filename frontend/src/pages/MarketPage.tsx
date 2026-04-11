import { useEffect, useState, useRef, useMemo } from "react";
import { fetchAssets } from "../api/assets";
import { fetchPairs } from "../api/pairs";
import { fetchHistorical } from "../api/historical";
import { useApi } from "../hooks/useApi";
import { useSettings } from "../app/settings";
import { useBreakpoint } from "../hooks/useBreakpoint";
import {
  formatCompactCurrencyNumber,
  formatCompactNumber,
  formatCurrencyNumber,
  formatNumber,
  formatUnix,
  formatVariation,
  getBrowserTimezone,
} from "../lib/format";
import type { Ticker, Candle, Pair } from "../types/api";
import { NewsTicker } from "../components/layout/NewsTicker";
import { MarketCard } from "../components/MarketCard";

function parseTickerStreamPayload(payload: string): Ticker[] {
  const parsed = JSON.parse(payload);
  if (Array.isArray(parsed)) {
    return parsed;
  }
  if (Array.isArray(parsed?.tickers)) {
    return parsed.tickers;
  }
  return [];
}

/** Minimum scale so flat data still renders visible bars. */
const MIN_TREND_SCALE_PCT = 0.5;

/** How often (ms) to re-fetch sparkline candles so the chart stays fresh. */
const SPARKLINE_REFRESH_MS = 5 * 60 * 1000; // 5 minutes

export function MarketPage() {
  const { quoteCurrency } = useSettings();
  const bp = useBreakpoint();
  const sparklineWidth = bp === 'mobile' ? 80 : 120;
  const sparklineHeight = bp === 'mobile' ? 32 : 40;
  const { data: assetsData } = useApi(fetchAssets);
  const { data: pairsData, error: pairsError, loading: pairsLoading } = useApi(fetchPairs);
  const [tickers, setTickers] = useState<Record<string, Ticker>>({});
  const [candles, setCandles] = useState<Record<string, Candle[]>>({});
  const [loadingExtras, setLoadingExtras] = useState(false);
  const [flashState, setFlashState] = useState<Record<string, 'up' | 'down'>>({});
  const [streamConnected, setStreamConnected] = useState(false);
  const [lastStreamUpdateUnix, setLastStreamUpdateUnix] = useState(0);
  
  // Track previous tickers to compute flashes
  const tickersRef = useRef(tickers);
  useEffect(() => {
    tickersRef.current = tickers;
  }, [tickers]);

  useEffect(() => {
    if (!pairsData?.data) return;

    let active = true;
    setLoadingExtras(true);
    setStreamConnected(false);

    const loadExtras = async () => {
      const items = pairsData.data.filter((p: Pair) => p.quote === quoteCurrency);
      const newCandles: Record<string, Candle[]> = {};
      const sparklineEnd = Math.floor(Date.now() / 1000);
      const sparklineStart = sparklineEnd - 24 * 3600;

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
          const histRes = await fetchHistorical(pair.symbol, "1h", sparklineStart, sparklineEnd);
          if (histRes?.data) {
            newCandles[pair.symbol] = histRes.data.slice(-24);
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
    const es = new EventSource(import.meta.env.VITE_API_BASE_URL + `/tickers/stream?quote=${quoteCurrency}`);
    es.onopen = () => {
      if (active) {
        setStreamConnected(true);
      }
    };
    es.onmessage = (event) => {
      try {
        const incoming = parseTickerStreamPayload(event.data);
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
          setStreamConnected(true);
          setLastStreamUpdateUnix(Math.floor(Date.now() / 1000));
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
    es.onerror = () => {
      if (active) {
        setStreamConnected(false);
      }
    };

    return () => { 
      active = false;
      es.close();
    };
  }, [pairsData, quoteCurrency]);

  // Periodically refresh sparkline candles so the trend chart stays current.
  useEffect(() => {
    if (!pairsData?.data) return;
    let active = true;

    const refreshCandles = async () => {
      const items = pairsData.data.filter((p: Pair) => p.quote === quoteCurrency);
      const sparklineEnd = Math.floor(Date.now() / 1000);
      const sparklineStart = sparklineEnd - 24 * 3600;
      const updated: Record<string, Candle[]> = {};

      for (const pair of items) {
        try {
          const histRes = await fetchHistorical(pair.symbol, "1h", sparklineStart, sparklineEnd);
          if (histRes?.data) {
            updated[pair.symbol] = histRes.data.slice(-24);
          }
        } catch {
          // keep previous candles on failure
        }
      }

      if (active && Object.keys(updated).length > 0) {
        setCandles(prev => ({ ...prev, ...updated }));
      }
    };

    const id = setInterval(refreshCandles, SPARKLINE_REFRESH_MS);
    return () => { active = false; clearInterval(id); };
  }, [pairsData, quoteCurrency]);

  const visiblePairs = useMemo(() => {
    const items = pairsData?.data ?? [];
    return items
      .filter((pair: Pair) => pair.quote === quoteCurrency)
      .sort((left, right) => {
        const leftCap = tickers[left.symbol]?.market_cap ?? 0;
        const rightCap = tickers[right.symbol]?.market_cap ?? 0;
        if (rightCap !== leftCap) {
          return rightCap - leftCap;
        }
        return left.base.localeCompare(right.base);
      });
  }, [pairsData, quoteCurrency, tickers]);

  const assetNames = useMemo(() => {
    const items = assetsData?.data ?? [];
    return Object.fromEntries(items.map((asset) => [asset.symbol, asset.name]));
  }, [assetsData]);

  return (
    <>
    <NewsTicker />
    <section id="market" className="panel">
      <div className="panel-header">
        <h2>Market Overview</h2>
        <p>Realtime ticker prices and 24h variation across supported pairs.</p>
      </div>

      {pairsLoading && <p>Loading pairs...</p>}
      {pairsError && <p className="error">{pairsError}</p>}
      
      {pairsData?.data ? (
        <>
          {bp === 'mobile' ? (
            <div className="market-card-list" style={{ opacity: loadingExtras ? 0.5 : 1 }}>
              {visiblePairs.map((pair) => (
                <MarketCard
                  key={pair.symbol}
                  pair={pair}
                  ticker={tickers[pair.symbol]}
                  assetName={assetNames[pair.base] ?? pair.base}
                  candles={candles[pair.symbol] || []}
                  flashState={flashState[pair.symbol]}
                  quoteCurrency={quoteCurrency}
                />
              ))}
            </div>
          ) : (
            <div className="data-table-wrapper">
              <table className={`data-table ${bp === 'tablet' ? 'tablet-market-table' : ''}`}>
                <thead>
                  <tr>
                    <th>Asset</th>
                    <th>Market Cap</th>
                    <th>Price</th>
                    <th className="col-1h">1h %</th>
                    <th>24h %</th>
                    <th className="col-7d">7d %</th>
                    <th title="Total volume across multiple sources (Kraken, Binance, etc.)">24h Vol *</th>
                    <th className="col-high col-low">24h High/Low</th>
                    <th>Trend (24h)</th>
                  </tr>
                </thead>
                <tbody style={{ opacity: loadingExtras ? 0.5 : 1 }}>
                  {visiblePairs.map((pair) => {
                    const tk = tickers[pair.symbol];
                    const hist = candles[pair.symbol] || [];
                    const var24h = tk?.variation_24h || 0;

                    return (
                      <tr key={pair.symbol} className={flashState[pair.symbol] ? `flash-${flashState[pair.symbol]}` : ""}>
                        <td className="symbol">
                          <div className="asset-cell">
                            <span className="asset-name">{assetNames[pair.base] ?? pair.base}</span>
                            <span className="asset-code">{pair.base}</span>
                          </div>
                        </td>
                        <td className="text-muted">{formatCompactCurrencyNumber(tk?.market_cap, quoteCurrency)}</td>
                        <td className="price">
                          {tk ? formatCurrencyNumber(tk.price, quoteCurrency) : "-"}
                        </td>
                        <td className={`col-1h ${tk && tk.variation_1h >= 0 ? "text-up" : "text-down"}`}>
                          {tk ? formatVariation(tk.variation_1h) : "-"}
                        </td>
                        <td className={var24h >= 0 ? "text-up" : "text-down"}>
                          {tk ? formatVariation(var24h) : "-"}
                        </td>
                        <td className={`col-7d ${tk && tk.variation_7d >= 0 ? "text-up" : "text-down"}`}>
                          {tk ? formatVariation(tk.variation_7d) : "-"}
                        </td>
                        <td className="text-muted">
                          {tk ? formatCompactCurrencyNumber(tk.volume_24h, quoteCurrency) : "-"}
                        </td>
                        <td className="text-muted col-high col-low" style={{ fontSize: '0.78rem' }}>
                          <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
                            <span>H: {tk?.high_24h !== undefined ? formatCurrencyNumber(tk.high_24h, quoteCurrency) : "-"}</span>
                            <span>L: {tk?.low_24h !== undefined ? formatCurrencyNumber(tk.low_24h, quoteCurrency) : "-"}</span>
                          </div>
                        </td>
                        <td>
                          <div className="chart-placeholder" style={{ width: `${sparklineWidth}px`, height: `${sparklineHeight}px`, margin: '0 auto', display: 'flex', alignItems: 'flex-end', gap: '2px' }}>
                            {(() => {
                              const latestCandleUnix = hist.length > 0 ? hist[hist.length - 1].timestamp : Math.floor(Date.now() / 1000);
                              const currentHourUnix = Math.floor(latestCandleUnix / 3600) * 3600;
                              const plotted = Array.from({ length: 24 }).map((_, i) => {
                                const bucketUnix = currentHourUnix - (23 - i) * 3600;
                                return hist.find(x => x.timestamp >= bucketUnix && x.timestamp < bucketUnix + 3600);
                              });
                              
                              const validCandles = plotted.filter(c => !!c) as typeof hist;
                              const referenceClose = validCandles.length > 0 ? validCandles[0].close : 0;

                              // Derive scale from actual data range so bars fill the chart.
                              let trendScale = MIN_TREND_SCALE_PCT;
                              if (referenceClose > 0 && validCandles.length > 1) {
                                const maxPct = validCandles.reduce((mx, cc) => Math.max(mx, Math.abs(((cc.close - referenceClose) / referenceClose) * 100)), 0);
                                if (maxPct > trendScale) trendScale = maxPct * 1.1; // 10 % headroom
                              }

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
                                const pctChange = referenceClose > 0 ? ((c.close - referenceClose) / referenceClose) * 100 : 0;
                                const boundedPctChange = Math.max(-trendScale, Math.min(trendScale, pctChange));
                                const heightPct = ((boundedPctChange + trendScale) / (trendScale * 2)) * 100;
                                return (
                                  <div 
                                    key={`val-${i}`} 
                                    className={`chart-bar ${isUp ? 'up' : 'down'}`}
                                    style={{ height: `${Math.max(8, heightPct)}%` }}
                                    title={`C: ${formatNumber(c.close)} (${pctChange >= 0 ? "+" : ""}${formatNumber(pctChange)}%)`}
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
            </div>
          )}

          <div className="market-footer">
            <span className={`market-stream-status ${streamConnected ? "is-live" : "is-offline"}`}>
              <span className="market-stream-dot" aria-hidden="true" />
              Live
            </span>
            <span>
              Last Updated: {lastStreamUpdateUnix > 0 ? formatUnix(lastStreamUpdateUnix) : "-"}
            </span>
            <span>All times shown in {getBrowserTimezone()}</span>
          </div>
        </>
      ) : null}
    </section>
    </>
  );
}
