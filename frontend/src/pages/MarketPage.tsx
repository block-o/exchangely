import './MarketPage.css';
import { useEffect, useState, useRef, useMemo } from "react";
import { fetchAssets } from "../api/assets";
import { fetchPairs } from "../api/pairs";
import { useApi } from "../hooks/useApi";
import { useSettings } from "../app/settings";
import { useBreakpoint } from "../hooks/useBreakpoint";
import {
  formatCompactCurrencyNumber,
  formatCompactNumber,
  formatCurrencyNumber,
  formatUnix,
  formatVariation,
  getBrowserTimezone,
} from "../lib/format";
import type { Ticker, Candle, Pair, SparklinePoint } from "../types/api";
import { NewsTicker } from "../components/layout/NewsTicker";
import { MarketCard } from "../components/MarketCard";
import { Sparkline } from "../components/ui";

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
      // Fetch initial global state — tickers now include sparkline data
      try {
        const res = await fetch(import.meta.env.VITE_API_BASE_URL + "/tickers");
        if (res.ok) {
          const json = await res.json();
          const tMap: Record<string, Ticker> = {};
          const cMap: Record<string, Candle[]> = {};
          if (json.data) {
            json.data.forEach((t: Ticker) => {
              tMap[t.pair] = t;
              // Convert sparkline points to Candle[] for the existing sparkline renderer
              if (t.sparkline && t.sparkline.length > 0) {
                cMap[t.pair] = t.sparkline.map((sp: SparklinePoint) => ({
                  pair: t.pair,
                  interval: "1h",
                  timestamp: sp.timestamp,
                  open: sp.open,
                  high: sp.high,
                  low: sp.low,
                  close: sp.close,
                  volume: sp.volume,
                  source: "",
                  finalized: true,
                }));
              }
            });
          }
          if (active) {
            setTickers(tMap);
            setCandles(cMap);
          }
        }
      } catch (e) {
        console.error("Failed to fetch initial tickers", e);
      }

      if (active) {
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
                          <Sparkline candles={hist} width={sparklineWidth} height={sparklineHeight} />
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
