import React, { useEffect, useState, useRef } from "react";

export interface Task {
  id: string;
  type: string;
  pair: string;
  interval: string;
  window_start: string;
  window_end: string;
  status?: string;
  last_error?: string;
  completed_at?: string;
}

interface TasksResponse {
  upcoming: Task[];
  upcomingTotal: number;
  recent: Task[];
  recentTotal: number;
  limit: number;
  page: number;
}

// All known task types in display order
const TASK_TYPES = [
  "historical_sweep",
  "live_ticker",
  "integrity_check",
  "consolidation",
  "task_cleanup",
];

const TYPE_LABELS: Record<string, string> = {
  historical_sweep: "Historical Sweep",
  live_ticker: "Live Ticker",
  integrity_check: "Integrity Check",
  consolidation: "Consolidation",
  task_cleanup: "Task Log Cleanup",
};

function formatDateTime(isoString?: string) {
  if (!isoString) return "—";
  const d = new Date(isoString);
  return d.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function formatShortDate(isoString?: string) {
  if (!isoString) return "";
  const d = new Date(isoString);
  return d.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

// ── Type filter dropdown ────────────────────────────────────────────────────
interface TypeFilterProps {
  allTypes: string[];
  selected: Set<string>;
  onChange: (next: Set<string>) => void;
}

function TypeFilter({ allTypes, selected, onChange }: TypeFilterProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  function toggle(type: string) {
    const next = new Set(selected);
    if (next.has(type)) {
      next.delete(type);
    } else {
      next.add(type);
    }
    onChange(next);
  }

  function selectAll() {
    onChange(new Set(allTypes));
  }

  function clearAll() {
    onChange(new Set());
  }

  const label =
    selected.size === 0
      ? "None"
      : selected.size === allTypes.length
      ? "All types"
      : `${selected.size} type${selected.size > 1 ? "s" : ""}`;

  return (
    <div ref={ref} style={{ position: "relative", display: "inline-block" }}>
      <button
        onClick={() => setOpen((v) => !v)}
        style={{
          background: "var(--surface-color)",
          border: "1px solid rgba(255,255,255,0.12)",
          borderRadius: "8px",
          color: "inherit",
          cursor: "pointer",
          display: "flex",
          alignItems: "center",
          gap: "0.4rem",
          fontSize: "0.82rem",
          padding: "0.3rem 0.7rem",
        }}
      >
        <span style={{ opacity: 0.7 }}>Filter:</span> {label}{" "}
        <span style={{ fontSize: "0.7rem", opacity: 0.6 }}>{open ? "▲" : "▼"}</span>
      </button>

      {open && (
        <div
          style={{
            position: "absolute",
            top: "calc(100% + 6px)",
            right: 0,
            zIndex: 50,
            background: "var(--bg-secondary, #1a1f2e)",
            border: "1px solid rgba(255,255,255,0.12)",
            borderRadius: "10px",
            padding: "0.5rem",
            minWidth: "190px",
            boxShadow: "0 8px 32px rgba(0,0,0,0.5)",
          }}
        >
          <div
            style={{
              display: "flex",
              justifyContent: "space-between",
              marginBottom: "0.4rem",
              padding: "0 0.3rem",
            }}
          >
            <button
              onClick={selectAll}
              style={{ background: "none", border: "none", color: "var(--accent-color, #7c6fff)", cursor: "pointer", fontSize: "0.78rem" }}
            >
              All
            </button>
            <button
              onClick={clearAll}
              style={{ background: "none", border: "none", color: "rgba(255,255,255,0.5)", cursor: "pointer", fontSize: "0.78rem" }}
            >
              None
            </button>
          </div>
          {allTypes.map((type) => (
            <label
              key={type}
              style={{
                display: "flex",
                alignItems: "center",
                gap: "0.5rem",
                padding: "0.3rem 0.3rem",
                borderRadius: "6px",
                cursor: "pointer",
                fontSize: "0.85rem",
                transition: "background 0.15s",
              }}
              onMouseEnter={(e) => ((e.currentTarget as HTMLElement).style.background = "rgba(255,255,255,0.06)")}
              onMouseLeave={(e) => ((e.currentTarget as HTMLElement).style.background = "none")}
            >
              <input
                type="checkbox"
                checked={selected.has(type)}
                onChange={() => toggle(type)}
                style={{ accentColor: "var(--accent-color, #7c6fff)", width: "14px", height: "14px" }}
              />
              {TYPE_LABELS[type] ?? type}
            </label>
          ))}
        </div>
      )}
    </div>
  );
}

// ── Pagination Controls ─────────────────────────────────────────────────────
interface PaginationProps {
  total: number;
  limit: number;
  page: number;
  onPageChange: (next: number) => void;
}

function Pagination({ total, limit, page, onPageChange }: PaginationProps) {
  const totalPages = Math.ceil(total / limit) || 1;
  const canPrev = page > 1;
  const canNext = page < totalPages;

  return (
    <div style={{ display: "flex", alignItems: "center", gap: "1rem", marginTop: "1rem", fontSize: "0.85rem" }}>
      <button
        disabled={!canPrev}
        onClick={() => onPageChange(page - 1)}
        style={{
          background: "rgba(255,255,255,0.06)",
          border: "1px solid rgba(255,255,255,0.1)",
          borderRadius: "6px",
          color: canPrev ? "inherit" : "rgba(255,255,255,0.2)",
          cursor: canPrev ? "pointer" : "default",
          padding: "0.3rem 0.6rem",
        }}
      >
        Previous
      </button>
      <span style={{ opacity: 0.6 }}>
        Page {page} of {totalPages} <span style={{ fontSize: "0.75rem" }}>({total} total)</span>
      </span>
      <button
        disabled={!canNext}
        onClick={() => onPageChange(page + 1)}
        style={{
          background: "rgba(255,255,255,0.06)",
          border: "1px solid rgba(255,255,255,0.1)",
          borderRadius: "6px",
          color: canNext ? "inherit" : "rgba(255,255,255,0.2)",
          cursor: canNext ? "pointer" : "default",
          padding: "0.3rem 0.6rem",
        }}
      >
        Next
      </button>
    </div>
  );
}

// ── Main Panel ──────────────────────────────────────────────────────────────
export function SystemPanel() {
  const [data, setData] = useState<{ upcoming: Task[]; recent: Task[] }>({ upcoming: [], recent: [] });
  const [upcomingTotal, setUpcomingTotal] = useState(0);
  const [recentTotal, setRecentTotal] = useState(0);
  const [apiVersion, setApiVersion] = useState<string>("Unknown");

  const [upcomingPage, setUpcomingPage] = useState(1);
  const [recentPage, setRecentPage] = useState(1);
  const [upcomingFilter, setUpcomingFilter] = useState<Set<string>>(new Set(TASK_TYPES));
  const [recentFilter, setRecentFilter] = useState<Set<string>>(new Set(TASK_TYPES));
  
  const LIMIT = 50;

  // @ts-ignore - injected by vite build
  const uiVersion = typeof __APP_VERSION__ !== "undefined" ? __APP_VERSION__ : "Unknown";

  const fetchTasks = async () => {
    // Note: Upcoming doesn't filter by type in backend yet (as per plan focus), but could.
    // For now, we always fetch page 1 of upcoming but paginate recent.
    const typeFilter = Array.from(recentFilter).join(",");
    const url = `${import.meta.env.VITE_API_BASE_URL}/system/tasks?limit=${LIMIT}&page=${recentPage}&type=${typeFilter}`;
    
    try {
      const res = await fetch(url);
      const json: TasksResponse = await res.json();
      setRecentTotal(json.recentTotal);
      setUpcomingTotal(json.upcomingTotal);
      
      // If we are on page 1 of recent, we also update upcoming from this response
      // But if we are only paginating recent, we keep upcoming from first fetch or SSE
      setData(prev => ({
        upcoming: json.upcoming || prev.upcoming,
        recent: json.recent || []
      }));
    } catch (e) {
      console.error("Failed to fetch tasks", e);
    }
  };

  // Initial fetch and version check
  useEffect(() => {
    fetch(import.meta.env.VITE_API_BASE_URL + "/system/version")
      .then((res) => res.json())
      .then((res) => setApiVersion(res.api_version))
      .catch(console.error);
    
    fetchTasks();
  }, []);

  // Re-fetch when recent page or filters change
  useEffect(() => {
    fetchTasks();
  }, [recentPage, recentFilter]);

  // SSE for live updates (only if on page 1)
  useEffect(() => {
    if (recentPage !== 1) return;

    const es = new EventSource(import.meta.env.VITE_API_BASE_URL + "/system/tasks/stream");
    es.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data);
        // SSE pushes don't include totals yet, we just update the lists
        setData({
          upcoming: parsed.upcoming || [],
          recent: parsed.recent || [],
        });
      } catch (e) {
        console.error("Failed to parse SSE task data", e);
      }
    };
    return () => es.close();
  }, [recentPage]);

  // Apply frontend filtering on top of whatever the API/SSE provides
  const filteredUpcoming = data.upcoming.filter(
    (t) => upcomingFilter.size === 0 || upcomingFilter.has(t.type)
  );
  
  const filteredRecent = data.recent.filter(
    (t) => recentFilter.size === 0 || recentFilter.has(t.type)
  );

  return (
    <section className="panel">
      <div
        className="panel-header"
        style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}
      >
        <div>
          <h2>System Operations</h2>
          <p>Real-time stream of backend planner tasks and states.</p>
        </div>
        <div style={{ textAlign: "right", fontSize: "0.85rem", opacity: 0.8 }}>
          <div>UI: v{uiVersion}</div>
          <div>API: {apiVersion}</div>
        </div>
      </div>

      <div style={{ display: "flex", flexWrap: "wrap", gap: "2rem", marginTop: "1rem" }}>
        {/* ─ Upcoming ─ */}
        <div
          style={{
            flex: "1 1 500px",
            padding: "1rem",
            backgroundColor: "var(--surface-color)",
            borderRadius: "12px",
            display: "flex",
            flexDirection: "column"
          }}
        >
          <div
            style={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
              marginBottom: "1rem",
            }}
          >
            <h3 style={{ fontSize: "1rem", margin: 0 }}>Upcoming / Pending Tasks</h3>
            <div style={{ display: "flex", gap: "1rem", alignItems: "center" }}>
              {recentPage === 1 && (
                <span style={{ 
                  fontSize: "0.65rem", 
                  background: "var(--status-success-bg, rgba(0,255,100,0.1))", 
                  color: "var(--status-success-text, #00ff66)", 
                  padding: "2px 6px", 
                  borderRadius: "4px",
                  textTransform: "uppercase",
                  fontWeight: "bold",
                  letterSpacing: "0.05em"
                }}>Live</span>
              )}
              <TypeFilter
                allTypes={TASK_TYPES}
                selected={upcomingFilter}
                onChange={setUpcomingFilter}
              />
            </div>
          </div>

          <div style={{ flex: 1, minHeight: "300px" }}>
            {filteredUpcoming.length === 0 ? (
              <p style={{ opacity: 0.6, fontSize: "0.9rem" }}>No pending tasks detected.</p>
            ) : (
              <table className="data-table" style={{ fontSize: "0.85rem" }}>
                <thead>
                  <tr>
                    <th style={{ textAlign: "left" }}>Type</th>
                    <th>Pair</th>
                    <th>Cadence</th>
                    <th>Status / Window</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredUpcoming.map((t) => (
                    <tr key={t.id}>
                      <td style={{ textAlign: "left" }}>
                        <span
                          style={{
                            background: "rgba(255,255,255,0.06)",
                            borderRadius: "5px",
                            padding: "1px 6px",
                            fontSize: "0.78rem",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {TYPE_LABELS[t.type] ?? t.type}
                        </span>
                      </td>
                      <td>{t.pair}</td>
                      <td style={{ opacity: 0.7 }}>{t.interval}</td>
                      <td style={{ opacity: 0.85, fontSize: "0.8rem" }}>
                        {t.status === "scheduled"
                          ? t.type === "historical_sweep"
                            ? `Backfill: ${formatShortDate(t.window_start)} → ${formatShortDate(t.window_end)}`
                            : `Next: ${formatShortDate(t.window_start)}`
                          : t.status || "pending"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>

        {/* ─ Recent ─ */}
        <div
          style={{
            flex: "1 1 600px",
            padding: "1rem",
            backgroundColor: "var(--surface-color)",
            borderRadius: "12px",
            display: "flex",
            flexDirection: "column"
          }}
        >
          <div
            style={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
              marginBottom: "1rem",
            }}
          >
            <h3 style={{ fontSize: "1rem", margin: 0 }}>Recently Completed Tasks</h3>
            <TypeFilter
              allTypes={TASK_TYPES}
              selected={recentFilter}
              onChange={(next) => {
                setRecentFilter(next);
                setRecentPage(1); // Reset to page 1 on filter change
              }}
            />
          </div>

          <div style={{ flex: 1, minHeight: "300px" }}>
            {filteredRecent.length === 0 ? (
              <p style={{ opacity: 0.6, fontSize: "0.9rem" }}>No completed tasks found.</p>
            ) : (
              <table className="data-table" style={{ fontSize: "0.85rem" }}>
                <thead>
                  <tr>
                    <th style={{ textAlign: "left" }}>Type</th>
                    <th>Pair</th>
                    <th>Completed At</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredRecent.map((t) => (
                    <tr key={t.id}>
                      <td style={{ textAlign: "left" }}>
                        <span
                          style={{
                            background: "rgba(255,255,255,0.06)",
                            borderRadius: "5px",
                            padding: "1px 6px",
                            fontSize: "0.78rem",
                            whiteSpace: "nowrap",
                          }}
                          title={t.last_error || ""}
                        >
                          {TYPE_LABELS[t.type] ?? t.type}
                        </span>
                      </td>
                      <td>{t.pair}</td>
                      <td style={{ opacity: 0.75, fontSize: "0.8rem", whiteSpace: "nowrap" }}>
                        {formatDateTime(t.completed_at)}
                      </td>
                      <td>
                        <span
                          className={t.status === "failed" ? "text-down" : "text-up"}
                          style={{ display: "inline-flex", alignItems: "center", gap: "0.3rem" }}
                        >
                          {t.status}
                          {t.status === "failed" && t.last_error && (
                            <span title={t.last_error}>⚠️</span>
                          )}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
          
          <Pagination 
            total={recentTotal} 
            limit={LIMIT} 
            page={recentPage} 
            onPageChange={setRecentPage} 
          />
        </div>
      </div>
    </section>
  );
}
