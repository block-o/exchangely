import { useCallback, useEffect, useState } from "react";
import {
  getWallets,
  createWallet,
  deleteWallet,
  syncWallet,
} from "../../api/portfolio";
import type { WalletAddress } from "../../api/portfolio";

const CHAINS = ["ethereum", "solana", "bitcoin"] as const;

function formatDate(iso: string | null | undefined): string {
  if (!iso) return "Never";
  return new Date(iso).toLocaleDateString(undefined, {
    year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit",
  });
}

export function WalletManager({ onSynced, initialShowAdd = false, onModalClose }: { onSynced: () => void; initialShowAdd?: boolean; onModalClose?: () => void }) {
  const [wallets, setWallets] = useState<WalletAddress[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [showAdd, setShowAdd] = useState(initialShowAdd);
  const [chain, setChain] = useState<string>(CHAINS[0]);

  const closeModal = () => {
    setShowAdd(false);
    onModalClose?.();
  };
  const [address, setAddress] = useState("");
  const [label, setLabel] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);

  const [syncingId, setSyncingId] = useState<string | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const fetchWallets = useCallback(async () => {
    try {
      const data = await getWallets();
      setWallets(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load wallets");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchWallets(); }, [fetchWallets]);

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    setAddError(null);
    setSubmitting(true);
    try {
      await createWallet({ chain, address: address.trim(), label: label.trim() || undefined });
      closeModal();
      setAddress("");
      setLabel("");
      await fetchWallets();
    } catch (err) {
      setAddError(err instanceof Error ? err.message : "Failed to add wallet");
    } finally {
      setSubmitting(false);
    }
  };

  const handleSync = async (id: string) => {
    setSyncingId(id);
    try {
      await syncWallet(id);
      await fetchWallets();
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
      await deleteWallet(id);
      setWallets((prev) => prev.filter((w) => w.id !== id));
      onSynced();
    } catch {
      setError("Delete failed");
    } finally {
      setDeletingId(null);
    }
  };

  const modalOnly = initialShowAdd && onModalClose;

  const modalContent = showAdd ? (
    <>
      <div className="modal-backdrop" onClick={closeModal} />
      <div className="modal" role="dialog" aria-label="Link Wallet" style={{ maxWidth: 440 }}>
        <div className="modal-header">
          <h3 className="settings-panel-title" style={{ marginBottom: 0 }}>Link Wallet</h3>
          <button className="icon-btn" onClick={closeModal} aria-label="Close">✕</button>
        </div>
        <form onSubmit={handleAdd}>
          <label className="login-label">Chain</label>
          <div className="toggle-group" style={{ marginBottom: 12 }}>
            {CHAINS.map((c) => (
              <button key={c} type="button" className={chain === c ? "active" : ""} onClick={() => setChain(c)} style={{ textTransform: "capitalize" }}>
                {c}
              </button>
            ))}
          </div>

          <label className="login-label" htmlFor="wallet-addr">Wallet Address</label>
          <input id="wallet-addr" className="login-input" type="text" required value={address} onChange={(e) => setAddress(e.target.value)} placeholder={chain === "ethereum" ? "0x…" : chain === "bitcoin" ? "bc1…" : "Base58 address"} />

          <label className="login-label" htmlFor="wallet-label" style={{ marginTop: 8 }}>
            Label <span style={{ color: "var(--color-text-secondary)", fontWeight: 400 }}>optional</span>
          </label>
          <input id="wallet-label" className="login-input" type="text" value={label} onChange={(e) => setLabel(e.target.value)} placeholder="e.g. Main wallet" />

          {addError && <div className="login-error" role="alert" style={{ marginTop: 12 }}>{addError}</div>}

          <button type="submit" className="apikeys-create-btn" disabled={submitting || !address.trim()} style={{ width: "100%", marginTop: 16 }}>
            {submitting ? "Linking…" : "Link Wallet"}
          </button>
        </form>
      </div>
    </>
  ) : null;

  if (modalOnly) return modalContent;

  return (
    <div className="portfolio-manager-section">
      <div className="portfolio-manager-header">
        <h3 className="settings-panel-title" style={{ marginBottom: 0 }}>Linked Wallets</h3>
        <button className="apikeys-create-btn" onClick={() => setShowAdd(true)} style={{ padding: "8px 16px", fontSize: "0.82rem" }}>
          Link Wallet
        </button>
      </div>

      {error && <div className="login-error" role="alert">{error}</div>}

      {loading ? (
        <div className="settings-loading">Loading…</div>
      ) : wallets.length === 0 ? (
        <p className="apikeys-empty">No wallets linked yet.</p>
      ) : (
        <div className="apikeys-list">
          {wallets.map((w) => (
            <div key={w.id} className="apikeys-token-row">
              <div className="apikeys-token-info">
                <div className="apikeys-token-label-row">
                  <span className="apikeys-token-label" style={{ textTransform: "capitalize" }}>{w.chain}</span>
                  {w.label && <span className="portfolio-wallet-label">{w.label}</span>}
                </div>
                <div className="apikeys-token-meta">
                  <code className="apikeys-token-prefix">{w.address_prefix}…</code>
                  <span>Last sync: {formatDate(w.last_sync_at)}</span>
                </div>
              </div>
              <div className="portfolio-cred-actions">
                <button
                  className="apikeys-copy-btn"
                  onClick={() => handleSync(w.id)}
                  disabled={syncingId === w.id}
                >
                  {syncingId === w.id ? "Syncing…" : "Sync"}
                </button>
                <button
                  className="apikeys-revoke-btn"
                  onClick={() => handleDelete(w.id)}
                  disabled={deletingId === w.id}
                >
                  {deletingId === w.id ? "Removing…" : "Remove"}
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
