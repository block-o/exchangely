import "./TransactionsTab.css";
import { useCallback, useEffect, useState } from "react";
import {
  getTransactions,
  updateTransaction,
  getCredentials,
  getWallets,
} from "../../api/portfolio";
import type { Transaction, ExchangeCredential, WalletAddress } from "../../api/portfolio";
import {
  Table,
  TableHead,
  TableBody,
  TableRow,
  TableCell,
  Badge,
  Spinner,
  Modal,
  Button,
  Input,
  Alert,
} from "../ui";

interface TransactionsTabProps {
  quoteCurrency: string;
  refreshKey?: number;
}

const PAGE_SIZE = 20;

// ISO 4217 codes that Intl.NumberFormat accepts. Crypto tickers like USDT/USDC
// are not valid ISO currency codes and will throw RangeError.
const KNOWN_FIAT = new Set(["USD", "EUR", "GBP", "JPY", "CHF", "CAD", "AUD", "NZD", "CNY", "KRW", "SGD", "HKD", "NOK", "SEK", "DKK", "PLN", "CZK", "HUF", "TRY", "BRL", "MXN", "ZAR", "INR", "THB", "TWD", "ILS", "ARS", "CLP", "COP", "PEN", "PHP", "IDR", "MYR", "VND", "RUB", "UAH", "RON", "BGN", "HRK", "ISK"]);

function safeFmtCurrency(value: number, currency: string, fractionDigits = 2): string {
  if (KNOWN_FIAT.has(currency)) {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
      minimumFractionDigits: fractionDigits,
      maximumFractionDigits: fractionDigits,
    }).format(value);
  }
  // Non-ISO currency: format as plain number with symbol suffix
  const num = value.toLocaleString(undefined, {
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: Math.max(fractionDigits, 6),
  });
  return `${num} ${currency}`;
}

function fmtValue(value: number | null, currency: string): string {
  if (value == null) return "—";
  return safeFmtCurrency(value, currency);
}

function fmtUnitPrice(value: number | null, qty: number, currency: string): string {
  if (value == null || qty === 0) return "";
  const unitPrice = value / qty;
  return safeFmtCurrency(unitPrice, currency);
}

function fmtQty(value: number): string {
  if (value >= 1) return value.toLocaleString(undefined, { maximumFractionDigits: 4 });
  return value.toLocaleString(undefined, { maximumFractionDigits: 8 });
}

function fmtTimestamp(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function fmtFee(fee: number | null, currency: string): string {
  if (fee == null || fee === 0) return "—";
  return safeFmtCurrency(fee, currency || "USD", 2);
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

function typeLabel(type: string): string {
  const labels: Record<string, string> = {
    buy: "Buy",
    sell: "Sell",
    transfer: "Transfer",
    fee: "Fee",
  };
  return labels[type] ?? type;
}

function ResolutionIndicator({ resolution, asset, quoteCurrency }: { resolution: string; asset?: string; quoteCurrency?: string }) {
  if (resolution === "hourly") {
    return <Badge variant="default" title="Price resolved from the nearest hourly candle">~1h</Badge>;
  }
  if (resolution === "daily") {
    return <Badge variant="default" title="Price resolved from the nearest daily candle (less precise)">~1d</Badge>;
  }
  if (resolution === "unresolvable") {
    const hint = asset && quoteCurrency
      ? `No ${asset}/${quoteCurrency} price data found for this transaction's date. The pair may not have been tracked yet, or candle data hasn't been backfilled that far.`
      : "No price data found for this transaction's date.";
    return (
      <span title={hint} className="tx-warning-icon" aria-label={hint}>
        ⚠️
      </span>
    );
  }
  return null;
}

function TransactionsEmptyState() {
  const [credentials, setCredentials] = useState<ExchangeCredential[]>([]);
  const [wallets, setWallets] = useState<WalletAddress[]>([]);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    Promise.all([
      getCredentials().catch(() => [] as ExchangeCredential[]),
      getWallets().catch(() => [] as WalletAddress[]),
    ]).then(([creds, ws]) => {
      setCredentials(creds);
      setWallets(ws);
      setLoaded(true);
    });
  }, []);

  const failedCreds = credentials.filter((c) => c.status === "failed");
  const activeCreds = credentials.filter((c) => c.status === "active");
  const hasConnections = credentials.length > 0 || wallets.length > 0;

  return (
    <div className="settings-panel portfolio-panel-grow">
      <div style={{ textAlign: "center", padding: "32px 16px" }}>

        {loaded && failedCreds.length > 0 && (
          <div style={{ maxWidth: 520, margin: "0 auto 16px", textAlign: "left" }}>
            <Alert level="error" title="Exchange connection issues">
              {failedCreds.map((c) => (
                <div key={c.id} style={{ marginBottom: 4 }}>
                  <strong style={{ textTransform: "capitalize" }}>{c.exchange}</strong>: {c.error_reason || "Connection failed"}
                </div>
              ))}
              <div style={{ marginTop: 8 }}>
                This is likely caused by insufficient API key permissions or expired credentials.
                Go to the Exchanges tab to check your connections, or remove and re-add them with the correct permissions.
              </div>
            </Alert>
          </div>
        )}

        {loaded && activeCreds.length > 0 && (
          <div style={{ maxWidth: 520, margin: "0 auto 16px", textAlign: "left" }}>
            <Alert level="info" title="Why no transactions?">
              Your exchange connections ({activeCreds.map((c) => c.exchange).join(", ")}) are syncing
              balances only. Exchangely does not yet pull trade history from exchange APIs.
              To see transactions, import your trade history via a Ledger Live export on the Ledger tab.
            </Alert>
          </div>
        )}

        {loaded && !hasConnections && (
          <p className="text-muted" style={{ fontSize: "0.88rem" }}>
            Connect an exchange, link a wallet, or import a Ledger Live export to see transactions here.
          </p>
        )}
      </div>
    </div>
  );
}

export function TransactionsTab({ quoteCurrency, refreshKey }: TransactionsTabProps) {
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Edit modal state
  const [editingTx, setEditingTx] = useState<Transaction | null>(null);
  const [editValue, setEditValue] = useState("");
  const [editNotes, setEditNotes] = useState("");
  const [editSubmitting, setEditSubmitting] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const fetchTransactions = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await getTransactions({ page, page_size: PAGE_SIZE });
      setTransactions(res.data ?? []);
      setTotal(res.total);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load transactions");
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    fetchTransactions();
  }, [fetchTransactions, refreshKey]);

  const openEdit = (tx: Transaction) => {
    setEditingTx(tx);
    setEditValue(tx.reference_value != null ? String(tx.reference_value) : "");
    setEditNotes(tx.notes ?? "");
    setEditError(null);
  };

  const closeEdit = () => {
    setEditingTx(null);
    setEditError(null);
  };

  const handleEditSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingTx) return;
    setEditSubmitting(true);
    setEditError(null);
    try {
      const body: { reference_value?: number; notes?: string } = {};
      const parsedValue = parseFloat(editValue);
      if (editValue.trim() !== "" && !isNaN(parsedValue)) {
        body.reference_value = parsedValue;
      }
      body.notes = editNotes;
      await updateTransaction(editingTx.id, body);
      closeEdit();
      await fetchTransactions();
    } catch (err) {
      setEditError(err instanceof Error ? err.message : "Failed to update transaction");
    } finally {
      setEditSubmitting(false);
    }
  };

  if (loading) {
    return (
      <div className="settings-panel portfolio-panel-grow" style={{ alignItems: "center", justifyContent: "center" }}>
        <Spinner label="Loading transactions…" />
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

  if (transactions.length === 0 && page === 1) {
    return <TransactionsEmptyState />;
  }

  return (
    <div className="settings-panel portfolio-panel-grow">
      <div className="portfolio-manager-header" style={{ marginBottom: 12 }}>
        <h3 className="settings-panel-title" style={{ marginBottom: 0 }}>Transactions</h3>
        <span className="text-muted" style={{ fontSize: "0.82rem" }}>
          {total} transaction{total !== 1 ? "s" : ""}
        </span>
      </div>

      <Table>
        <TableHead>
          <tr>
            <th style={{ textAlign: "left" }}>Pair</th>
            <th>Quantity</th>
            <th>Type</th>
            <th>Date</th>
            <th>Total ({quoteCurrency})</th>
            <th>Fee</th>
            <th className="portfolio-col-desktop">Source</th>
            <th>Info</th>
            <th></th>
          </tr>
        </TableHead>
        <TableBody>
          {transactions.map((tx) => {
            const unitPrice = fmtUnitPrice(tx.reference_value, tx.quantity, quoteCurrency);
            return (
            <TableRow key={tx.id}>
              <TableCell align="left">
                <span className="tx-pair">
                  <span className="asset-name">{tx.asset_symbol}</span>
                  <span className="tx-pair-quote">/{quoteCurrency}</span>
                </span>
              </TableCell>
              <TableCell>
                <span className="tx-qty-cell">
                  <span>{fmtQty(tx.quantity)} {tx.asset_symbol}</span>
                  {unitPrice && (
                    <span className="tx-unit-price">@ {unitPrice}</span>
                  )}
                </span>
              </TableCell>
              <TableCell>
                <Badge variant={tx.type === "buy" ? "success" : tx.type === "sell" ? "danger" : "default"}>
                  {typeLabel(tx.type)}
                </Badge>
              </TableCell>
              <TableCell>{fmtTimestamp(tx.timestamp)}</TableCell>
              <TableCell>
                {tx.reference_value != null ? (
                  fmtValue(tx.reference_value, quoteCurrency)
                ) : (
                  <span
                    className="tx-no-value"
                    title={`No ${tx.asset_symbol}/${quoteCurrency} price data available for ${fmtTimestamp(tx.timestamp)}. The pair may not have been tracked yet, or candle data hasn't been backfilled that far.`}
                  >
                    —
                  </span>
                )}
              </TableCell>
              <TableCell>
                {fmtFee(tx.fee, tx.fee_currency || quoteCurrency)}
              </TableCell>
              <TableCell className="portfolio-col-desktop">
                <Badge variant="default">{sourceLabel(tx.source)}</Badge>
              </TableCell>
              <TableCell>
                <span style={{ display: "inline-flex", gap: 6, alignItems: "center" }}>
                  <ResolutionIndicator resolution={tx.resolution} asset={tx.asset_symbol} quoteCurrency={quoteCurrency} />
                  {tx.manually_edited && (
                    <Badge variant="accent" title="Manually edited">✎</Badge>
                  )}
                </span>
              </TableCell>
              <TableCell>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => openEdit(tx)}
                  aria-label={`Edit ${tx.asset_symbol} transaction`}
                >
                  Edit
                </Button>
              </TableCell>
            </TableRow>
            );
          })}
        </TableBody>
      </Table>

      {totalPages > 1 && (
        <div className="tx-pagination">
          <Button
            variant="secondary"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            ← Prev
          </Button>
          <span className="tx-pagination-info">
            Page {page} of {totalPages}
          </span>
          <Button
            variant="secondary"
            size="sm"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
          >
            Next →
          </Button>
        </div>
      )}

      {editingTx && (
        <Modal title={`Edit ${editingTx.asset_symbol} Transaction`} onClose={closeEdit} style={{ maxWidth: 440 }}>
          <form onSubmit={handleEditSubmit}>
            <Input
              label={`Value (${quoteCurrency})`}
              type="number"
              step="any"
              value={editValue}
              onChange={(e) => setEditValue(e.target.value)}
              placeholder="Reference currency value"
            />
            <div style={{ marginTop: 12 }}>
              <Input
                label="Notes"
                value={editNotes}
                onChange={(e) => setEditNotes(e.target.value)}
                placeholder="Optional notes"
              />
            </div>
            {editError && (
              <p style={{ color: "var(--color-down)", fontSize: "0.85rem", marginTop: 8 }}>{editError}</p>
            )}
            <Button
              type="submit"
              variant="primary"
              disabled={editSubmitting}
              style={{ width: "100%", marginTop: 16 }}
            >
              {editSubmitting ? "Saving…" : "Save Changes"}
            </Button>
          </form>
        </Modal>
      )}
    </div>
  );
}
