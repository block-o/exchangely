import './PortfolioPage.css';
import { useCallback, useEffect, useRef, useState } from "react";
import { useAuth } from "../app/auth";
import { useSettings } from "../app/settings";
import { getValuation, getHoldings, syncAll, triggerRecompute, subscribePortfolioStream } from "../api/portfolio";
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
import { TransactionsTab } from "../components/portfolio/TransactionsTab";
import { PnLTab } from "../components/portfolio/PnLTab";
import { Alert, ToggleGroup } from "../components/ui";

type ActivePanel = "holdings" | "pnl" | "transactions" | "sources";

function buildPanelOptions(resyncing: boolean) {
  return [
    { value: "holdings", label: "Holdings" },
    { value: "pnl", label: resyncing ? "P&L (recalculating…)" : "P&L" },
    { value: "transactions", label: resyncing ? "Transactions (recalculating…)" : "Transactions" },
    { value: "sources", label: "Sources" },
  ];
}

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
  const [activePanel, setActivePanel] = useState<ActivePanel>("holdings");
  const [refreshCounter, setRefreshCounter] = useState(0);
  const [txRefreshKey, setTxRefreshKey] = useState(0);
  const [pnlRefreshKey, setPnlRefreshKey] = useState(0);
  const [resyncing, setResyncing] = useState(false);
  const prevCurrencyRef = useRef(quoteCurrency);

  // Detect currency changes and trigger recompute
  useEffect(() => {
    if (prevCurrencyRef.current === quoteCurrency) return;
    prevCurrencyRef.current = quoteCurrency;
    if (!isAuthenticated) return;

    setResyncing(true);
    triggerRecompute(quoteCurrency).catch(() => {
      // recompute endpoint may fail; clear resyncing on error
      setResyncing(false);
    });

    // Subscribe to SSE for completion, with a timeout fallback
    const es = subscribePortfolioStream();
    const timeout = setTimeout(() => {
      setResyncing(false);
      es.close();
    }, 10_000);

    const onMessage = () => {
      clearTimeout(timeout);
      setResyncing(false);
      es.close();
    };

    es.addEventListener("message", onMessage);
    es.addEventListener("portfolio", onMessage);
    es.addEventListener("transactions_updated", onMessage);
    es.addEventListener("pnl_updated", onMessage);

    return () => {
      clearTimeout(timeout);
      es.removeEventListener("message", onMessage);
      es.removeEventListener("portfolio", onMessage);
      es.removeEventListener("transactions_updated", onMessage);
      es.removeEventListener("pnl_updated", onMessage);
      es.close();
    };
  }, [quoteCurrency, isAuthenticated]);

  const panelOptions = buildPanelOptions(resyncing);

  // Persistent SSE subscription for transaction and P&L update events.
  useEffect(() => {
    if (!isAuthenticated) return;

    const es = subscribePortfolioStream();

    const onTxUpdated = () => setTxRefreshKey((k) => k + 1);
    const onPnlUpdated = () => setPnlRefreshKey((k) => k + 1);

    es.addEventListener("transactions_updated", onTxUpdated);
    es.addEventListener("pnl_updated", onPnlUpdated);

    return () => {
      es.removeEventListener("transactions_updated", onTxUpdated);
      es.removeEventListener("pnl_updated", onPnlUpdated);
      es.close();
    };
  }, [isAuthenticated]);

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

        {resyncing && (
          <Alert level="warning">
            Transaction values are being recalculated in the new currency. Previously computed values are shown until the update completes.
          </Alert>
        )}

        <div className="portfolio-top-bar">
          <ToggleGroup
            options={panelOptions}
            value={activePanel}
            onChange={(v) => setActivePanel(v as ActivePanel)}
          />
        </div>

        {activePanel === "holdings" && isEmpty && !error && (
          <EmptyState
            onAddHolding={() => setActivePanel("sources")}
            onManageExchanges={() => setShowAddExchange(true)}
            onManageWallets={() => setShowAddWallet(true)}
            onImportLedger={() => setShowLedgerUpload(true)}
          />
        )}

        {activePanel === "holdings" && hasHoldings && (
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

        {activePanel === "pnl" && (
          <PnLTab quoteCurrency={quoteCurrency} refreshKey={pnlRefreshKey} />
        )}

        {activePanel === "transactions" && (
          <TransactionsTab quoteCurrency={quoteCurrency} refreshKey={txRefreshKey} />
        )}

        {activePanel === "sources" && (
          <div className="portfolio-sources-panel">
            <div className="settings-panel portfolio-panel-grow">
              <ExchangeCredentialManager onSynced={handleRefresh} />
            </div>
            <div className="settings-panel portfolio-panel-grow">
              <WalletManager onSynced={handleRefresh} />
            </div>
            <div className="settings-panel portfolio-panel-grow">
              <LedgerManager onSynced={handleRefresh} />
            </div>
            <div className="settings-panel portfolio-panel-grow">
              <ManualHoldingPanel quoteCurrency={quoteCurrency} onCreated={handleRefresh} />
            </div>
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
