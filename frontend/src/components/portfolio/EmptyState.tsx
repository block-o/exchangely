import { useState } from "react";

type EmptyStateProps = {
  onAddHolding: () => void;
  onManageExchanges: () => void;
  onManageWallets: () => void;
  onImportLedger: () => void;
};

export function EmptyState({ onAddHolding, onManageExchanges, onManageWallets, onImportLedger }: EmptyStateProps) {
  const [hoveredCard, setHoveredCard] = useState<string | null>(null);

  return (
    <div className="portfolio-empty">
      <div className="portfolio-empty-icon" aria-hidden="true">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <path d="M21 12V7H5a2 2 0 0 1 0-4h14v4" />
          <path d="M3 5v14a2 2 0 0 0 2 2h16v-5" />
          <path d="M18 12a2 2 0 0 0 0 4h4v-4h-4z" />
        </svg>
      </div>
      <h3 className="portfolio-empty-title">Your portfolio is empty</h3>
      <p className="portfolio-empty-desc">
        Start tracking your crypto investments by adding holdings manually,
        connecting an exchange, linking a wallet, or importing from Ledger Live.
      </p>
      <div className="portfolio-empty-actions">
        <button
          className="portfolio-empty-card"
          onClick={onManageExchanges}
          onMouseEnter={() => setHoveredCard("exchange")}
          onMouseLeave={() => setHoveredCard(null)}
          data-hovered={hoveredCard === "exchange" || undefined}
        >
          <span className="portfolio-empty-card-icon" aria-hidden="true">⇄</span>
          <span className="portfolio-empty-card-label">Connect Exchange</span>
          <span className="portfolio-empty-card-hint">Sync from Binance, Kraken, Coinbase</span>
        </button>
        <button
          className="portfolio-empty-card"
          onClick={onImportLedger}
          onMouseEnter={() => setHoveredCard("ledger")}
          onMouseLeave={() => setHoveredCard(null)}
          data-hovered={hoveredCard === "ledger" || undefined}
        >
          <span className="portfolio-empty-card-icon" aria-hidden="true">⬡</span>
          <span className="portfolio-empty-card-label">Import Ledger</span>
          <span className="portfolio-empty-card-hint">Upload Ledger Live export</span>
        </button>
        <button
          className="portfolio-empty-card"
          onClick={onManageWallets}
          onMouseEnter={() => setHoveredCard("wallet")}
          onMouseLeave={() => setHoveredCard(null)}
          data-hovered={hoveredCard === "wallet" || undefined}
        >
          <span className="portfolio-empty-card-icon" aria-hidden="true">🔗</span>
          <span className="portfolio-empty-card-label">Link Wallet</span>
          <span className="portfolio-empty-card-hint">Track ETH, SOL, BTC on-chain</span>
        </button>
        <button
          className="portfolio-empty-card"
          onClick={onAddHolding}
          onMouseEnter={() => setHoveredCard("manual")}
          onMouseLeave={() => setHoveredCard(null)}
          data-hovered={hoveredCard === "manual" || undefined}
        >
          <span className="portfolio-empty-card-icon" aria-hidden="true">+</span>
          <span className="portfolio-empty-card-label">Add Holding</span>
          <span className="portfolio-empty-card-hint">Enter positions manually</span>
        </button>
      </div>
    </div>
  );
}
