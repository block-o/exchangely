import "./PnLTab.css";
import { useCallback, useEffect, useRef, useState } from "react";
import { getPnL, getTransactions } from "../../api/portfolio";
import type { PnLSnapshot, AssetPnL } from "../../api/portfolio";
import {
  Table,
  TableHead,
  TableBody,
  TableRow,
  TableCell,
  Badge,
  Alert,
  Spinner,
} from "../ui";

interface PnLTabProps {
  quoteCurrency: string;
  refreshKey?: number;
}

function fmtCurrency(value: number, currency: string): string {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value);
}

function pnlColorClass(value: number): string {
  if (value > 0) return "pnl-positive";
  if (value < 0) return "pnl-negative";
  return "pnl-neutral";
}

export function PnLTab({ quoteCurrency, refreshKey }: PnLTabProps) {
  const [snapshot, setSnapshot] = useState<PnLSnapshot | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [txCount, setTxCount] = useState<number | null>(null);
  const txCountFetchedRef = useRef(false);

  const fetchPnL = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await getPnL(quoteCurrency);
      setSnapshot(data);

      // Only fetch the transaction count once to avoid repeated API calls
      // when P&L hasn't been computed yet. SSE events trigger refreshes
      // periodically, and we don't want to hammer the transactions endpoint.
      if ((!data || !data.computed_at) && !txCountFetchedRef.current) {
        txCountFetchedRef.current = true;
        try {
          const txRes = await getTransactions({ page: 1, page_size: 1 });
          setTxCount(txRes.total);
        } catch {
          setTxCount(null);
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load P&L data");
    } finally {
      setLoading(false);
    }
  }, [quoteCurrency]);

  // Reset the tx count cache when currency changes.
  useEffect(() => {
    txCountFetchedRef.current = false;
    setTxCount(null);
  }, [quoteCurrency]);

  useEffect(() => {
    fetchPnL();
  }, [fetchPnL, refreshKey]);

  if (loading) {
    return (
      <div
        className="settings-panel portfolio-panel-grow"
        style={{ alignItems: "center", justifyContent: "center" }}
      >
        <Spinner label="Loading P&L…" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="settings-panel portfolio-panel-grow">
        <p style={{ color: "var(--color-down)" }}>{error}</p>
      </div>
    );
  }

  if (!snapshot || !snapshot.computed_at) {
    const hasTx = txCount != null && txCount > 0;
    return (
      <div className="settings-panel portfolio-panel-grow">
        <div style={{ textAlign: "center", padding: "32px 16px" }}>
          <h3 className="settings-panel-title">Profit &amp; Loss</h3>
          {hasTx ? (
            <Alert level="info" title="Pending calculation">
              You have {txCount} transaction{txCount !== 1 ? "s" : ""} but P&amp;L hasn't been calculated yet.
              It will be computed automatically on the next scheduled refresh cycle.
            </Alert>
          ) : (
            <>
              <p className="text-muted" style={{ marginBottom: 8 }}>
                No transactions available to compute P&amp;L from.
              </p>
              <p className="text-muted" style={{ fontSize: "0.85rem" }}>
                Sync an exchange with trade history support (like Kraken), or import transactions via a Ledger Live export to see profit and loss here.
              </p>
            </>
          )}
        </div>
      </div>
    );
  }

  const assets: AssetPnL[] = snapshot.assets ?? [];

  return (
    <div className="settings-panel portfolio-panel-grow">
      <div
        className="portfolio-manager-header"
        style={{ marginBottom: 12 }}
      >
        <h3 className="settings-panel-title" style={{ marginBottom: 0 }}>
          Profit &amp; Loss
        </h3>
        <span className="text-muted" style={{ fontSize: "0.82rem" }}>
          {quoteCurrency}
        </span>
      </div>

      {/* Summary cards */}
      <div className="pnl-summary">
        <div className="pnl-summary-card">
          <span className="pnl-summary-label">Realized P&amp;L</span>
          <span
            className={`pnl-summary-value ${pnlColorClass(snapshot.total_realized)}`}
          >
            {fmtCurrency(snapshot.total_realized, quoteCurrency)}
          </span>
        </div>
        <div className="pnl-summary-card">
          <span className="pnl-summary-label">Unrealized P&amp;L</span>
          <span
            className={`pnl-summary-value ${pnlColorClass(snapshot.total_unrealized)}`}
          >
            {fmtCurrency(snapshot.total_unrealized, quoteCurrency)}
          </span>
        </div>
        <div className="pnl-summary-card">
          <span className="pnl-summary-label">Total P&amp;L</span>
          <span
            className={`pnl-summary-value ${pnlColorClass(snapshot.total_pnl)}`}
          >
            {fmtCurrency(snapshot.total_pnl, quoteCurrency)}
          </span>
        </div>
      </div>

      {/* Notices */}
      <div className="pnl-notices">
        {snapshot.has_approximate && (
          <Alert level="info" title="Approximate Values">
            Some P&amp;L values are based on hourly or daily candle prices
            rather than exact trade prices. These are marked as approximate.
          </Alert>
        )}
        {snapshot.excluded_count > 0 && (
          <Alert level="warning" title="Excluded Transactions">
            {snapshot.excluded_count} transaction
            {snapshot.excluded_count !== 1 ? "s were" : " was"} excluded from
            P&amp;L computation because no price data could be resolved for{" "}
            {snapshot.excluded_count !== 1 ? "them" : "it"}.
          </Alert>
        )}
      </div>

      {/* Per-asset breakdown */}
      {assets.length > 0 ? (
        <Table>
          <TableHead>
            <tr>
              <th style={{ textAlign: "left" }}>Asset</th>
              <th>Realized</th>
              <th>Unrealized</th>
              <th>Total</th>
              <th>Transactions</th>
            </tr>
          </TableHead>
          <TableBody>
            {assets.map((a) => (
              <TableRow key={a.asset_symbol}>
                <TableCell align="left">
                  <span className="asset-name">{a.asset_symbol}</span>
                </TableCell>
                <TableCell>
                  <span className={pnlColorClass(a.realized_pnl)}>
                    {fmtCurrency(a.realized_pnl, quoteCurrency)}
                  </span>
                </TableCell>
                <TableCell>
                  <span className={pnlColorClass(a.unrealized_pnl)}>
                    {fmtCurrency(a.unrealized_pnl, quoteCurrency)}
                  </span>
                </TableCell>
                <TableCell>
                  <span className={pnlColorClass(a.total_pnl)}>
                    {fmtCurrency(a.total_pnl, quoteCurrency)}
                  </span>
                </TableCell>
                <TableCell>
                  <Badge variant="default">{a.transaction_count}</Badge>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : (
        <p className="text-muted" style={{ textAlign: "center", padding: "24px 0" }}>
          No per-asset P&amp;L data available. P&amp;L will update on the next scheduled refresh.
        </p>
      )}
    </div>
  );
}
