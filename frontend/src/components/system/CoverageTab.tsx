import { useEffect, useMemo, useState } from "react";
import { API_BASE_URL, authFetch, authEventSource } from "../../api/client";
import type { SyncPairStatus, Ticker, Pair } from "../../types/api";
import { formatUnixDate } from "./shared";

// ── Constants ───────────────────────────────────────────────────────────────

const TICKER_STALE_THRESHOLD = 300; // 5 minutes

// ── Types ───────────────────────────────────────────────────────────────────

interface TickerStatus {
  pair: string;
  price: number;
  lastUpdateUnix: number;
  /** Wall-clock unix timestamp of the last SSE update received by the browser. */
  lastSeenUnix: number;
  source: string;
}

interface CoinGroup {
  base: string;
  quotes: QuoteRow[];
}

interface QuoteRow {
  quote: string;
  symbol: string;
  ticker: TickerStatus | null;
  sync: SyncPairStatus | null;
}

// ── Helpers ─────────────────────────────────────────────────────────────────

function isHealthy(ticker: TickerStatus): boolean {
  // Use the most recent of the backend candle timestamp and the browser-side
  // reception timestamp so the badge stays green as long as data is flowing.
  const effective = Math.max(ticker.lastUpdateUnix, ticker.lastSeenUnix);
  if (effective <= 0) return false;
  return Date.now() / 1000 - effective < TICKER_STALE_THRESHOLD;
}

function buildGroups(
  pairs: Pair[],
  tickerMap: Map<string, TickerStatus>,
  syncMap: Map<string, SyncPairStatus>,
): CoinGroup[] {
  const grouped = new Map<string, QuoteRow[]>();

  for (const p of pairs) {
    const rows = grouped.get(p.base) ?? [];
    rows.push({
      quote: p.quote,
      symbol: p.symbol,
      ticker: tickerMap.get(p.symbol) ?? null,
      sync: syncMap.get(p.symbol) ?? null,
    });
    grouped.set(p.base, rows);
  }

  return Array.from(grouped.entries())
    .map(([base, quotes]) => ({
      base,
      quotes: quotes.sort((a, b) => a.quote.localeCompare(b.quote)),
    }))
    .sort((a, b) => {
      // Sort by highest market cap ticker across quotes
      const capA = Math.max(...a.quotes.map((q) => q.ticker?.price ?? 0));
      const capB = Math.max(...b.quotes.map((q) => q.ticker?.price ?? 0));
      if (capB !== capA) return capB - capA;
      return a.base.localeCompare(b.base);
    });
}

// ── Component ───────────────────────────────────────────────────────────────

export function CoverageTab() {
  const [pairs, setPairs] = useState<Pair[]>([]);
  const [tickers, setTickers] = useState<Map<string, TickerStatus>>(new Map());
  const [syncStatus, setSyncStatus] = useState<Map<string, SyncPairStatus>>(new Map());
  const [loading, setLoading] = useState(true);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [initialExpandDone, setInitialExpandDone] = useState(false);

  // Re-evaluate staleness every 30s
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = window.setInterval(() => setTick((n) => n + 1), 30_000);
    return () => window.clearInterval(id);
  }, []);

  // Bootstrap data from REST, then subscribe to SSE streams
  useEffect(() => {
    let cancelled = false;

    async function bootstrap() {
      try {
        const [pairsRes, tickersRes, syncRes] = await Promise.all([
          fetch(`${API_BASE_URL}/pairs`).then((r) => r.json()),
          fetch(`${API_BASE_URL}/tickers`).then((r) => r.json()),
          authFetch(`${API_BASE_URL}/system/sync-status`).then((r) => r.json()),
        ]);

        if (cancelled) return;

        const pairsList: Pair[] = pairsRes?.data ?? [];
        setPairs(pairsList);

        const tMap = new Map<string, TickerStatus>();
        for (const t of tickersRes?.data ?? []) {
          tMap.set(t.pair, {
            pair: t.pair,
            price: t.price,
            lastUpdateUnix: t.last_update_unix,
            lastSeenUnix: Math.floor(Date.now() / 1000),
            source: t.source,
          });
        }
        setTickers(tMap);

        const sMap = new Map<string, SyncPairStatus>();
        for (const s of Array.isArray(syncRes) ? syncRes : []) {
          sMap.set(s.pair, s);
        }
        setSyncStatus(sMap);
        setLoading(false);
      } catch {
        if (!cancelled) setLoading(false);
      }
    }

    bootstrap();

    // Ticker SSE
    const tickerEs = new EventSource(`${API_BASE_URL}/tickers/stream`);
    tickerEs.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data);
        const deltas: Ticker[] = parsed.tickers ?? [];
        if (deltas.length === 0) return;

        setTickers((prev) => {
          const next = new Map(prev);
          const now = Math.floor(Date.now() / 1000);
          for (const d of deltas) {
            next.set(d.pair, {
              pair: d.pair,
              price: d.price,
              lastUpdateUnix: d.last_update_unix,
              lastSeenUnix: now,
              source: d.source,
            });
          }
          return next;
        });
      } catch {
        // ignore
      }
    };

    // Task/sync SSE
    const taskEs = authEventSource(
      `${API_BASE_URL}/system/tasks/stream?upcoming_limit=1&recent_limit=1`,
    );
    taskEs.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data);
        if (Array.isArray(parsed.syncStatus)) {
          const sMap = new Map<string, SyncPairStatus>();
          for (const s of parsed.syncStatus) {
            sMap.set(s.pair, s);
          }
          setSyncStatus(sMap);
        }
      } catch {
        // ignore
      }
    };

    return () => {
      cancelled = true;
      tickerEs.close();
      taskEs.close();
    };
  }, []);

  const groups = useMemo(
    () => buildGroups(pairs, tickers, syncStatus),
    [pairs, tickers, syncStatus],
  );

  // Auto-expand first 5 coins once data loads
  useEffect(() => {
    if (!initialExpandDone && groups.length > 0) {
      setExpanded(new Set(groups.slice(0, 5).map((g) => g.base)));
      setInitialExpandDone(true);
    }
  }, [groups, initialExpandDone]);

  function toggleExpand(base: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(base)) next.delete(base);
      else next.add(base);
      return next;
    });
  }

  // Aggregate stats
  const totalPairs = pairs.length;
  const liveCount = Array.from(tickers.values()).filter((t) => isHealthy(t)).length;
  const fullyBackfilled = Array.from(syncStatus.values()).filter(
    (s) => s.hourly_backfill_completed && s.daily_backfill_completed,
  ).length;

  return (
    <div>
      {/* Summary bar */}
      <div
        style={{
          display: "flex",
          gap: "1.5rem",
          marginBottom: "1rem",
          fontSize: "0.82rem",
          opacity: 0.7,
          flexWrap: "wrap",
        }}
      >
        {loading ? (
          <span>Loading…</span>
        ) : (
          <>
            <span>{totalPairs} pairs across {groups.length} coins</span>
            <span>{liveCount}/{totalPairs} live</span>
            <span>{fullyBackfilled}/{totalPairs} fully backfilled</span>
          </>
        )}
      </div>

      {loading ? (
        <p style={{ opacity: 0.6, fontSize: "0.9rem" }}>Loading coverage data…</p>
      ) : groups.length === 0 ? (
        <p style={{ opacity: 0.6, fontSize: "0.9rem" }}>No pairs tracked yet.</p>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: "0.6rem" }}>
          {groups.map((group) => (
            <CoinCard
              key={group.base}
              group={group}
              isExpanded={expanded.has(group.base)}
              onToggle={() => toggleExpand(group.base)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// ── CoinCard ────────────────────────────────────────────────────────────────

function CoinCard({
  group,
  isExpanded,
  onToggle,
}: {
  group: CoinGroup;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const liveCount = group.quotes.filter(
    (q) => q.ticker && isHealthy(q.ticker),
  ).length;
  const staleCount = group.quotes.length - liveCount;
  const fullyBackfilled = group.quotes.filter(
    (q) => q.sync?.hourly_backfill_completed && q.sync?.daily_backfill_completed,
  ).length;

  const allLive = staleCount === 0 && liveCount > 0;
  const allBackfilled = fullyBackfilled === group.quotes.length;

  return (
    <div
      style={{
        borderRadius: "12px",
        border: `1px solid ${allLive && allBackfilled ? "rgba(80,200,120,0.2)" : "rgba(255,255,255,0.08)"}`,
        background: "var(--surface-color, rgba(22,33,44,0.5))",
        overflow: "hidden",
        transition: "border-color 0.2s ease",
      }}
    >
      {/* Header */}
      <button
        onClick={onToggle}
        aria-expanded={isExpanded}
        aria-label={`${group.base} coverage details`}
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          width: "100%",
          padding: "0.75rem 1rem",
          background: "none",
          border: "none",
          color: "inherit",
          cursor: "pointer",
          gap: "1rem",
          textAlign: "left",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: "0.75rem" }}>
          <span
            style={{
              fontWeight: 600,
              fontSize: "1rem",
              fontFamily: "var(--font-display, 'Outfit', sans-serif)",
            }}
          >
            {group.base}
          </span>
          <span style={{ fontSize: "0.78rem", opacity: 0.5 }}>
            {group.quotes.length} pair{group.quotes.length !== 1 ? "s" : ""}
          </span>
        </div>

        <div style={{ display: "flex", alignItems: "center", gap: "1rem", fontSize: "0.78rem" }}>
          {/* Live status summary */}
          <span
            style={{
              color: allLive
                ? "rgba(80,200,120,0.9)"
                : staleCount > 0
                  ? "rgba(255,107,107,0.9)"
                  : "rgba(255,255,255,0.4)",
            }}
          >
            {liveCount}/{group.quotes.length} live
          </span>

          {/* Backfill progress bar */}
          <div
            style={{
              display: "flex",
              gap: "2px",
              alignItems: "center",
            }}
            title={`${fullyBackfilled}/${group.quotes.length} fully backfilled`}
          >
            {group.quotes.map((q) => {
              const done = q.sync?.hourly_backfill_completed && q.sync?.daily_backfill_completed;
              const partial =
                !done &&
                (q.sync?.hourly_synced_unix ?? 0) > 0;
              return (
                <div
                  key={q.symbol}
                  style={{
                    width: "18px",
                    height: "6px",
                    borderRadius: "3px",
                    background: done
                      ? "rgba(80,200,120,0.7)"
                      : partial
                        ? "rgba(255,196,61,0.6)"
                        : "rgba(255,255,255,0.1)",
                    transition: "background 0.3s ease",
                  }}
                />
              );
            })}
          </div>

          {/* Chevron */}
          <span
            style={{
              fontSize: "0.7rem",
              opacity: 0.5,
              transition: "transform 0.2s ease",
              transform: isExpanded ? "rotate(180deg)" : "rotate(0deg)",
              display: "inline-block",
            }}
          >
            ▼
          </span>
        </div>
      </button>

      {/* Expanded content */}
      {isExpanded && (
        <div style={{ padding: "0 1rem 0.75rem" }}>
          <table
            className="data-table"
            style={{ fontSize: "0.84rem", width: "100%", borderCollapse: "collapse" }}
          >
            <thead>
              <tr>
                <th style={{ textAlign: "left", padding: "0.5rem 0.5rem" }}>Quote</th>
                <th style={{ padding: "0.5rem 0.5rem" }}>Price</th>
                <th style={{ padding: "0.5rem 0.5rem" }}>Feed</th>
                <th style={{ padding: "0.5rem 0.5rem" }}>1h</th>
                <th style={{ padding: "0.5rem 0.5rem" }}>1d</th>
              </tr>
            </thead>
            <tbody>
              {group.quotes.map((q) => (
                <QuoteRowView key={q.symbol} row={q} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ── QuoteRowView ────────────────────────────────────────────────────────────

function QuoteRowView({ row }: { row: QuoteRow }) {
  const healthy = row.ticker ? isHealthy(row.ticker) : false;

  return (
    <tr>
      <td style={{ textAlign: "left", fontWeight: 600, padding: "0.5rem 0.5rem" }}>
        {row.quote}
      </td>
      <td style={{ padding: "0.5rem 0.5rem", fontVariantNumeric: "tabular-nums" }}>
        {row.ticker ? formatPrice(row.ticker.price, row.quote) : "—"}
      </td>
      <td style={{ padding: "0.5rem 0.5rem" }}>
        <FeedBadge healthy={healthy} hasTicker={!!row.ticker} source={row.ticker?.source} />
      </td>
      <td style={{ padding: "0.5rem 0.5rem" }}>
        <ResolutionCell
          available={
            (row.sync?.hourly_backfill_completed ?? false) ||
            (row.sync?.hourly_synced_unix ?? 0) > 0
          }
          complete={row.sync?.hourly_backfill_completed ?? false}
          earliestUnix={row.sync?.hourly_synced_unix ?? 0}
        />
      </td>
      <td style={{ padding: "0.5rem 0.5rem" }}>
        <ResolutionCell
          available={
            (row.sync?.daily_backfill_completed ?? false) ||
            (row.sync?.daily_synced_unix ?? 0) > 0
          }
          complete={row.sync?.daily_backfill_completed ?? false}
          earliestUnix={row.sync?.daily_synced_unix ?? 0}
        />
      </td>
    </tr>
  );
}

// ── Small UI pieces ─────────────────────────────────────────────────────────

function FeedBadge({
  healthy,
  hasTicker,
  source,
}: {
  healthy: boolean;
  hasTicker: boolean;
  source?: string;
}) {
  if (!hasTicker) {
    return (
      <span style={{ fontSize: "0.78rem", opacity: 0.4 }}>—</span>
    );
  }

  const label = source === "consolidated" ? "Consolidated" : source ?? "";

  return (
    <span
      style={{
        display: "inline-flex",
        flexDirection: "column",
        alignItems: "center",
        gap: "1px",
      }}
    >
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
      {label && (
        <span style={{ fontSize: "0.68rem", opacity: 0.45, textTransform: "capitalize" }}>
          {label}
        </span>
      )}
    </span>
  );
}

function ResolutionCell({
  available,
  complete,
  earliestUnix,
}: {
  available: boolean;
  complete: boolean;
  earliestUnix: number;
}) {
  return (
    <span
      style={{
        display: "inline-flex",
        flexDirection: "column",
        alignItems: "center",
        gap: "2px",
      }}
    >
      <ResolutionBadge available={available} complete={complete} />
      {earliestUnix > 0 && (
        <span style={{ fontSize: "0.68rem", opacity: 0.45, whiteSpace: "nowrap" }}>
          {formatUnixDate(earliestUnix)}
        </span>
      )}
    </span>
  );
}

function ResolutionBadge({
  available,
  complete,
}: {
  available: boolean;
  complete: boolean;
}) {
  if (complete) {
    return (
      <span
        style={{
          display: "inline-block",
          padding: "1px 8px",
          borderRadius: "6px",
          fontSize: "0.78rem",
          background: "rgba(80,200,120,0.15)",
          color: "rgba(80,200,120,0.9)",
          border: "1px solid rgba(80,200,120,0.3)",
        }}
      >
        ✓
      </span>
    );
  }
  if (available) {
    return (
      <span
        style={{
          display: "inline-block",
          padding: "1px 8px",
          borderRadius: "6px",
          fontSize: "0.78rem",
          background: "rgba(255,196,61,0.12)",
          color: "rgba(255,196,61,0.9)",
          border: "1px solid rgba(255,196,61,0.3)",
        }}
      >
        …
      </span>
    );
  }
  return (
    <span
      style={{
        display: "inline-block",
        padding: "1px 8px",
        borderRadius: "6px",
        fontSize: "0.78rem",
        background: "rgba(255,255,255,0.04)",
        color: "rgba(255,255,255,0.3)",
        border: "1px solid rgba(255,255,255,0.08)",
      }}
    >
      —
    </span>
  );
}

function formatPrice(price: number, quote: string): string {
  const currency = quote === "EUR" ? "EUR" : quote === "USD" ? "USD" : undefined;
  if (currency) {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(price);
  }
  return price.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}
