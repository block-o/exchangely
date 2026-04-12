import { useCallback, useEffect, useState } from "react";
import {
  getCredentials,
  createCredential,
  deleteCredential,
  syncCredential,
} from "../../api/portfolio";
import type { ExchangeCredential } from "../../api/portfolio";

const EXCHANGES = ["binance", "kraken", "coinbase"] as const;

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
      await createCredential({ exchange, api_key: apiKey.trim(), api_secret: apiSecret.trim() });
      closeModal();
      setApiKey("");
      setApiSecret("");
      await fetchCredentials();
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

  // When rendered in modal-only mode (from empty state), skip the list panel.
  const modalOnly = initialShowAdd && onModalClose;

  const modalContent = showAdd ? (
    <>
      <div className="modal-backdrop" onClick={closeModal} />
      <div className="modal" role="dialog" aria-label="Add Exchange Credential" style={{ maxWidth: 440 }}>
        <div className="modal-header">
          <h3 className="settings-panel-title" style={{ marginBottom: 0 }}>Add Exchange</h3>
          <button className="icon-btn" onClick={closeModal} aria-label="Close">✕</button>
        </div>
        <form onSubmit={handleAdd}>
          <label className="login-label" htmlFor="cred-exchange">Exchange</label>
          <div className="toggle-group" style={{ marginBottom: 12 }}>
            {EXCHANGES.map((ex) => (
              <button key={ex} type="button" className={exchange === ex ? "active" : ""} onClick={() => setExchange(ex)} style={{ textTransform: "capitalize" }}>
                {ex}
              </button>
            ))}
          </div>

          <label className="login-label" htmlFor="cred-key">API Key</label>
          <input id="cred-key" className="login-input" type="text" required value={apiKey} onChange={(e) => setApiKey(e.target.value)} placeholder="Your read-only API key" />

          <label className="login-label" htmlFor="cred-secret" style={{ marginTop: 8 }}>API Secret</label>
          <input id="cred-secret" className="login-input" type="password" required value={apiSecret} onChange={(e) => setApiSecret(e.target.value)} placeholder="Your API secret" />

          {addError && <div className="login-error" role="alert" style={{ marginTop: 12 }}>{addError}</div>}

          <button type="submit" className="apikeys-create-btn" disabled={submitting || !apiKey.trim() || !apiSecret.trim()} style={{ width: "100%", marginTop: 16 }}>
            {submitting ? "Adding…" : "Add Credential"}
          </button>
        </form>
      </div>
    </>
  ) : null;

  if (modalOnly) return modalContent;

  return (
    <div className="portfolio-manager-section">
      <div className="portfolio-manager-header">
        <h3 className="settings-panel-title" style={{ marginBottom: 0 }}>Exchange Connections</h3>
        <button className="apikeys-create-btn" onClick={() => setShowAdd(true)} style={{ padding: "8px 16px", fontSize: "0.82rem" }}>
          Add Exchange
        </button>
      </div>

      {error && <div className="login-error" role="alert">{error}</div>}

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
                <button
                  className="apikeys-revoke-btn"
                  onClick={() => handleDelete(c.id)}
                  disabled={deletingId === c.id}
                >
                  {deletingId === c.id ? "Removing…" : "Remove"}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {modalContent}
    </div>
  );
}
