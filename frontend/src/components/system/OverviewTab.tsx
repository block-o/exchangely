import { useEffect, useState } from "react";
import { dismissWarning as dismissSystemWarning, fetchWarnings as fetchSystemWarnings } from "../../api/system";
import { API_BASE_URL } from "../../api/client";
import type { ActiveWarning } from "../../types/api";
import { formatUnixTimestamp } from "./shared";

export function OverviewTab() {
  const [warnings, setWarnings] = useState<ActiveWarning[]>([]);
  const [warningsLoading, setWarningsLoading] = useState(true);
  const [dismissingWarningIds, setDismissingWarningIds] = useState<Set<string>>(new Set());
  const [apiVersion, setApiVersion] = useState<string>("Unknown");

  // @ts-ignore - injected by vite build
  const uiVersion = typeof __APP_VERSION__ !== "undefined" ? __APP_VERSION__ : "Unknown";

  const loadWarnings = async () => {
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

  useEffect(() => {
    fetch(API_BASE_URL + "/config")
      .then((res) => res.json())
      .then((res) => setApiVersion(res.version))
      .catch(console.error);
    loadWarnings();
  }, []);

  useEffect(() => {
    const interval = window.setInterval(() => loadWarnings(), 15000);
    return () => window.clearInterval(interval);
  }, []);

  async function dismissWarning(warning: ActiveWarning) {
    setDismissingWarningIds((prev) => new Set(prev).add(warning.id));
    try {
      await dismissSystemWarning(warning.id, warning.fingerprint);
      setWarnings((prev) => prev.filter((item) => item.id !== warning.id));
    } catch (e) {
      console.error("Failed to dismiss warning", e);
    } finally {
      setDismissingWarningIds((prev) => {
        const next = new Set(prev);
        next.delete(warning.id);
        return next;
      });
    }
  }

  return (
    <>
      {/* Version info */}
      <div
        style={{
          display: "flex",
          gap: "1.5rem",
          marginBottom: "1rem",
          fontSize: "0.85rem",
          opacity: 0.8,
        }}
      >
        <span>UI: v{uiVersion}</span>
        <span>API: {apiVersion}</span>
      </div>

      {/* Active Warnings */}
      <div
        style={{
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
          <div style={{ display: "flex", flexDirection: "column", gap: "0.6rem" }}>
            {warnings.map((warning) => (
              <article
                key={warning.id}
                style={{
                  padding: "0.9rem 1rem",
                  borderRadius: "10px",
                  border: `1px solid ${
                    warning.level === "error" ? "rgba(255,107,107,0.45)" : "rgba(255,196,61,0.35)"
                  }`,
                  background:
                    warning.level === "error" ? "rgba(120,28,28,0.18)" : "rgba(140,96,0,0.14)",
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
                  <div style={{ display: "flex", alignItems: "center", gap: "0.6rem" }}>
                    <strong style={{ fontSize: "0.92rem" }}>{warning.title}</strong>
                    {warning.timestamp ? (
                      <span style={{ fontSize: "0.75rem", opacity: 0.55 }}>
                        {formatUnixTimestamp(warning.timestamp)}
                      </span>
                    ) : null}
                  </div>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: "0.5rem" }}>
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
                        border: "1px solid var(--color-interactive-border)",
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
    </>
  );
}
