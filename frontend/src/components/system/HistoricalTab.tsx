import { useEffect, useState } from "react";
import { API_BASE_URL } from "../../api/client";
import type { SyncPairStatus } from "../../types/api";
import { formatUnixDate } from "./shared";

export function HistoricalTab() {
  const [syncStatus, setSyncStatus] = useState<SyncPairStatus[]>([]);
  const [loading, setLoading] = useState(true);

  // Bootstrap from REST, then subscribe to the task stream for live updates.
  useEffect(() => {
    let cancelled = false;

    async function bootstrap() {
      try {
        const res = await fetch(`${API_BASE_URL}/system/sync-status`);
        const json = await res.json();
        if (!cancelled) {
          setSyncStatus(Array.isArray(json) ? json : []);
          setLoading(false);
        }
      } catch {
        if (!cancelled) setLoading(false);
      }
    }

    bootstrap();

    // The task stream now includes syncStatus in every event.
    const es = new EventSource(`${API_BASE_URL}/system/tasks/stream?upcoming_limit=1&recent_limit=1`);
    es.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data);
        if (Array.isArray(parsed.syncStatus)) {
          setSyncStatus(parsed.syncStatus);
          setLoading(false);
        }
      } catch {
        // ignore parse errors
      }
    };

    return () => {
      cancelled = true;
      es.close();
    };
  }, []);

  const completedCount = syncStatus.filter(
    (s) => s.hourly_backfill_completed && s.daily_backfill_completed
  ).length;

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
          <h3 style={{ fontSize: "1rem", margin: 0 }}>Historical Coverage</h3>
          <p style={{ margin: "0.35rem 0 0", opacity: 0.7, fontSize: "0.85rem" }}>
            Per-pair data resolution and backfill depth. Updates live as backfill tasks complete.
          </p>
        </div>
        <div style={{ fontSize: "0.82rem", opacity: 0.7 }}>
          {loading
            ? "Loading…"
            : `${completedCount}/${syncStatus.length} fully backfilled`}
        </div>
      </div>

      {loading ? (
        <p style={{ opacity: 0.6, fontSize: "0.9rem", margin: 0 }}>Loading coverage data…</p>
      ) : syncStatus.length === 0 ? (
        <p style={{ opacity: 0.6, fontSize: "0.9rem", margin: 0 }}>No pairs tracked yet.</p>
      ) : (
        <table className="data-table" style={{ fontSize: "0.85rem", width: "100%" }}>
          <thead>
            <tr>
              <th style={{ textAlign: "left" }}>Pair</th>
              <th>1h</th>
              <th>1d</th>
              <th>Earliest Data</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {syncStatus.map((s) => {
              const fullyDone = s.hourly_backfill_completed && s.daily_backfill_completed;
              const earliestUnix = s.hourly_synced_unix || s.daily_synced_unix;
              return (
                <tr key={s.pair}>
                  <td style={{ textAlign: "left", fontWeight: 600 }}>{s.pair}</td>
                  <td>
                    <ResolutionBadge available={s.hourly_backfill_completed || s.hourly_synced_unix > 0} complete={s.hourly_backfill_completed} />
                  </td>
                  <td>
                    <ResolutionBadge available={s.daily_backfill_completed || s.daily_synced_unix > 0} complete={s.daily_backfill_completed} />
                  </td>
                  <td style={{ opacity: 0.8, fontSize: "0.82rem", whiteSpace: "nowrap" }}>
                    {formatUnixDate(earliestUnix)}
                  </td>
                  <td>
                    {fullyDone ? (
                      <span className="text-up" style={{ fontSize: "0.82rem" }}>Complete</span>
                    ) : (
                      <span style={{ color: "rgba(255,196,61,0.9)", fontSize: "0.82rem" }}>Backfilling…</span>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}

function ResolutionBadge({ available, complete }: { available: boolean; complete: boolean }) {
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
