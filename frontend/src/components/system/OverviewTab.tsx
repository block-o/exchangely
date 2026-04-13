import { useEffect, useState } from "react";
import { dismissWarning as dismissSystemWarning, fetchWarnings as fetchSystemWarnings } from "../../api/system";
import { API_BASE_URL } from "../../api/client";
import type { ActiveWarning } from "../../types/api";
import { formatUnixTimestamp } from "./shared";
import { Alert } from "../../components/ui";

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
              <Alert
                key={warning.id}
                level={warning.level === "error" ? "error" : "warning"}
                title={warning.title}
                onDismiss={
                  dismissingWarningIds.has(warning.id)
                    ? undefined
                    : () => dismissWarning(warning)
                }
                className="overview-alert"
              >
                <div className="overview-alert__meta">
                  {warning.timestamp ? (
                    <span className="overview-alert__timestamp">
                      {formatUnixTimestamp(warning.timestamp)}
                    </span>
                  ) : null}
                  <span className="overview-alert__level">{warning.level}</span>
                </div>
                <p style={{ margin: 0 }}>{warning.detail}</p>
              </Alert>
            ))}
          </div>
        )}
      </div>
    </>
  );
}
