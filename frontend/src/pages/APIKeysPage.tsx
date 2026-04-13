import './APIKeysPage.css';
import { useCallback, useEffect, useState } from "react";
import { useAuth } from "../app/auth";
import { API_BASE_URL, authFetch } from "../api/client";
import { Alert, Badge, Button, Input, Modal } from "../components/ui";

// Types

type APIToken = {
  id: string;
  label: string;
  prefix: string;
  status: "active" | "revoked" | "expired";
  created_at: string;
  last_used_at: string | null;
  revoked_at: string | null;
  expires_at: string;
};

type RateLimitInfo = {
  limit: number;
  remaining: number;
  resetAt: number; // unix timestamp
};

// Helpers

function formatDate(iso: string | null): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function badgeVariant(status: string): "success" | "danger" | "default" {
  switch (status) {
    case "active":
      return "success";
    case "expired":
      return "danger";
    case "revoked":
    default:
      return "default";
  }
}

// Component

export function APIKeysPage() {
  const { user, isAuthenticated, isLoading } = useAuth();

  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [rateLimit, setRateLimit] = useState<RateLimitInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Create modal state
  const [showCreate, setShowCreate] = useState(false);
  const [createLabel, setCreateLabel] = useState("");
  const [creating, setCreating] = useState(false);
  const [createdToken, setCreatedToken] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  // Revoke confirmation state
  const [revokeTarget, setRevokeTarget] = useState<APIToken | null>(null);
  const [revoking, setRevoking] = useState(false);

  // Redirect unauthenticated users
  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      window.location.hash = "#login";
    }
  }, [isLoading, isAuthenticated]);

  // Fetch tokens

  const fetchTokens = useCallback(async () => {
    try {
      const res = await authFetch(`${API_BASE_URL}/auth/api-tokens`, {
        credentials: "include",
      });
      if (!res.ok) throw new Error(`Failed to load tokens (${res.status})`);

      // Parse rate limit headers
      const rlLimit = res.headers.get("X-RateLimit-Limit");
      const rlRemaining = res.headers.get("X-RateLimit-Remaining");
      const rlReset = res.headers.get("X-RateLimit-Reset");
      if (rlLimit && rlRemaining && rlReset) {
        setRateLimit({
          limit: parseInt(rlLimit, 10),
          remaining: parseInt(rlRemaining, 10),
          resetAt: parseInt(rlReset, 10),
        });
      }

      const data = await res.json();
      setTokens(data.data ?? []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load tokens");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (isAuthenticated) fetchTokens();
  }, [isAuthenticated, fetchTokens]);

  // Create token

  const handleCreate = useCallback(async () => {
    setCreateError(null);
    setCreating(true);
    try {
      const res = await authFetch(`${API_BASE_URL}/auth/api-tokens`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ label: createLabel.trim() }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        throw new Error(body?.error ?? `Request failed (${res.status})`);
      }
      const data = await res.json();
      setCreatedToken(data.token);
      setCopied(false);
      // Refresh the list
      await fetchTokens();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : "Failed to create token");
    } finally {
      setCreating(false);
    }
  }, [createLabel, fetchTokens]);

  const handleCopy = useCallback(async () => {
    if (!createdToken) return;
    try {
      await navigator.clipboard.writeText(createdToken);
      setCopied(true);
    } catch {
      // Fallback: select text for manual copy
    }
  }, [createdToken]);

  const closeCreateModal = useCallback(() => {
    setShowCreate(false);
    setCreateLabel("");
    setCreatedToken(null);
    setCreateError(null);
    setCopied(false);
  }, []);

  // Revoke token

  const handleRevoke = useCallback(async () => {
    if (!revokeTarget) return;
    setRevoking(true);
    try {
      const res = await authFetch(
        `${API_BASE_URL}/auth/api-tokens/${revokeTarget.id}`,
        { method: "DELETE", credentials: "include" },
      );
      if (!res.ok && res.status !== 204) {
        throw new Error(`Revoke failed (${res.status})`);
      }
      // Update inline without full reload
      setTokens((prev) =>
        prev.map((t) =>
          t.id === revokeTarget.id
            ? { ...t, status: "revoked", revoked_at: new Date().toISOString() }
            : t,
        ),
      );
      setRevokeTarget(null);
    } catch {
      setError("Failed to revoke token");
      setRevokeTarget(null);
    } finally {
      setRevoking(false);
    }
  }, [revokeTarget]);

  // Render guards

  if (isLoading) {
    return (
      <section className="settings-page">
        <div className="settings-loading">Loading…</div>
      </section>
    );
  }

  if (!user) return null;

  // Render

  return (
    <section className="settings-page">
      <div className="settings-container">
        {/* Header */}
        <div className="settings-panel">
          <div className="apikeys-header">
            <h2 className="settings-panel-title" style={{ marginBottom: 0 }}>
              API Keys
            </h2>
            <Button
              variant="primary"
              className="apikeys-create-btn"
              onClick={() => setShowCreate(true)}
            >
              Create API Key
            </Button>
          </div>
          <p className="apikeys-subtitle">
            Manage your API tokens for programmatic access to Exchangely data.
          </p>
        </div>

        {/* Rate Limit Info */}
        {rateLimit && (
          <div className="settings-panel">
            <h2 className="settings-panel-title">Rate Limit Usage</h2>
            <div className="apikeys-ratelimit">
              <div className="apikeys-ratelimit-bar-track">
                <div
                  className="apikeys-ratelimit-bar-fill"
                  style={{
                    width: `${Math.min(((rateLimit.limit - rateLimit.remaining) / rateLimit.limit) * 100, 100)}%`,
                  }}
                />
              </div>
              <div className="apikeys-ratelimit-text">
                <span>
                  {rateLimit.limit - rateLimit.remaining} / {rateLimit.limit}{" "}
                  requests used
                </span>
                <span className="apikeys-ratelimit-remaining">
                  {rateLimit.remaining} remaining
                </span>
              </div>
            </div>
          </div>
        )}

        {/* Error */}
        {error && (
          <Alert level="error">{error}</Alert>
        )}

        {/* Token List */}
        <div className="settings-panel">
          <h2 className="settings-panel-title">Your Tokens</h2>
          {loading ? (
            <div className="settings-loading">Loading tokens…</div>
          ) : tokens.length === 0 ? (
            <p className="apikeys-empty">
              No API keys yet. Create one to get started.
            </p>
          ) : (
            <div className="apikeys-list">
              {tokens.map((token) => (
                <div key={token.id} className="apikeys-token-row">
                  <div className="apikeys-token-info">
                    <div className="apikeys-token-label-row">
                      <span className="apikeys-token-label">{token.label}</span>
                      <Badge variant={badgeVariant(token.status)} className="apikeys-badge">
                        {token.status}
                      </Badge>
                    </div>
                    <div className="apikeys-token-meta">
                      <code className="apikeys-token-prefix">{token.prefix}…</code>
                      <span>Created {formatDate(token.created_at)}</span>
                      <span>
                        Last used {formatDate(token.last_used_at)}
                      </span>
                    </div>
                  </div>
                  {token.status === "active" && (
                    <Button
                      variant="danger"
                      className="apikeys-revoke-btn"
                      onClick={() => setRevokeTarget(token)}
                    >
                      Revoke
                    </Button>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Create Modal */}
      {showCreate && (
        <Modal
          title={createdToken ? "API Key Created" : "Create API Key"}
          onClose={closeCreateModal}
        >
          {createdToken ? (
            <div className="apikeys-created-result">
              <Alert level="warning">
                Copy this token now. It will not be shown again.
              </Alert>
              <div className="apikeys-token-display">
                <code className="apikeys-raw-token">{createdToken}</code>
                <Button
                  variant="secondary"
                  className="apikeys-copy-btn"
                  onClick={handleCopy}
                >
                  {copied ? "Copied!" : "Copy"}
                </Button>
              </div>
              <Button
                variant="primary"
                className="apikeys-create-btn"
                onClick={closeCreateModal}
                style={{ width: "100%", marginTop: 12 }}
              >
                Done
              </Button>
            </div>
          ) : (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                handleCreate();
              }}
            >
              <Input
                label="Label"
                id="apikey-label"
                type="text"
                required
                value={createLabel}
                onChange={(e) => setCreateLabel(e.target.value)}
                placeholder="e.g. My Trading Bot"
                autoFocus
              />
              {createError && (
                <Alert level="error" className="apikeys-create-error">
                  {createError}
                </Alert>
              )}
              <Button
                type="submit"
                variant="primary"
                className="apikeys-create-btn"
                disabled={creating || !createLabel.trim()}
                style={{ width: "100%", marginTop: 16 }}
              >
                {creating ? "Creating…" : "Create Token"}
              </Button>
            </form>
          )}
        </Modal>
      )}

      {/* Revoke Confirmation */}
      {revokeTarget && (
        <Modal title="Revoke API Key" onClose={() => setRevokeTarget(null)}>
          <p style={{ color: "var(--color-text-secondary)", margin: "0 0 8px" }}>
            Are you sure you want to revoke{" "}
            <strong style={{ color: "var(--color-text-primary)" }}>
              {revokeTarget.label}
            </strong>
            ? This action cannot be undone.
          </p>
          <p style={{ color: "var(--color-text-secondary)", fontSize: "0.85rem", margin: "0 0 20px" }}>
            Any applications using this key will lose access immediately.
          </p>
          <div style={{ display: "flex", gap: 12 }}>
            <Button
              variant="ghost"
              className="apikeys-cancel-btn"
              onClick={() => setRevokeTarget(null)}
              style={{ flex: 1 }}
            >
              Cancel
            </Button>
            <Button
              variant="danger"
              className="apikeys-revoke-confirm-btn"
              onClick={handleRevoke}
              disabled={revoking}
              style={{ flex: 1 }}
            >
              {revoking ? "Revoking…" : "Revoke"}
            </Button>
          </div>
        </Modal>
      )}
    </section>
  );
}
