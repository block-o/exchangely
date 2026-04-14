import { useCallback, useEffect, useState } from "react";
import {
  getCredentials,
  createCredential,
  deleteCredential,
  syncCredential,
} from "../../api/portfolio";
import type { ExchangeCredential } from "../../api/portfolio";
import { Modal, Input, Alert, Button, ToggleGroup } from "../ui";

const EXCHANGES = ["binance", "kraken", "coinbase"] as const;
const EXCHANGE_OPTIONS = EXCHANGES.map((ex) => ({ value: ex, label: ex.charAt(0).toUpperCase() + ex.slice(1) }));

// Each exchange has different credential naming and formats.
type ExchangeFieldConfig = {
  keyLabel: string;
  keyPlaceholder: string;
  keyHint?: string;
  secretLabel: string;
  secretPlaceholder: string;
  secretHint?: string;
  secretMultiline?: boolean;
};

const EXCHANGE_FIELDS: Record<string, ExchangeFieldConfig> = {
  binance: {
    keyLabel: "API Key",
    keyPlaceholder: "Your read-only API key",
    secretLabel: "API Secret",
    secretPlaceholder: "Your API secret",
  },
  kraken: {
    keyLabel: "API Key",
    keyPlaceholder: "Your read-only API key",
    keyHint: "Found in Kraken → Settings → API → Create Key",
    secretLabel: "Private Key",
    secretPlaceholder: "Base64-encoded private key",
    secretHint: "The long base64 string shown once when you create the key",
  },
  coinbase: {
    keyLabel: "API Key Name",
    keyPlaceholder: "organizations/…/apiKeys/…",
    keyHint: "The full path shown in CDP Portal → API Keys",
    secretLabel: "Private Key",
    secretPlaceholder: "Starts with -----BEGIN EC PRIVATE KEY-----",
    secretHint: "PEM-encoded EC key, downloaded once at creation",
    secretMultiline: true,
  },
};

function formatDate(iso: string | null | undefined): string {
  if (!iso) return "Never";
  return new Date(iso).toLocaleDateString(undefined, {
    year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit",
  });
}

export function ExchangeCredentialManager({ onSynced, initialShowAdd = false, onModalClose }: { onSynced: () => void; initialShowAdd?: boolean; onModalClose?: () => void }) {
  const [credentials, setCredentials] = useState<ExchangeCredential[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [showAdd, setShowAdd] = useState(initialShowAdd);
  const [exchange, setExchange] = useState<string>(EXCHANGES[0]);

  const closeModal = () => {
    setShowAdd(false);
    onModalClose?.();
  };
  const [apiKey, setApiKey] = useState("");
  const [apiSecret, setApiSecret] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);

  const [syncingId, setSyncingId] = useState<string | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const fetchCredentials = useCallback(async () => {
    try {
      const data = await getCredentials();
      setCredentials(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load credentials");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchCredentials(); }, [fetchCredentials]);

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    setAddError(null);
    setSubmitting(true);
    try {
      const cred = await createCredential({ exchange, api_key: apiKey.trim(), api_secret: apiSecret.trim() });
      setApiKey("");
      setApiSecret("");
      // Auto-sync to pull balances immediately after connecting.
      try {
        await syncCredential(cred.id);
      } catch {
        // Sync failure is non-fatal — credential is saved, user can retry.
      }
      await fetchCredentials();
      onSynced();
      closeModal();
    } catch (err) {
      setAddError(err instanceof Error ? err.message : "Failed to add credential");
    } finally {
      setSubmitting(false);
    }
  };

  const handleSync = async (id: string) => {
    setSyncingId(id);
    try {
      await syncCredential(id);
      await fetchCredentials();
      onSynced();
    } catch {
      setError("Sync failed");
    } finally {
      setSyncingId(null);
    }
  };

  const handleDelete = async (id: string) => {
    setDeletingId(id);
    try {
      await deleteCredential(id);
      setCredentials((prev) => prev.filter((c) => c.id !== id));
      onSynced();
    } catch {
      setError("Delete failed");
    } finally {
      setDeletingId(null);
    }
  };

  const modalOnly = initialShowAdd && onModalClose;

  const modalContent = showAdd ? (
    <Modal title="Add Exchange" onClose={closeModal} style={{ maxWidth: 440 }}>
      <form onSubmit={handleAdd}>
        <label className="ui-input-label" htmlFor="cred-exchange">Exchange</label>
        <ToggleGroup
          options={EXCHANGE_OPTIONS}
          value={exchange}
          onChange={(v) => setExchange(v)}
          style={{ marginBottom: 12 }}
        />

        <Input label={EXCHANGE_FIELDS[exchange]?.keyLabel ?? "API Key"} id="cred-key" type="password" required value={apiKey} onChange={(e) => setApiKey(e.target.value)} placeholder={EXCHANGE_FIELDS[exchange]?.keyPlaceholder ?? "Your read-only API key"} />
        {EXCHANGE_FIELDS[exchange]?.keyHint && (
          <p className="portfolio-field-hint">{EXCHANGE_FIELDS[exchange].keyHint}</p>
        )}

        {EXCHANGE_FIELDS[exchange]?.secretMultiline ? (
          <>
            <label className="ui-input-label" htmlFor="cred-secret" style={{ marginTop: 8 }}>{EXCHANGE_FIELDS[exchange]?.secretLabel ?? "API Secret"}</label>
            <textarea
              id="cred-secret"
              className="ui-input portfolio-secret-textarea"
              required
              value={apiSecret}
              onChange={(e) => setApiSecret(e.target.value)}
              placeholder={EXCHANGE_FIELDS[exchange]?.secretPlaceholder ?? "Your API secret"}
              rows={4}
              style={{ fontSize: "0.82rem", resize: "vertical" }}
            />
          </>
        ) : (
          <Input label={EXCHANGE_FIELDS[exchange]?.secretLabel ?? "API Secret"} id="cred-secret" type="password" required value={apiSecret} onChange={(e) => setApiSecret(e.target.value)} placeholder={EXCHANGE_FIELDS[exchange]?.secretPlaceholder ?? "Your API secret"} style={{ marginTop: 8 }} />
        )}
        {EXCHANGE_FIELDS[exchange]?.secretHint && (
          <p className="portfolio-field-hint">{EXCHANGE_FIELDS[exchange].secretHint}</p>
        )}

        {addError && <div style={{ marginTop: 12 }}><Alert level="error">{addError}</Alert></div>}

        <Button type="submit" variant="primary" disabled={submitting || !apiKey.trim() || !apiSecret.trim()} style={{ width: "100%", marginTop: 16 }}>
          {submitting ? "Connecting…" : "Connect Exchange"}
        </Button>
      </form>
    </Modal>
  ) : null;

  if (modalOnly) return modalContent;

  return (
    <div className="portfolio-manager-section">
      <div className="portfolio-manager-header">
        <h3 className="settings-panel-title" style={{ marginBottom: 0 }}>Exchange Connections</h3>
        <Button variant="primary" onClick={() => setShowAdd(true)} style={{ padding: "8px 16px", fontSize: "0.82rem" }}>
          Add Exchange
        </Button>
      </div>

      {error && <Alert level="error">{error}</Alert>}

      {loading ? (
        <div className="settings-loading">Loading…</div>
      ) : credentials.length === 0 ? (
        <p className="apikeys-empty">No exchange connections yet.</p>
      ) : (
        <div className="apikeys-list">
          {credentials.map((c) => (
            <div key={c.id} className="apikeys-token-row">
              <div className="apikeys-token-info">
                <div className="apikeys-token-label-row">
                  <span className="apikeys-token-label" style={{ textTransform: "capitalize" }}>{c.exchange}</span>
                  <span className={`apikeys-badge ${c.status === "active" ? "apikeys-badge-active" : "apikeys-badge-revoked"}`}>
                    {c.status}
                  </span>
                </div>
                <div className="apikeys-token-meta">
                  <code className="apikeys-token-prefix">{c.api_key_prefix}…</code>
                  <span>Last sync: {formatDate(c.last_sync_at)}</span>
                </div>
                {c.error_reason && <div className="portfolio-cred-error">{c.error_reason}</div>}
              </div>
              <div className="portfolio-cred-actions">
                <button
                  className="apikeys-copy-btn"
                  onClick={() => handleSync(c.id)}
                  disabled={syncingId === c.id}
                >
                  {syncingId === c.id ? "Syncing…" : "Sync"}
                </button>
                <Button
                  variant="danger"
                  className="apikeys-revoke-btn"
                  onClick={() => handleDelete(c.id)}
                  disabled={deletingId === c.id}
                >
                  {deletingId === c.id ? "Removing…" : "Remove"}
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {modalContent}
    </div>
  );
}
