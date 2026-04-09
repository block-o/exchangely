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
        className="toggle-group"
        style={{ marginTop: "1rem", marginBottom: "1.25rem" }}
      >
        {TABS.map((tab) => (
          <button
            key={tab}
            role="tab"
            aria-selected={activeTab === tab}
            className={activeTab === tab ? "active" : ""}
            onClick={() => switchTab(tab)}
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
