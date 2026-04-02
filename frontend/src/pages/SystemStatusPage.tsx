import { fetchSyncStatus } from "../api/system";
import { useApi } from "../hooks/useApi";
import { formatUnix } from "../lib/format";

export function SystemStatusPage() {
  const { data, error, loading } = useApi(fetchSyncStatus);

  return (
    <section id="system-status" className="panel">
      <div className="panel-header">
        <h2>Sync Status</h2>
        <p>Per-pair backfill progress snapshot exposed by the backend.</p>
      </div>
      {loading ? <p>Loading sync state...</p> : null}
      {error ? <p className="error">{error}</p> : null}
      {data ? (
        <div className="sync-list">
          {data.pairs.map((item) => (
            <article key={item.pair} className="sync-item">
              <strong>{item.pair}</strong>
              <span>{item.backfill_completed ? "Complete" : "Catching up"}</span>
              <span>Last synced: {formatUnix(item.last_synced_unix)}</span>
              <span>Next target: {formatUnix(item.next_target_unix)}</span>
            </article>
          ))}
        </div>
      ) : null}
    </section>
  );
}
