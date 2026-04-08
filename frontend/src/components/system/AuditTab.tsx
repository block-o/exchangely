import { useEffect, useState } from "react";
import { API_BASE_URL } from "../../api/client";
import {
  type Task,
  type TasksResponse,
  TASK_TYPES,
  TYPE_LABELS,
  RECENT_TASK_STATUSES,
  STATUS_LABELS,
  formatDateTime,
  formatShortDate,
  taskStatusLabel,
  MultiFilter,
  Pagination,
  FailureStatus,
} from "./shared";

const UPCOMING_LIMIT = 10;
const RECENT_LIMIT = 10;

export function AuditTab() {
  const [data, setData] = useState<{ upcoming: Task[]; recent: Task[] }>({ upcoming: [], recent: [] });
  const [upcomingTotal, setUpcomingTotal] = useState(0);
  const [recentTotal, setRecentTotal] = useState(0);

  const [upcomingPage, setUpcomingPage] = useState(1);
  const [recentPage, setRecentPage] = useState(1);
  const [upcomingFilter, setUpcomingFilter] = useState<Set<string>>(new Set(TASK_TYPES));
  const [recentFilter, setRecentFilter] = useState<Set<string>>(new Set(TASK_TYPES));
  const [recentStatusFilter, setRecentStatusFilter] = useState<Set<string>>(new Set(RECENT_TASK_STATUSES));

  const fetchTasks = async () => {
    const typeFilter = Array.from(recentFilter).join(",");
    const statusFilter = Array.from(recentStatusFilter).join(",");
    const url = `${API_BASE_URL}/system/tasks?upcoming_limit=${UPCOMING_LIMIT}&upcoming_page=${upcomingPage}&recent_limit=${RECENT_LIMIT}&recent_page=${recentPage}&type=${typeFilter}&status=${statusFilter}`;

    try {
      const res = await fetch(url);
      const json: TasksResponse = await res.json();
      setRecentTotal(json.recentTotal);
      setUpcomingTotal(json.upcomingTotal);
      setData({ upcoming: json.upcoming || [], recent: json.recent || [] });
    } catch (e) {
      console.error("Failed to fetch tasks", e);
    }
  };

  useEffect(() => {
    fetchTasks();
  }, []);

  useEffect(() => {
    fetchTasks();
  }, [upcomingPage, recentPage, recentFilter, recentStatusFilter]);

  // SSE for live updates only on the first page of both panes.
  useEffect(() => {
    if (upcomingPage !== 1 || recentPage !== 1) return;

    const typeFilter = Array.from(recentFilter).join(",");
    const statusFilter = Array.from(recentStatusFilter).join(",");
    const es = new EventSource(
      `${API_BASE_URL}/system/tasks/stream?upcoming_limit=${UPCOMING_LIMIT}&recent_limit=${RECENT_LIMIT}&type=${typeFilter}&status=${statusFilter}`
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

  const filteredUpcoming = data.upcoming.filter(
    (t) => upcomingFilter.size === 0 || upcomingFilter.has(t.type)
  );

  const filteredRecent = data.recent.filter(
    (t) =>
      (recentFilter.size === 0 || recentFilter.has(t.type)) &&
      (recentStatusFilter.size === 0 || recentStatusFilter.has(t.status || ""))
  );

  return (
    <div style={{ display: "flex", flexWrap: "wrap", gap: "2rem" }}>
      {/* Upcoming */}
      <div
        style={{
          flex: "1 1 500px",
          padding: "1rem",
          backgroundColor: "var(--surface-color)",
          borderRadius: "12px",
          display: "flex",
          flexDirection: "column",
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

      {/* Recent */}
      <div
        style={{
          flex: "1 1 600px",
          padding: "1rem",
          backgroundColor: "var(--surface-color)",
          borderRadius: "12px",
          display: "flex",
          flexDirection: "column",
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
                setRecentPage(1);
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
  );
}
