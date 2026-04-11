import { useCallback, useEffect, useRef, useState } from "react";
import { API_BASE_URL, authFetch, authEventSource } from "../../api/client";
import {
  type Task,
  type TasksResponse,
  TASK_TYPES,
  TYPE_LABELS,
  RECENT_TASK_STATUSES,
  STATUS_LABELS,
  formatDateTime,
  compactDescription,
  MultiFilter,
  Pagination,
  TaskTypeIcon,
  DescriptionCell,
} from "./shared";

const UPCOMING_LIMIT = 10;
const LOG_PAGE_SIZE = 50;

function buildLogLine(t: Task): string {
  const time = formatDateTime(t.completed_at);
  const level = t.status === "failed" ? "ERROR" : "OK";
  const type = TYPE_LABELS[t.type] ?? t.type;
  const pair = t.pair || "";
  const desc = compactDescription(t);

  // Build a natural sentence: "Historical Backfill BTCUSD — Hourly candles from ..."
  let message = type;
  if (pair) message += ` ${pair}`;
  if (desc && desc !== "—") message += ` — ${desc}`;
  if (t.status === "failed" && t.last_error) message += ` [${t.last_error}]`;

  return `${time}  ${level}  ${message}`;
}

function TaskLogViewer({
  tasks,
  search,
  hasMore,
  loadingMore,
  onLoadMore,
}: {
  tasks: Task[];
  search: string;
  hasMore: boolean;
  loadingMore: boolean;
  onLoadMore: () => void;
}) {
  const sentinelRef = useRef<HTMLDivElement>(null);
  const query = search.toLowerCase();

  const filtered = query
    ? tasks.filter((t) => buildLogLine(t).toLowerCase().includes(query))
    : tasks;

  // When searching and there are more pages, keep loading them automatically
  useEffect(() => {
    if (!query || !hasMore || loadingMore) return;
    onLoadMore();
  }, [query, hasMore, loadingMore, onLoadMore, tasks.length]);

  // Infinite scroll: observe the sentinel element at the bottom
  useEffect(() => {
    if (!hasMore || loadingMore || query) return;
    const sentinel = sentinelRef.current;
    if (!sentinel) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) onLoadMore();
      },
      { threshold: 0.1 }
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [hasMore, loadingMore, query, onLoadMore]);

  const searching = query && hasMore;

  return (
    <div
      className="task-log-viewer"
      role="log"
      aria-label="Task outcome log"
    >
      {filtered.length === 0 && !searching ? (
        <div style={{ opacity: 0.4, padding: "1.5rem 0", textAlign: "center" }}>
          {query ? "No matching log entries." : "No completed tasks found."}
        </div>
      ) : (
        <>
          {filtered.map((t) => (
            <div
              key={t.id}
              className={`task-log-line ${t.status === "failed" ? "task-log-error" : "task-log-ok"}`}
            >
              {buildLogLine(t)}
            </div>
          ))}
          {searching && (
            <div style={{ padding: "0.5rem 1rem", opacity: 0.4, fontSize: "0.75rem", textAlign: "center" }}>
              Searching… ({tasks.length} entries loaded)
            </div>
          )}
          {!query && hasMore && (
            <div ref={sentinelRef} style={{ padding: "0.5rem 1rem", opacity: 0.4, fontSize: "0.75rem", textAlign: "center" }}>
              {loadingMore ? "Loading…" : ""}
            </div>
          )}
        </>
      )}
    </div>
  );
}

export function AuditTab() {
  const [upcoming, setUpcoming] = useState<Task[]>([]);
  const [upcomingTotal, setUpcomingTotal] = useState(0);
  const [upcomingPage, setUpcomingPage] = useState(1);
  const [upcomingFilter, setUpcomingFilter] = useState<Set<string>>(new Set(TASK_TYPES));

  // Log state: accumulated entries with infinite scroll
  const [logEntries, setLogEntries] = useState<Task[]>([]);
  const [logTotal, setLogTotal] = useState(0);
  const [logPage, setLogPage] = useState(1);
  const [loadingMore, setLoadingMore] = useState(false);
  const [recentFilter, setRecentFilter] = useState<Set<string>>(new Set(TASK_TYPES));
  const [recentStatusFilter, setRecentStatusFilter] = useState<Set<string>>(new Set(RECENT_TASK_STATUSES));
  const [logSearch, setLogSearch] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);

  const hasMore = logEntries.length < logTotal;

  // Build the query string for recent filters
  const buildFilterParams = useCallback(() => {
    const typeFilter = Array.from(recentFilter).join(",");
    const statusFilter = Array.from(recentStatusFilter).join(",");
    return `type=${typeFilter}&status=${statusFilter}`;
  }, [recentFilter, recentStatusFilter]);

  // Fetch initial data (upcoming + first page of logs)
  const fetchInitial = useCallback(async () => {
    const filterParams = buildFilterParams();
    const url = `${API_BASE_URL}/system/tasks?upcoming_limit=${UPCOMING_LIMIT}&upcoming_page=${upcomingPage}&recent_limit=${LOG_PAGE_SIZE}&recent_page=1&${filterParams}`;

    try {
      const res = await authFetch(url);
      const json: TasksResponse = await res.json();
      setUpcomingTotal(json.upcomingTotal);
      setUpcoming(json.upcoming || []);
      setLogTotal(json.recentTotal);
      setLogEntries(json.recent || []);
      setLogPage(1);
    } catch (e) {
      console.error("Failed to fetch tasks", e);
    }
  }, [upcomingPage, buildFilterParams]);

  // Fetch next page of logs and append
  const fetchMoreLogs = useCallback(async () => {
    if (loadingMore || !hasMore) return;
    setLoadingMore(true);
    const nextPage = logPage + 1;
    const filterParams = buildFilterParams();
    const url = `${API_BASE_URL}/system/tasks?upcoming_limit=0&upcoming_page=1&recent_limit=${LOG_PAGE_SIZE}&recent_page=${nextPage}&${filterParams}`;

    try {
      const res = await authFetch(url);
      const json: TasksResponse = await res.json();
      const newEntries = json.recent || [];
      if (newEntries.length > 0) {
        // Deduplicate by id
        setLogEntries((prev) => {
          const existingIds = new Set(prev.map((t) => t.id));
          const unique = newEntries.filter((t) => !existingIds.has(t.id));
          return [...prev, ...unique];
        });
        setLogPage(nextPage);
      }
      setLogTotal(json.recentTotal);
    } catch (e) {
      console.error("Failed to fetch more logs", e);
    } finally {
      setLoadingMore(false);
    }
  }, [loadingMore, hasMore, logPage, buildFilterParams]);

  useEffect(() => { fetchInitial(); }, [fetchInitial]);

  // SSE: keep the first page fresh with live updates (prepend new entries)
  useEffect(() => {
    if (upcomingPage !== 1) return;
    const filterParams = buildFilterParams();
    const es = authEventSource(
      `${API_BASE_URL}/system/tasks/stream?upcoming_limit=${UPCOMING_LIMIT}&recent_limit=${LOG_PAGE_SIZE}&${filterParams}`
    );
    es.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data);
        setUpcomingTotal(parsed.upcomingTotal ?? 0);
        setUpcoming((parsed.upcoming || []).slice(0, UPCOMING_LIMIT));

        // Merge SSE entries into accumulated log (new entries at the top)
        const sseRecent: Task[] = (parsed.recent || []).slice(0, LOG_PAGE_SIZE);
        setLogTotal(parsed.recentTotal ?? 0);
        setLogEntries((prev) => {
          const existingIds = new Set(prev.map((t) => t.id));
          const brandNew = sseRecent.filter((t) => !existingIds.has(t.id));
          return brandNew.length > 0 ? [...brandNew, ...prev] : prev;
        });
      } catch (e) {
        console.error("Failed to parse SSE task data", e);
      }
    };
    return () => es.close();
  }, [upcomingPage, buildFilterParams]);

  const filteredUpcoming = upcoming.filter(
    (t) => upcomingFilter.size === 0 || upcomingFilter.has(t.type)
  );

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1.5rem" }}>
      {/* Upcoming — table */}
      <div
        style={{
          padding: "1rem",
          backgroundColor: "var(--surface-color)",
          borderRadius: "12px",
          display: "flex",
          flexDirection: "column",
        }}
      >
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "1rem" }}>
          <h3 style={{ fontSize: "1rem", margin: 0 }}>Upcoming / Pending Tasks</h3>
          <MultiFilter
            title="Type"
            allOptions={TASK_TYPES}
            selected={upcomingFilter}
            onChange={(next) => { setUpcomingFilter(next); setUpcomingPage(1); }}
          />
        </div>

        <div style={{ flex: 1, minHeight: "200px", overflowX: "auto" }}>
          {filteredUpcoming.length === 0 ? (
            <p style={{ opacity: 0.6, fontSize: "0.9rem" }}>No pending tasks detected.</p>
          ) : (
            <table className="data-table" style={{ fontSize: "0.85rem" }}>
              <thead>
                <tr>
                  <th style={{ textAlign: "left" }}>Type</th>
                  <th>Pair</th>
                  <th>Cadence</th>
                  <th style={{ textAlign: "left" }}>Description</th>
                </tr>
              </thead>
              <tbody>
                {filteredUpcoming.map((t) => (
                  <tr key={t.id}>
                    <td style={{ textAlign: "left" }}><TaskTypeIcon type={t.type} /></td>
                    <td>{t.pair}</td>
                    <td style={{ opacity: 0.7 }}>{t.interval}</td>
                    <DescriptionCell task={t} />
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <Pagination total={upcomingTotal} limit={UPCOMING_LIMIT} page={upcomingPage} onPageChange={setUpcomingPage} />
      </div>

      {/* Task Log */}
      <div
        style={{
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
            marginBottom: "0.75rem",
            flexWrap: "wrap",
            gap: "0.5rem",
          }}
        >
          <h3 style={{ fontSize: "1rem", margin: 0 }}>Task Log</h3>
          <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
            {searchOpen && (
              <input
                type="text"
                value={logSearch}
                onChange={(e) => setLogSearch(e.target.value)}
                placeholder="Search logs…"
                autoFocus
                style={{
                  background: "rgba(255,255,255,0.06)",
                  border: "1px solid rgba(255,255,255,0.15)",
                  borderRadius: "6px",
                  color: "inherit",
                  fontSize: "0.82rem",
                  padding: "0.3rem 0.6rem",
                  width: "180px",
                  outline: "none",
                }}
                aria-label="Search task logs"
              />
            )}
            <button
              onClick={() => { setSearchOpen((v) => !v); if (searchOpen) setLogSearch(""); }}
              title="Search"
              aria-label="Toggle search"
              style={{
                background: searchOpen ? "rgba(255,255,255,0.1)" : "rgba(255,255,255,0.04)",
                border: "1px solid rgba(255,255,255,0.12)",
                borderRadius: "6px",
                color: "inherit",
                cursor: "pointer",
                padding: "0.3rem 0.5rem",
                display: "inline-flex",
                alignItems: "center",
              }}
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="11" cy="11" r="8" /><line x1="21" y1="21" x2="16.65" y2="16.65" />
              </svg>
            </button>
            <MultiFilter
              title="Status"
              allOptions={RECENT_TASK_STATUSES}
              labels={STATUS_LABELS}
              selected={recentStatusFilter}
              onChange={(next) => { setRecentStatusFilter(next); }}
            />
            <MultiFilter
              title="Type"
              allOptions={TASK_TYPES}
              selected={recentFilter}
              onChange={(next) => { setRecentFilter(next); }}
            />
          </div>
        </div>

        <TaskLogViewer
          tasks={logEntries}
          search={logSearch}
          hasMore={hasMore}
          loadingMore={loadingMore}
          onLoadMore={fetchMoreLogs}
        />
      </div>
    </div>
  );
}
