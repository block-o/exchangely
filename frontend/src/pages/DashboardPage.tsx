import { fetchHealth } from "../api/health";
import { fetchSyncStatus } from "../api/system";
import { StatusCard } from "../components/status/StatusCard";
import { useApi } from "../hooks/useApi";

export function DashboardPage() {
  const health = useApi(fetchHealth);
  const sync = useApi(fetchSyncStatus);

  return (
    <section id="dashboard" className="panel">
      <div className="panel-header">
        <h2>Dashboard</h2>
        <p>Top-level operational view for the API, Kafka, and TimescaleDB.</p>
      </div>
      <div className="status-grid">
        <StatusCard label="API Status" value={health.data?.status ?? (health.loading ? "Loading" : "Unknown")} />
        <StatusCard label="Planner Leader" value={sync.data?.planner_leader ?? "Unknown"} />
        <StatusCard label="TimescaleDB" value={health.data?.checks.timescaledb ?? "Unknown"} />
        <StatusCard label="Kafka" value={health.data?.checks.kafka ?? "Unknown"} />
      </div>
      {health.error ? <p className="error">{health.error}</p> : null}
    </section>
  );
}
