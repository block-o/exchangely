import { useEffect, useState } from "react";
import { API_BASE_URL } from "../../api/client";
import type { Ticker } from "../../types/api";

const TICKER_STALE_THRESHOLD = 300; // 5 minutes

interface TickerStatus {
  pair: string;
  price: number;
  lastUpdateUnix: number;
  source: string;
  healthy: boolean;
}

function isHealthy(lastUpdateUnix: number): boolean {
  if (lastUpdateUnix <= 0) return false;
  return Date.now() / 1000 - lastUpdateUnix < TICKER_STALE_THRESHOLD;
}

function toTickerStatus(t: Ticker): TickerStatus {
  return {
    pair: t.pair,
    price: t.price,
    lastUpdateUnix: t.last_update_unix,
    source: t.source,
    healthy: isHealthy(t.last_update_unix),
  };
}

function mapTickers(data: Ticker[]): TickerStatus[] {
  return data.map(toTickerStatus).sort((a, b) => a.pair.localeCompare(b.pair));
}

export function LiveTab() {
  const [tickers, setTickers] = useState<TickerStatus[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function bootstrap() {
      try {
        const res = await fetch(`${API_BASE_URL}/tickers`);
        const json = await res.json();
        if (!cancelled) {
          setTickers(mapTickers(json.data || []));
          setLoading(false);
        }
      } catch {
        if (!cancelled) setLoading(false);
      }
    }

    bootstrap();

    const es = new EventSource(`${API_BASE_URL}/tickers/stream`);
    es.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data);
        const deltas: Ticker[] = parsed.tickers || [];
        if (deltas.length === 0) return;

        setTickers((prev) => {
          const map = new Map(prev.map((t) => [t.pair, t]));
          for (const d of deltas) {
            map.set(d.pair, toTickerStatus(d));
          }
          return Array.from(map.values()).sort((a, b) => a.pair.localeCompare(b.pair));
        });
      } catch {
        // ignore parse errors
      }
    };

    return () => {
      cancelled = true;
      es.close();
    };
  }, []);

  // Re-evaluate staleness every 30s
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = window.setInterval(() => setTick((n) => n + 1), 30_000);
    return () => window.clearInterval(id);
  }, []);

  const healthyCount = tickers.filter((t) => isHealthy(t.lastUpdateUnix)).length;
  const staleCount = tickers.length - healthyCount;

  return (
    <div
      style={{
        padding: "1rem",
        backgroundColor: "var(--surface-color)",
        borderRadius: "12px",
      }}
    >
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          marginBottom: "1rem",
          gap: "1rem",
          flexWrap: "wrap",
        }}
      >
        <div>
          <h3 style={{ fontSize: "1rem", margin: 0 }}>Live Ticker Status</h3>
          <p style={{ margin: "0.35rem 0 0", opacity: 0.7, fontSize: "0.85rem" }}>
            Per-pair realtime feed health. Tickers not updated in {TICKER_STALE_THRESHOLD / 60}m are flagged stale.
          </p>
        </div>
        <div style={{ fontSize: "0.82rem", opacity: 0.7 }}>
          {loading
            ? "Loading…"
            : staleCount > 0
            ? `${healthyCount} healthy · ${staleCount} stale`
            : `${tickers.length} healthy`}
        </div>
      </div>

      {loading ? (
        <p style={{ opacity: 0.6, fontSize: "0.9rem", margin: 0 }}>Loading ticker status…</p>
      ) : tickers.length === 0 ? (
        <p style={{ opacity: 0.6, fontSize: "0.9rem", margin: 0 }}>No tickers available yet.</p>
      ) : (
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))",
            gap: "0.6rem",
          }}
        >
          {tickers.map((t) => {
            const healthy = isHealthy(t.lastUpdateUnix);
            return (
              <div
                key={t.pair}
                style={{
                  padding: "0.7rem 0.85rem",
                  borderRadius: "10px",
                  border: `1px solid ${healthy ? "rgba(80,200,120,0.3)" : "rgba(255,107,107,0.4)"}`,
                  background: healthy ? "rgba(30,80,50,0.12)" : "rgba(120,28,28,0.15)",
                  display: "flex",
                  flexDirection: "column",
                  gap: "0.3rem",
                }}
              >
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <span style={{ fontWeight: 600, fontSize: "0.9rem" }}>{t.pair}</span>
                  <span
                    style={{
                      fontSize: "0.72rem",
                      letterSpacing: "0.04em",
                      textTransform: "uppercase",
                      color: healthy ? "rgba(80,200,120,0.9)" : "rgba(255,107,107,0.9)",
                    }}
                  >
                    {healthy ? "● Live" : "● Stale"}
                  </span>
                </div>
                <div style={{ fontSize: "0.8rem", opacity: 0.75 }}>
                  Source: {t.source === "consolidated" ? "Consolidated" : t.source}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
