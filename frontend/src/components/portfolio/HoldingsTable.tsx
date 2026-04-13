import { useState } from "react";
import { deleteHolding } from "../../api/portfolio";
import type { AssetValuation, Holding } from "../../api/portfolio";
import { Badge, Button } from "../ui";

type HoldingsTableProps = {
  assets: AssetValuation[];
  holdings: Holding[];
  quoteCurrency: string;
  onDeleted: () => void;
};

function fmt(value: number, currency: string): string {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value);
}

function fmtQty(value: number): string {
  if (value >= 1) return value.toLocaleString(undefined, { maximumFractionDigits: 4 });
  return value.toLocaleString(undefined, { maximumFractionDigits: 8 });
}

function fmtPct(value: number): string {
  const sign = value >= 0 ? "+" : "";
  return `${sign}${value.toFixed(2)}%`;
}

function sourceLabel(source: string): string {
  const labels: Record<string, string> = {
    manual: "Manual",
    binance: "Binance",
    kraken: "Kraken",
    coinbase: "Coinbase",
    ethereum: "Ethereum",
    solana: "Solana",
    bitcoin: "Bitcoin",
    ledger: "Ledger",
  };
  return labels[source] ?? source;
}

export function HoldingsTable({ assets, holdings, quoteCurrency, onDeleted }: HoldingsTableProps) {
  const sorted = [...assets].sort((a, b) => b.current_value - a.current_value);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  if (sorted.length === 0) {
    return null;
  }

  // Build a lookup from (asset_symbol, source) to holding ID for deletion.
  const holdingIdMap = new Map<string, string>();
  for (const h of holdings) {
    holdingIdMap.set(`${h.asset_symbol}:${h.source}`, h.id);
  }

  const handleDelete = async (holdingId: string) => {
    setDeletingId(holdingId);
    try {
      await deleteHolding(holdingId);
      onDeleted();
    } catch {
      // Silently fail — the user can retry
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <div className="portfolio-holdings">
      <h3 className="settings-panel-title">Holdings</h3>
      <div className="data-table-wrapper">
        <table className="data-table">
          <thead>
            <tr>
              <th>Asset</th>
              <th>Quantity</th>
              <th className="portfolio-col-tablet">Price</th>
              <th>Value</th>
              <th className="portfolio-col-desktop">Avg Buy</th>
              <th>P&amp;L</th>
              <th className="portfolio-col-desktop">Source</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((a) => {
              const pnlClass = a.unrealized_pnl != null
                ? a.unrealized_pnl >= 0 ? "text-up" : "text-down"
                : "text-muted";
              return (
                <tr key={`${a.asset_symbol}-${a.source}`} className="hoverable-row">
                  <td style={{ textAlign: "left" }}>
                    <span className="asset-name">{a.asset_symbol}</span>
                    {!a.priced && <Badge variant="warning" className="portfolio-unpriced-badge">unpriced</Badge>}
                  </td>
                  <td>{fmtQty(a.quantity)}</td>
                  <td className="portfolio-col-tablet">{a.priced ? fmt(a.current_price, quoteCurrency) : "—"}</td>
                  <td>{a.priced ? fmt(a.current_value, quoteCurrency) : "—"}</td>
                  <td className="portfolio-col-desktop">
                    {a.avg_buy_price != null ? fmt(a.avg_buy_price, quoteCurrency) : "—"}
                  </td>
                  <td>
                    {a.unrealized_pnl != null ? (
                      <div className="portfolio-pnl-cell">
                        <span className={`portfolio-pnl-amount ${pnlClass}`}>{fmt(a.unrealized_pnl, quoteCurrency)}</span>
                        {a.unrealized_pnl_pct != null && (
                          <span className={`portfolio-pnl-pct ${pnlClass}`}>
                            {fmtPct(a.unrealized_pnl_pct)}
                          </span>
                        )}
                      </div>
                    ) : (
                      <span className="text-muted">—</span>
                    )}
                  </td>
                  <td className="portfolio-col-desktop">
                    <Badge variant="default" className="portfolio-source-badge">{sourceLabel(a.source)}</Badge>
                  </td>
                  <td>
                    {(() => {
                      const hId = holdingIdMap.get(`${a.asset_symbol}:${a.source}`);
                      if (!hId) return null;
                      return (
                        <Button
                          variant="danger"
                          className="portfolio-delete-btn"
                          onClick={() => handleDelete(hId)}
                          disabled={deletingId === hId}
                          aria-label={`Remove ${a.asset_symbol} holding`}
                        >
                          {deletingId === hId ? "…" : "✕"}
                        </Button>
                      );
                    })()}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
