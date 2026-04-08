import { useEffect, useRef, useState } from "react";

// ── Shared types ────────────────────────────────────────────────────────────

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
];

export const TYPE_LABELS: Record<string, string> = {
  historical_backfill: "Historical Backfill",
  live_ticker: "Live Ticker",
  integrity_check: "Integrity Check",
  consolidation: "Consolidation",
  task_cleanup: "Task Log Cleanup",
};

export const RECENT_TASK_STATUSES = ["completed", "failed"];

export const STATUS_LABELS: Record<string, string> = {
  completed: "Completed",
  failed: "Failed",
};

// ── Formatting helpers ──────────────────────────────────────────────────────

export function formatDateTime(isoString?: string) {
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

export function formatShortDate(isoString?: string) {
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

export function formatUnixTimestamp(unix?: number) {
  if (!unix) return "";
  const d = new Date(unix * 1000);
  return d.toLocaleString(undefined, {
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

// ── Reusable UI components ──────────────────────────────────────────────────

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
