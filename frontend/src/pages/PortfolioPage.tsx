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
import { AddHoldingModal } from "../components/portfolio/AddHoldingModal";
import { ExchangeCredentialManager } from "../components/portfolio/ExchangeCredentialManager";
import { WalletManager } from "../components/portfolio/WalletManager";
import { LedgerManager } from "../components/portfolio/LedgerManager";

type ActivePanel = "overview" | "exchanges" | "wallets" | "ledger";

export function PortfolioPage() {
  const { user, isAuthenticated, isLoading } = useAuth();
  const { quoteCurrency } = useSettings();

  const [valuation, setValuation] = useState<Valuation | null>(null);
  const [holdings, setHoldings] = useState<Holding[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showAddHolding, setShowAddHolding] = useState(false);
  const [showAddExchange, setShowAddExchange] = useState(false);
  const [showAddWallet, setShowAddWallet] = useState(false);
  const [showLedgerUpload, setShowLedgerUpload] = useState(false);
  const [activePanel, setActivePanel] = useState<ActivePanel>("overview");

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
        {error && <div className="login-error" role="alert">{error}</div>}

        {isEmpty && !error ? (
          <EmptyState
            onAddHolding={() => setShowAddHolding(true)}
            onManageExchanges={() => setShowAddExchange(true)}
            onManageWallets={() => setShowAddWallet(true)}
            onImportLedger={() => setShowLedgerUpload(true)}
          />
        ) : (
          <>
            <div className="portfolio-top-bar">
              <div className="toggle-group">
                <button className={activePanel === "overview" ? "active" : ""} onClick={() => setActivePanel("overview")}>
                  Overview
                </button>
                <button className={activePanel === "exchanges" ? "active" : ""} onClick={() => setActivePanel("exchanges")}>
                  Exchanges
                </button>
                <button className={activePanel === "wallets" ? "active" : ""} onClick={() => setActivePanel("wallets")}>
                  Wallets
                </button>
                <button className={activePanel === "ledger" ? "active" : ""} onClick={() => setActivePanel("ledger")}>
                  Ledger
                </button>
              </div>
              <button className="apikeys-create-btn" onClick={() => setShowAddHolding(true)} style={{ padding: "8px 16px", fontSize: "0.82rem" }}>
                Add Holding
              </button>
            </div>
          </>
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
                <HistoryChart quoteCurrency={quoteCurrency} />
              </div>
            </div>

            <div className="settings-panel">
              <HoldingsTable assets={valuation!.assets} holdings={holdings} quoteCurrency={quoteCurrency} onDeleted={handleRefresh} />
            </div>
          </>
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

      {showAddHolding && (
        <AddHoldingModal
          quoteCurrency={quoteCurrency}
          onClose={() => setShowAddHolding(false)}
          onCreated={() => {
            setShowAddHolding(false);
            fetchValuation();
          }}
        />
      )}

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
