import { fetchHealth } from "../api/health";
import { fetchSyncStatus } from "../api/system";
import { useApi } from "../hooks/useApi";
import { formatUnix } from "../lib/format";

export function SystemPage() {
  const health = useApi(fetchHealth);
  const sync = useApi(fetchSyncStatus);

  return (
    <section id="system" className="panel">
      <div className="panel-header">
        <h2>System Operations</h2>
        <p>Overall backend health, service availability, and backfill progression.</p>
      </div>

      {health.error && <p className="error">Health check failed: {health.error}</p>}
      
      <div className="status-grid">
        <div className="status-card">
          <span className="label">API Status</span>
          <span className={`value ${health.data?.status || 'unknown'}`}>
            {health.data?.status ?? (health.loading ? "Loading" : "Unknown")}
          </span>
        </div>
        <div className="status-card">
          <span className="label">Planner Leader</span>
          <span className="value text-muted">{sync.data?.planner_leader ?? "Unknown"}</span>
        </div>
        <div className="status-card">
          <span className="label">TimescaleDB</span>
          <span className={`value ${health.data?.checks?.timescaledb || 'unknown'}`}>
            {health.data?.checks?.timescaledb ?? "Unknown"}
          </span>
        </div>
        <div className="status-card">
          <span className="label">Kafka</span>
          <span className={`value ${health.data?.checks?.kafka || 'unknown'}`}>
            {health.data?.checks?.kafka ?? "Unknown"}
          </span>
        </div>
      </div>

      <div className="panel-header" style={{ marginTop: '48px' }}>
        <h3>Sync Status Progress</h3>
      </div>
      
      {sync.loading && <p>Loading sync state...</p>}
      {sync.error && <p className="error">{sync.error}</p>}
      
      {sync.data ? (
        <div className="sync-list">
          {sync.data.pairs.map((item) => (
            <article key={item.pair} className="sync-item">
              <strong>{item.pair}</strong>
              <span className={item.backfill_completed ? "text-up" : "text-muted"}>
                {item.backfill_completed ? "Complete" : "Catching up"}
              </span>
              <span className="text-muted">Last: {item.last_synced_unix ? formatUnix(item.last_synced_unix).split(", ")[0] : "None"}</span>
              <span className="text-muted">Target: {item.next_target_unix ? formatUnix(item.next_target_unix).split(", ")[0] : "None"}</span>
            </article>
          ))}
        </div>
      ) : null}
    </section>
  );
}
