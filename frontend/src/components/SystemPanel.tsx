import React, { useEffect, useState, useRef } from "react";
import { dismissWarning as dismissSystemWarning, fetchWarnings as fetchSystemWarnings } from "../api/system";
import type { ActiveWarning } from "../types/api";

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
  upcomingLimit: number;
  upcomingPage: number;
  recent: Task[];
  recentTotal: number;
  recentLimit: number;
  recentPage: number;
}

// All known task types in display order
const TASK_TYPES = [
  "historical_backfill",
  "live_ticker",
  "integrity_check",
  "consolidation",
  "task_cleanup",
];

const TYPE_LABELS: Record<string, string> = {
  historical_backfill: "Historical Backfill",
  live_ticker: "Live Ticker",
  integrity_check: "Integrity Check",
  consolidation: "Consolidation",
  task_cleanup: "Task Log Cleanup",
};

const RECENT_TASK_STATUSES = ["completed", "failed"];

const STATUS_LABELS: Record<string, string> = {
  completed: "Completed",
  failed: "Failed",
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

function taskStatusLabel(status?: string) {
  switch (status) {
    case "running":
      return "Ongoing";
    case "failed":
      return "Failed";
    case "completed":
      return "Completed";
    case "scheduled":
      return "Scheduled";
    case "pending":
      return "Pending";
    default:
      return status || "Pending";
  }
}

// ── Multi filter dropdown ───────────────────────────────────────────────────
interface MultiFilterProps {
  allOptions: string[];
  labels?: Record<string, string>;
  title: string;
  selected: Set<string>;
  onChange: (next: Set<string>) => void;
}

function MultiFilter({ allOptions, labels, title, selected, onChange }: MultiFilterProps) {
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
    onChange(new Set(allOptions));
  }

  function clearAll() {
    onChange(new Set());
  }

  const label =
    selected.size === 0
      ? `${title}: None`
      : selected.size === allOptions.length
      ? `${title}: All`
      : `${title}: ${selected.size} selected`;

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
        {label}{" "}
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
          {allOptions.map((type) => (
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
              {labels?.[type] ?? TYPE_LABELS[type] ?? type}
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

function FailureStatus({ reason }: { reason: string }) {
  const [open, setOpen] = useState(false);

  return (
    <span
      style={{ position: "relative", display: "inline-flex", alignItems: "center" }}
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
      onFocus={() => setOpen(true)}
      onBlur={() => setOpen(false)}
    >
      <span
        className="text-down"
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: "0.35rem",
          textDecoration: "underline dotted",
          textUnderlineOffset: "2px",
          cursor: "help",
        }}
        title={reason}
        tabIndex={0}
        aria-label={`Failed: ${reason}`}
      >
        Failed
        <span
          aria-hidden="true"
          style={{
            display: "inline-flex",
            alignItems: "center",
            justifyContent: "center",
            width: "1rem",
            height: "1rem",
            borderRadius: "999px",
            border: "1px solid currentColor",
            fontSize: "0.7rem",
            lineHeight: 1,
          }}
        >
          i
        </span>
      </span>

      {open && (
        <span
          role="tooltip"
          style={{
            position: "absolute",
            left: 0,
            top: "calc(100% + 0.45rem)",
            zIndex: 20,
            minWidth: "220px",
            maxWidth: "320px",
            padding: "0.6rem 0.7rem",
            borderRadius: "8px",
            border: "1px solid rgba(255,107,107,0.35)",
            background: "rgba(15,18,28,0.96)",
            boxShadow: "0 12px 28px rgba(0,0,0,0.35)",
            color: "rgba(255,255,255,0.92)",
            fontSize: "0.78rem",
            lineHeight: 1.45,
            whiteSpace: "normal",
          }}
        >
          {reason}
        </span>
      )}
    </span>
  );
}

// ── Main Panel ──────────────────────────────────────────────────────────────
export function SystemPanel() {
  const [data, setData] = useState<{ upcoming: Task[]; recent: Task[] }>({ upcoming: [], recent: [] });
  const [upcomingTotal, setUpcomingTotal] = useState(0);
  const [recentTotal, setRecentTotal] = useState(0);
  const [apiVersion, setApiVersion] = useState<string>("Unknown");
  const [warnings, setWarnings] = useState<ActiveWarning[]>([]);
  const [warningsLoading, setWarningsLoading] = useState(true);
  const [dismissingWarningIds, setDismissingWarningIds] = useState<Set<string>>(new Set());

  const [upcomingPage, setUpcomingPage] = useState(1);
  const [recentPage, setRecentPage] = useState(1);
  const [upcomingFilter, setUpcomingFilter] = useState<Set<string>>(new Set(TASK_TYPES));
  const [recentFilter, setRecentFilter] = useState<Set<string>>(new Set(TASK_TYPES));
  const [recentStatusFilter, setRecentStatusFilter] = useState<Set<string>>(new Set(RECENT_TASK_STATUSES));
  
  const UPCOMING_LIMIT = 10;
  const RECENT_LIMIT = 10;

  // @ts-ignore - injected by vite build
  const uiVersion = typeof __APP_VERSION__ !== "undefined" ? __APP_VERSION__ : "Unknown";

  const fetchWarnings = async () => {
    try {
      setWarnings(await fetchSystemWarnings());
    } catch (e) {
      console.error("Failed to fetch active warnings", e);
      setWarnings([
        {
          id: "warnings-unavailable",
          level: "warning",
          title: "Warnings unavailable",
          detail: "Health and synchronization warnings could not be loaded.",
          fingerprint: "warnings-unavailable",
        },
      ]);
    } finally {
      setWarningsLoading(false);
    }
  };

  const fetchTasks = async () => {
    const typeFilter = Array.from(recentFilter).join(",");
    const statusFilter = Array.from(recentStatusFilter).join(",");
    const url = `${import.meta.env.VITE_API_BASE_URL}/system/tasks?upcoming_limit=${UPCOMING_LIMIT}&upcoming_page=${upcomingPage}&recent_limit=${RECENT_LIMIT}&recent_page=${recentPage}&type=${typeFilter}&status=${statusFilter}`;
    
    try {
      const res = await fetch(url);
      const json: TasksResponse = await res.json();
      setRecentTotal(json.recentTotal);
      setUpcomingTotal(json.upcomingTotal);

      setData({
        upcoming: json.upcoming || [],
        recent: json.recent || [],
      });
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
    fetchWarnings();
  }, []);

  useEffect(() => {
    const interval = window.setInterval(() => {
      fetchWarnings();
    }, 15000);

    return () => window.clearInterval(interval);
  }, []);

  // Re-fetch when paging or the recent-task filter changes
  useEffect(() => {
    fetchTasks();
  }, [upcomingPage, recentPage, recentFilter, recentStatusFilter]);

  // SSE for live updates only on the first page of both panes.
  useEffect(() => {
    if (upcomingPage !== 1 || recentPage !== 1) return;

    const typeFilter = Array.from(recentFilter).join(",");
    const statusFilter = Array.from(recentStatusFilter).join(",");
    const es = new EventSource(
      `${import.meta.env.VITE_API_BASE_URL}/system/tasks/stream?upcoming_limit=${UPCOMING_LIMIT}&recent_limit=${RECENT_LIMIT}&type=${typeFilter}&status=${statusFilter}`
    );
    es.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data);
        setUpcomingTotal(parsed.upcomingTotal ?? 0);
        setRecentTotal(parsed.recentTotal ?? 0);
        setData({
          upcoming: (parsed.upcoming || []).slice(0, UPCOMING_LIMIT),
          recent: (parsed.recent || []).slice(0, RECENT_LIMIT),
        });
      } catch (e) {
        console.error("Failed to parse SSE task data", e);
      }
    };
    return () => es.close();
  }, [upcomingPage, recentPage, recentFilter, recentStatusFilter]);

  // Apply frontend filtering on top of whatever the API/SSE provides
  const filteredUpcoming = data.upcoming.filter(
    (t) => upcomingFilter.size === 0 || upcomingFilter.has(t.type)
  );
  
  const filteredRecent = data.recent.filter(
    (t) =>
      (recentFilter.size === 0 || recentFilter.has(t.type)) &&
      (recentStatusFilter.size === 0 || recentStatusFilter.has(t.status || ""))
  );

  async function dismissWarning(warning: ActiveWarning) {
    setDismissingWarningIds((previous) => new Set(previous).add(warning.id));

    try {
      await dismissSystemWarning(warning.id, warning.fingerprint);
      setWarnings((previous) => previous.filter((item) => item.id !== warning.id));
    } catch (e) {
      console.error("Failed to dismiss warning", e);
    } finally {
      setDismissingWarningIds((previous) => {
        const next = new Set(previous);
        next.delete(warning.id);
        return next;
      });
    }
  }

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

      <div
        style={{
          marginTop: "1rem",
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
            <h3 style={{ fontSize: "1rem", margin: 0 }}>Active Warnings</h3>
            <p style={{ margin: "0.35rem 0 0", opacity: 0.7, fontSize: "0.85rem" }}>
              Current platform risks derived from health, sync progress, and failed tasks.
            </p>
          </div>
          <div style={{ fontSize: "0.82rem", opacity: 0.7 }}>
            {warningsLoading ? "Loading…" : `${warnings.length} active`}
          </div>
        </div>

        {warningsLoading ? (
          <p style={{ opacity: 0.6, fontSize: "0.9rem", margin: 0 }}>Loading active warnings…</p>
        ) : warnings.length === 0 ? (
          <p style={{ opacity: 0.6, fontSize: "0.9rem", margin: 0 }}>
            No active warnings. Health checks are passing and backfills are caught up.
          </p>
        ) : (
          <div
            style={{
              display: "flex",
              gap: "0.75rem",
              overflowX: "auto",
              paddingBottom: "0.25rem",
              scrollSnapType: "x proximity",
            }}
          >
            {warnings.map((warning) => (
              <article
                key={warning.id}
                style={{
                  flex: "0 0 min(320px, calc(100vw - 5rem))",
                  padding: "0.9rem 1rem",
                  borderRadius: "10px",
                  border: `1px solid ${
                    warning.level === "error" ? "rgba(255,107,107,0.45)" : "rgba(255,196,61,0.35)"
                  }`,
                  background:
                    warning.level === "error" ? "rgba(120,28,28,0.18)" : "rgba(140,96,0,0.14)",
                  scrollSnapAlign: "start",
                }}
              >
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: "0.75rem",
                    marginBottom: "0.45rem",
                  }}
                >
                  <strong style={{ fontSize: "0.92rem" }}>{warning.title}</strong>
                  <span
                    style={{ display: "inline-flex", alignItems: "center", gap: "0.5rem" }}
                  >
                    <span
                      style={{
                        fontSize: "0.72rem",
                        letterSpacing: "0.04em",
                        textTransform: "uppercase",
                        opacity: 0.75,
                      }}
                    >
                      {warning.level}
                    </span>
                    <button
                      type="button"
                      onClick={() => dismissWarning(warning)}
                      aria-label={`Dismiss warning ${warning.title}`}
                      disabled={dismissingWarningIds.has(warning.id)}
                      style={{
                        background: "transparent",
                        border: "1px solid rgba(255,255,255,0.18)",
                        borderRadius: "999px",
                        color: "inherit",
                        cursor: dismissingWarningIds.has(warning.id) ? "default" : "pointer",
                        fontSize: "0.9rem",
                        lineHeight: 1,
                        opacity: dismissingWarningIds.has(warning.id) ? 0.45 : 1,
                        padding: "0.12rem 0.42rem",
                      }}
                    >
                      ×
                    </button>
                  </span>
                </div>
                <p style={{ margin: 0, fontSize: "0.84rem", lineHeight: 1.45, opacity: 0.86 }}>
                  {warning.detail}
                </p>
              </article>
            ))}
          </div>
        )}
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
              <MultiFilter
                title="Type"
                allOptions={TASK_TYPES}
                selected={upcomingFilter}
                onChange={(next) => {
                  setUpcomingFilter(next);
                  setUpcomingPage(1);
                }}
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
                        {t.status === "running"
                          ? "Ongoing"
                          : t.status === "scheduled"
                          ? t.type === "historical_backfill"
                            ? `Backfill: ${formatShortDate(t.window_start)} → ${formatShortDate(t.window_end)}`
                            : `Next: ${formatShortDate(t.window_start)}`
                          : taskStatusLabel(t.status)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          <Pagination
            total={upcomingTotal}
            limit={UPCOMING_LIMIT}
            page={upcomingPage}
            onPageChange={setUpcomingPage}
          />
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
            <h3 style={{ fontSize: "1rem", margin: 0 }}>Recent Task Outcomes</h3>
            <div style={{ display: "flex", gap: "0.75rem", alignItems: "center" }}>
              <MultiFilter
                title="Status"
                allOptions={RECENT_TASK_STATUSES}
                labels={STATUS_LABELS}
                selected={recentStatusFilter}
                onChange={(next) => {
                  setRecentStatusFilter(next);
                  setRecentPage(1);
                }}
              />
              <MultiFilter
                title="Type"
                allOptions={TASK_TYPES}
                selected={recentFilter}
                onChange={(next) => {
                  setRecentFilter(next);
                  setRecentPage(1); // Reset to page 1 on filter change
                }}
              />
            </div>
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
                    <th>Updated At</th>
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
                        >
                          {TYPE_LABELS[t.type] ?? t.type}
                        </span>
                      </td>
                      <td>{t.pair}</td>
                      <td style={{ opacity: 0.75, fontSize: "0.8rem", whiteSpace: "nowrap" }}>
                        {formatDateTime(t.completed_at)}
                      </td>
                      <td>
                        {t.status === "failed" && t.last_error ? (
                          <FailureStatus reason={t.last_error} />
                        ) : (
                          <span className={t.status === "failed" ? "text-down" : "text-up"}>
                            {taskStatusLabel(t.status)}
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
          
          <Pagination 
            total={recentTotal} 
            limit={RECENT_LIMIT} 
            page={recentPage} 
            onPageChange={setRecentPage} 
          />
        </div>
      </div>
    </section>
  );
}
