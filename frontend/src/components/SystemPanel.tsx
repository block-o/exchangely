import React, { useEffect, useState } from "react";

export interface Task {
  id: string;
  type: string;
  pair: string;
  interval: string;
  window_start: string;
  window_end: string;
  status?: string;
  last_error?: string;
}

interface SystemData {
  upcoming: Task[];
  recent: Task[];
}

function formatShortDate(isoString?: string) {
  if (!isoString) return "";
  const d = new Date(isoString);
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'});
}


export function SystemPanel() {
  const [data, setData] = useState<SystemData>({ upcoming: [], recent: [] });
  const [apiVersion, setApiVersion] = useState<string>("Unknown");
  
  // @ts-ignore - injected by vite build
  const uiVersion = typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : "Unknown";

  useEffect(() => {
    fetch(import.meta.env.VITE_API_BASE_URL + "/system/version")
      .then((res) => res.json())
      .then((res) => setApiVersion(res.api_version))
      .catch(console.error);

    const es = new EventSource(import.meta.env.VITE_API_BASE_URL + "/system/tasks/stream");
    es.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data);
        setData({
          upcoming: parsed.upcoming || [],
          recent: parsed.recent || [],
        });
      } catch (e) {
        console.error("Failed to parse SSE task data", e);
      }
    };
    return () => es.close();
  }, []);

  return (
    <section className="panel">
      <div className="panel-header" style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <div>
          <h2>System Operations</h2>
          <p>Real-time stream of backend planner tasks and states.</p>
        </div>
        <div style={{ textAlign: "right", fontSize: "0.85rem", opacity: 0.8 }}>
          <div>UI: v{uiVersion}</div>
          <div>API: {apiVersion}</div>
        </div>
      </div>
      
      <div style={{ display: "flex", gap: "2rem", marginTop: "1rem" }}>
        <div style={{ flex: 1, padding: '1rem', backgroundColor: 'var(--surface-color)', borderRadius: '12px' }}>
          <h3 style={{ marginBottom: '1rem', fontSize: '1rem' }}>Upcoming Pending Tasks</h3>
          {data.upcoming.length === 0 ? <p style={{ opacity: 0.6 }}>No upcoming tasks.</p> : (
            <table className="data-table" style={{ fontSize: '0.9rem' }}>
              <thead>
                <tr>
                  <th style={{ textAlign: 'left' }}>Type</th>
                  <th>Pair</th>
                  <th>Interval</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {data.upcoming.map((t) => (
                  <tr key={t.id}>
                    <td style={{ textAlign: 'left' }}>{t.type}</td>
                    <td>{t.pair}</td>
                    <td>{t.interval}</td>
                    <td style={{ opacity: 0.8 }}>
                      {t.status === 'scheduled' 
                        ? (t.type === 'backfill' 
                           ? `Backfill: ${formatShortDate(t.window_start)} to ${formatShortDate(t.window_end)}` 
                           : `Scheduled: ${formatShortDate(t.window_start)}`) 
                        : (t.status || 'pending')}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <div style={{ flex: 1, padding: '1rem', backgroundColor: 'var(--surface-color)', borderRadius: '12px' }}>
          <h3 style={{ marginBottom: '1rem', fontSize: '1rem' }}>Recently Completed Tasks</h3>
          {data.recent.length === 0 ? <p style={{ opacity: 0.6 }}>No recent tasks.</p> : (
            <table className="data-table" style={{ fontSize: '0.9rem' }}>
              <thead>
                <tr>
                  <th style={{ textAlign: 'left' }}>Type</th>
                  <th>Pair</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {data.recent.map((t) => (
                  <tr key={t.id}>
                    <td style={{ textAlign: 'left' }}>
                      <span title={t.last_error || ''}>
                        {t.type}
                      </span>
                    </td>
                    <td>{t.pair}</td>
                    <td className={t.status === "failed" ? "text-down" : "text-up"}>
                      {t.status} {t.status === "failed" && t.last_error && "⚠️"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </section>
  );
}
