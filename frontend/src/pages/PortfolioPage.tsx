import './PortfolioPage.css';
import { useCallback, useEffect, useState } from "react";
import { useAuth } from "../app/auth";
import { useSettings } from "../app/settings";
import { getValuation, getHoldings, syncAll } from "../api/portfolio";
import type { Valuation, Holding } from "../api/portfolio";
import { ValuationHeader } from "../components/portfolio/ValuationHeader";
import { AllocationChart } from "../components/portfolio/AllocationChart";
import { HoldingsTable } from "../components/portfolio/HoldingsTable";
import { HistoryChart } from "../components/portfolio/HistoryChart";
import { EmptyState } from "../components/portfolio/EmptyState";
import { ManualHoldingPanel } from "../components/portfolio/ManualHoldingPanel";
import { ExchangeCredentialManager } from "../components/portfolio/ExchangeCredentialManager";
import { WalletManager } from "../components/portfolio/WalletManager";
import { LedgerManager } from "../components/portfolio/LedgerManager";
import { Alert, ToggleGroup } from "../components/ui";

type ActivePanel = "overview" | "exchanges" | "wallets" | "ledger" | "add";

const PANEL_OPTIONS = [
  { value: "overview", label: "Overview" },
  { value: "exchanges", label: "Exchanges" },
  { value: "wallets", label: "Wallets" },
  { value: "ledger", label: "Ledger" },
  { value: "add", label: "Others" },
];

export function PortfolioPage() {
  const { user, isAuthenticated, isLoading } = useAuth();
  const { quoteCurrency } = useSettings();

  const [valuation, setValuation] = useState<Valuation | null>(null);
  const [holdings, setHoldings] = useState<Holding[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showAddExchange, setShowAddExchange] = useState(false);
  const [showAddWallet, setShowAddWallet] = useState(false);
  const [showLedgerUpload, setShowLedgerUpload] = useState(false);
  const [activePanel, setActivePanel] = useState<ActivePanel>("overview");
  const [refreshCounter, setRefreshCounter] = useState(0);

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      window.location.hash = "#login";
    }
  }, [isLoading, isAuthenticated]);

  const fetchValuation = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [data, h] = await Promise.all([
        getValuation(quoteCurrency),
        getHoldings(),
      ]);
      setValuation(data);
      setHoldings(h);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load portfolio");
    } finally {
      setLoading(false);
    }
  }, [quoteCurrency]);

  useEffect(() => {
    if (isAuthenticated) fetchValuation();
  }, [isAuthenticated, fetchValuation]);

  const handleRefresh = useCallback(() => {
    fetchValuation();
    setRefreshCounter((c) => c + 1);
  }, [fetchValuation]);

  const handleSyncAll = useCallback(async () => {
    await syncAll();
    await fetchValuation();
  }, [fetchValuation]);

  if (isLoading) {
    return (
      <section className="settings-page">
        <div className="settings-loading">Loading…</div>
      </section>
    );
  }

  if (!user) return null;

  const hasHoldings = valuation && valuation.assets && valuation.assets.length > 0;
  const isEmpty = !loading && !hasHoldings;

  return (
    <section className="portfolio-page">
      <div className="portfolio-container">
        {error && <Alert level="error">{error}</Alert>}

        <div className="portfolio-top-bar">
          <ToggleGroup
            options={PANEL_OPTIONS}
            value={activePanel}
            onChange={(v) => setActivePanel(v as ActivePanel)}
          />
        </div>

        {activePanel === "overview" && isEmpty && !error && (
          <EmptyState
            onAddHolding={() => setActivePanel("add")}
            onManageExchanges={() => setShowAddExchange(true)}
            onManageWallets={() => setShowAddWallet(true)}
            onImportLedger={() => setShowLedgerUpload(true)}
          />
        )}

        {activePanel === "overview" && hasHoldings && (
          <>
            <div className="settings-panel">
              <ValuationHeader valuation={valuation} loading={loading} onSyncAll={handleSyncAll} />
            </div>

            <div className="portfolio-grid">
              <div className="settings-panel">
                <AllocationChart assets={valuation!.assets} />
              </div>
              <div className="settings-panel">
                <HistoryChart quoteCurrency={quoteCurrency} refreshKey={refreshCounter} />
              </div>
            </div>

            <div className="settings-panel">
              <HoldingsTable assets={valuation!.assets} holdings={holdings} quoteCurrency={quoteCurrency} onDeleted={handleRefresh} />
            </div>
          </>
        )}

        {activePanel === "add" && (
          <div className="settings-panel portfolio-panel-grow">
            <ManualHoldingPanel quoteCurrency={quoteCurrency} onCreated={handleRefresh} />
          </div>
        )}

        {activePanel === "exchanges" && (
          <div className="settings-panel portfolio-panel-grow">
            <ExchangeCredentialManager onSynced={handleRefresh} />
          </div>
        )}

        {activePanel === "wallets" && (
          <div className="settings-panel portfolio-panel-grow">
            <WalletManager onSynced={handleRefresh} />
          </div>
        )}

        {activePanel === "ledger" && (
          <div className="settings-panel portfolio-panel-grow">
            <LedgerManager onSynced={handleRefresh} />
          </div>
        )}
      </div>

      {showAddExchange && (
        <ExchangeCredentialManager
          onSynced={() => { setShowAddExchange(false); fetchValuation(); }}
          initialShowAdd
          onModalClose={() => setShowAddExchange(false)}
        />
      )}

      {showAddWallet && (
        <WalletManager
          onSynced={() => { setShowAddWallet(false); fetchValuation(); }}
          initialShowAdd
          onModalClose={() => setShowAddWallet(false)}
        />
      )}

      {showLedgerUpload && (
        <LedgerManager
          onSynced={() => { setShowLedgerUpload(false); fetchValuation(); }}
          initialShowUpload
          onModalClose={() => setShowLedgerUpload(false)}
        />
      )}
    </section>
  );
}
