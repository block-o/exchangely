import { useState } from "react";
import { OverviewTab } from "./system/OverviewTab";
import { CoverageTab } from "./system/CoverageTab";
import { AuditTab } from "./system/AuditTab";
import { UsersTab } from "./system/UsersTab";
import { ToggleGroup } from "./ui";

const TABS = ["Overview", "Coverage", "Users", "Audit"] as const;
type Tab = (typeof TABS)[number];

const TAB_OPTIONS = TABS.map((t) => ({ value: t, label: t }));

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
    <section className="panel" style={{ display: "flex", flexDirection: "column", minHeight: "calc(100vh - 120px)" }}>
      <div className="panel-header">
        <h2>System Operations</h2>
        <p>Monitor platform health, live feeds, historical coverage, and task activity.</p>
      </div>

      {/* Tab bar */}
      <ToggleGroup
        options={TAB_OPTIONS}
        value={activeTab}
        onChange={(v) => switchTab(v as Tab)}
        aria-label="Operations tabs"
        style={{ marginTop: "1rem", marginBottom: "1.25rem" }}
      />

      {/* Tab content */}
      <div role="tabpanel" style={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0 }}>
        {activeTab === "Overview" && <OverviewTab />}
        {activeTab === "Coverage" && <CoverageTab />}
        {activeTab === "Audit" && <AuditTab />}
        {activeTab === "Users" && <UsersTab />}
      </div>
    </section>
  );
}
