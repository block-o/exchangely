import { useEffect, useRef, useState } from "react";

export interface Task {
  id: string;
  type: string;
  pair: string;
  interval: string;
  window_start: string;
  window_end: string;
  status?: string;
  description?: string;
  last_error?: string;
  completed_at?: string;
}

export interface TasksResponse {
  upcoming: Task[];
  upcomingTotal: number;
  upcomingLimit: number;
  upcomingPage: number;
  recent: Task[];
  recentTotal: number;
  recentLimit: number;
  recentPage: number;
}

export const TASK_TYPES = [
  "historical_backfill",
  "live_ticker",
  "integrity_check",
  "consolidation",
  "task_cleanup",
  "news_fetch",
  "gap_validation",
];

export const TYPE_LABELS: Record<string, string> = {
  historical_backfill: "Historical Backfill",
  live_ticker: "Live Ticker",
  integrity_check: "Integrity Check",
  consolidation: "Consolidation",
  task_cleanup: "Task Log Cleanup",
  news_fetch: "News Fetch",
  gap_validation: "Gap Validation",
};

export const RECENT_TASK_STATUSES = ["completed", "failed"];

export const STATUS_LABELS: Record<string, string> = {
  completed: "Completed",
  failed: "Failed",
};

/** Inline SVG icon props matching the app's Feather-style convention. */
const ICON_STYLE = { width: 16, height: 16, viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", strokeWidth: 2, strokeLinecap: "round" as const, strokeLinejoin: "round" as const };

/** SVG paths for each task type, matching the Feather icon style used in the app header. */
const TYPE_ICON_PATHS: Record<string, React.ReactNode> = {
  // archive / box
  historical_backfill: (
    <>
      <path d="M21 8v13H3V8" /><path d="M1 3h22v5H1z" /><path d="M10 12h4" />
    </>
  ),
  // radio / broadcast
  live_ticker: (
    <>
      <path d="M16.72 11.06A10.94 10.94 0 0 1 19 17.94" /><path d="M7.28 11.06A10.94 10.94 0 0 0 5 17.94" /><path d="M14.34 14.18a5 5 0 0 1 1.66 3.76" /><path d="M9.66 14.18a5 5 0 0 0-1.66 3.76" /><circle cx="12" cy="18" r="1" />
    </>
  ),
  // search
  integrity_check: (
    <>
      <circle cx="11" cy="11" r="8" /><line x1="21" y1="21" x2="16.65" y2="16.65" />
    </>
  ),
  // refresh-cw
  consolidation: (
    <>
      <polyline points="23 4 23 10 17 10" /><polyline points="1 20 1 14 7 14" /><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
    </>
  ),
  // trash-2
  task_cleanup: (
    <>
      <polyline points="3 6 5 6 21 6" /><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" /><line x1="10" y1="11" x2="10" y2="17" /><line x1="14" y1="11" x2="14" y2="17" />
    </>
  ),
  // rss
  news_fetch: (
    <>
      <path d="M4 11a9 9 0 0 1 9 9" /><path d="M4 4a16 16 0 0 1 16 16" /><circle cx="5" cy="19" r="1" />
    </>
  ),
  // alert-triangle
  gap_validation: (
    <>
      <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" /><line x1="12" y1="9" x2="12" y2="13" /><line x1="12" y1="17" x2="12.01" y2="17" />
    </>
  ),
};

/** Compact SVG icon for a task type. On desktop shows icon + text label.
 *  On mobile/tablet the label is hidden via CSS and the full name appears on hover. */
export function TaskTypeIcon({ type }: { type: string }) {
  const label = TYPE_LABELS[type] ?? type;
  const paths = TYPE_ICON_PATHS[type] ?? (
    // fallback: file-text
    <>
      <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" /><polyline points="14 2 14 8 20 8" /><line x1="16" y1="13" x2="8" y2="13" /><line x1="16" y1="17" x2="8" y2="17" /><polyline points="10 9 9 9 8 9" />
    </>
  );

  return (
    <span
      style={{ display: "inline-flex", alignItems: "center", gap: "0.4rem", whiteSpace: "nowrap" }}
      title={label}
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        {...ICON_STYLE}
        style={{ opacity: 0.7, flexShrink: 0 }}
        aria-hidden="true"
      >
        {paths}
      </svg>
      <span className="task-type-label">{label}</span>
    </span>
  );
}

/** Plain description cell — shows the compact description as simple text. */
export function DescriptionCell({ task }: { task: Task }) {
  return (
    <td
      style={{
        textAlign: "left",
        opacity: 0.75,
        fontSize: "0.8rem",
      }}
    >
      {compactDescription(task)}
    </td>
  );
}

/** Build a short description from structured task fields, falling back to the
 *  server-provided description when the type isn't one we can shorten. */
export function compactDescription(t: Task): string {
  const fmtShort = (iso?: string) => {
    if (!iso) return "";
    const d = new Date(iso);
    return d.toLocaleString(undefined, { year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
  };
  const fmtTimeOnly = (iso?: string) => {
    if (!iso) return "";
    const d = new Date(iso);
    return d.toLocaleString(undefined, { hour: "2-digit", minute: "2-digit" });
  };
  const sameDay = (a?: string, b?: string) => {
    if (!a || !b) return false;
    const da = new Date(a), db = new Date(b);
    return da.getFullYear() === db.getFullYear() && da.getMonth() === db.getMonth() && da.getDate() === db.getDate();
  };

  const INTERVAL_LABELS: Record<string, string> = {
    "1h": "Hourly",
    "1d": "Daily",
    "4h": "4-Hour",
    "15m": "15-Min",
    "5m": "5-Min",
    "1m": "1-Min",
  };

  switch (t.type) {
    case "historical_backfill": {
      const res = INTERVAL_LABELS[t.interval] || t.interval || "Hourly";
      const end = sameDay(t.window_start, t.window_end) ? fmtTimeOnly(t.window_end) : fmtShort(t.window_end);
      return `${res} candles from ${fmtShort(t.window_start)} to ${end}`;
    }
    case "consolidation":
      return `Rebuild daily candle for ${fmtShort(t.window_start)}`;
    case "integrity_check": {
      const end = sameDay(t.window_start, t.window_end) ? fmtTimeOnly(t.window_end) : fmtShort(t.window_end);
      return `Verify coverage from ${fmtShort(t.window_start)} to ${end}`;
    }
    case "gap_validation":
      return `Check for gaps at ${fmtShort(t.window_start)}`;
    case "task_cleanup":
      return "Prune completed and failed task logs";
    case "news_fetch":
      return "Fetch latest crypto news";
    case "live_ticker":
      return "—";
    default:
      return t.description || "—";
  }
}

export function formatDateTime(isoString?: string) {
  if (!isoString) return "—";
  const d = new Date(isoString);
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export function formatShortDate(isoString?: string) {
  if (!isoString) return "";
  const d = new Date(isoString);
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export function formatUnixTimestamp(unix?: number) {
  if (!unix) return "";
  const d = new Date(unix * 1000);
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function formatUnixDate(unix: number) {
  if (!unix || unix <= 0) return "—";
  const d = new Date(unix * 1000);
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export function taskStatusLabel(status?: string) {
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

interface MultiFilterProps {
  allOptions: string[];
  labels?: Record<string, string>;
  title: string;
  selected: Set<string>;
  onChange: (next: Set<string>) => void;
}

export function MultiFilter({ allOptions, labels, title, selected, onChange }: MultiFilterProps) {
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
          border: "1px solid var(--color-interactive-border)",
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
            background: "var(--bg-secondary)",
            border: "1px solid var(--color-interactive-border)",
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
              style={{ background: "none", border: "none", color: "var(--color-muted)", cursor: "pointer", fontSize: "0.78rem" }}
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
              onMouseEnter={(e) => ((e.currentTarget as HTMLElement).style.background = "var(--color-interactive-bg)")}
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

interface PaginationProps {
  total: number;
  limit: number;
  page: number;
  onPageChange: (next: number) => void;
}

export function Pagination({ total, limit, page, onPageChange }: PaginationProps) {
  const totalPages = Math.ceil(total / limit) || 1;
  const canPrev = page > 1;
  const canNext = page < totalPages;

  return (
    <div style={{ display: "flex", alignItems: "center", gap: "1rem", marginTop: "1rem", fontSize: "0.85rem" }}>
      <button
        disabled={!canPrev}
        onClick={() => onPageChange(page - 1)}
        style={{
          background: "var(--color-interactive-bg)",
          border: "1px solid var(--color-subtle-border)",
          borderRadius: "6px",
          color: canPrev ? "inherit" : "var(--color-disabled)",
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
          background: "var(--color-interactive-bg)",
          border: "1px solid var(--color-subtle-border)",
          borderRadius: "6px",
          color: canNext ? "inherit" : "var(--color-disabled)",
          cursor: canNext ? "pointer" : "default",
          padding: "0.3rem 0.6rem",
        }}
      >
        Next
      </button>
    </div>
  );
}

/** Compact status icon with tooltip — replaces verbose text labels to save table width. */
export function StatusIcon({ status, lastError }: { status?: string; lastError?: string }) {
  const isCompleted = status === "completed";
  const isFailed = status === "failed";

  if (isFailed && lastError) {
    return <FailureStatus reason={lastError} />;
  }

  const icon = isCompleted ? "✓" : isFailed ? "✗" : "●";
  const color = isCompleted
    ? "var(--up-color)"
    : isFailed
    ? "var(--down-color)"
    : "var(--color-muted)";
  const label = taskStatusLabel(status);

  return (
    <span
      title={label}
      aria-label={label}
      style={{
        color,
        fontSize: "1rem",
        fontWeight: 700,
        cursor: "default",
      }}
    >
      {icon}
    </span>
  );
}

export function FailureStatus({ reason }: { reason: string }) {
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
            background: "var(--color-tooltip-bg)",
            boxShadow: "0 12px 28px rgba(0,0,0,0.35)",
            color: "var(--color-tooltip-text)",
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
