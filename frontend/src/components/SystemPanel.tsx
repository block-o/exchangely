import { useState } from "react";
import { OverviewTab } from "./system/OverviewTab";
import { CoverageTab } from "./system/CoverageTab";
import { AuditTab } from "./system/AuditTab";

const TABS = ["Overview", "Coverage", "Audit"] as const;
type Tab = (typeof TABS)[number];

function getInitialTab(): Tab {
  const params = new URLSearchParams(window.location.search);
  const raw = params.get("tab");
  if (raw && TABS.includes(raw as Tab)) return raw as Tab;
  return "Overview";
}

export function SystemPanel() {
  const [activeTab, setActiveTab] = useState<Tab>(getInitialTab);

  function switchTab(tab: Tab) {
    setActiveTab(tab);
    const url = new URL(window.location.href);
    url.searchParams.set("tab", tab);
    window.history.replaceState({}, "", url.toString());
  }

  return (
    <section className="panel">
      <div className="panel-header">
        <h2>System Operations</h2>
        <p>Monitor platform health, live feeds, historical coverage, and task activity.</p>
      </div>

      {/* Tab bar */}
      <div
        role="tablist"
        aria-label="Operations tabs"
        style={{
          display: "flex",
          gap: "0.25rem",
          marginTop: "1rem",
          marginBottom: "1.25rem",
          borderBottom: "1px solid rgba(255,255,255,0.08)",
          paddingBottom: "0",
        }}
      >
        {TABS.map((tab) => (
          <button
            key={tab}
            role="tab"
            aria-selected={activeTab === tab}
            onClick={() => switchTab(tab)}
            style={{
              background: "none",
              border: "none",
              borderBottom: activeTab === tab ? "2px solid var(--accent-color, #7c6fff)" : "2px solid transparent",
              color: activeTab === tab ? "var(--accent-color, #7c6fff)" : "inherit",
              cursor: "pointer",
              fontSize: "0.9rem",
              fontWeight: activeTab === tab ? 600 : 400,
              opacity: activeTab === tab ? 1 : 0.6,
              padding: "0.5rem 1rem",
              transition: "all 0.15s ease",
            }}
          >
            {tab}
          </button>
        ))}
      </div>

      {/* Tab content */}
      <div role="tabpanel">
        {activeTab === "Overview" && <OverviewTab />}
        {activeTab === "Coverage" && <CoverageTab />}
        {activeTab === "Audit" && <AuditTab />}
      </div>
    </section>
  );
}
