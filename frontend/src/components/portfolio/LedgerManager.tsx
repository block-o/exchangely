import { useCallback, useRef, useState } from "react";
import {
  uploadLedgerExport,
  disconnectLedger,
} from "../../api/portfolio";
import { Modal, Alert, Button, Badge } from "../ui";

export function LedgerManager({ onSynced, initialShowUpload = false, onModalClose }: { onSynced: () => void; initialShowUpload?: boolean; onModalClose?: () => void }) {
  const [showUpload, setShowUpload] = useState(initialShowUpload);
  const [uploading, setUploading] = useState(false);
  const [disconnecting, setDisconnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [importedCount, setImportedCount] = useState<number | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const closeModal = () => {
    setShowUpload(false);
    onModalClose?.();
  };

  const handleFile = useCallback(async (file: File) => {
    setError(null);
    setImportedCount(null);
    setUploading(true);
    try {
      const result = await uploadLedgerExport(file);
      setImportedCount(result.imported);
      closeModal();
      onSynced();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to import Ledger export");
    } finally {
      setUploading(false);
    }
  }, [onSynced]);

  const handleFileChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) handleFile(file);
  }, [handleFile]);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const file = e.dataTransfer.files[0];
    if (file) handleFile(file);
  }, [handleFile]);

  const handleDisconnect = useCallback(async () => {
    setError(null);
    setDisconnecting(true);
    try {
      await disconnectLedger();
      setImportedCount(null);
      onSynced();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to remove Ledger holdings");
    } finally {
      setDisconnecting(false);
    }
  }, [onSynced]);

  const modalOnly = initialShowUpload && onModalClose;

  const modalContent = showUpload ? (
    <Modal title="Import Ledger Live Export" onClose={closeModal} style={{ maxWidth: 440 }}>
      <div
        className="ledger-dropzone"
        onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
        onDragLeave={() => setDragOver(false)}
        onDrop={handleDrop}
        data-dragover={dragOver || undefined}
        onClick={() => fileInputRef.current?.click()}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") fileInputRef.current?.click(); }}
        aria-label="Drop a JSON file or click to browse"
      >
        <input
          ref={fileInputRef}
          type="file"
          accept=".json,application/json"
          onChange={handleFileChange}
          style={{ display: "none" }}
        />
        <div className="ledger-dropzone-icon" aria-hidden="true">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
            <polyline points="17 8 12 3 7 8" />
            <line x1="12" y1="3" x2="12" y2="15" />
          </svg>
        </div>
        <p style={{ margin: "8px 0 4px", fontWeight: 500 }}>
          {uploading ? "Uploading…" : "Drop your Ledger Live export here"}
        </p>
        <p style={{ margin: 0, fontSize: "0.82rem", color: "var(--color-text-secondary)" }}>
          or click to browse (.json)
        </p>
      </div>

      <p style={{ margin: "16px 0 0", fontSize: "0.8rem", color: "var(--color-text-secondary)" }}>
        In Ledger Live: Settings → Accounts → Export accounts
      </p>
    </Modal>
  ) : null;

  if (modalOnly) return modalContent;

  return (
    <div className="portfolio-manager-section">
      <div className="portfolio-manager-header">
        <h3 className="settings-panel-title" style={{ marginBottom: 0 }}>Ledger Live</h3>
        <Button
          variant="primary"
          onClick={() => setShowUpload(true)}
          style={{ padding: "8px 16px", fontSize: "0.82rem" }}
        >
          Import Ledger
        </Button>
      </div>

      {error && <Alert level="error">{error}</Alert>}

      {importedCount !== null && (
        <div className="apikeys-list">
          <div className="apikeys-token-row">
            <div className="apikeys-token-info">
              <div className="apikeys-token-label-row">
                <span className="apikeys-token-label">Ledger Live Import</span>
                <Badge variant="success">imported</Badge>
              </div>
              <div className="apikeys-token-meta">
                <span>{importedCount} holding{importedCount !== 1 ? "s" : ""} imported</span>
              </div>
            </div>
            <div className="portfolio-cred-actions">
              <button
                className="apikeys-copy-btn"
                onClick={() => setShowUpload(true)}
                disabled={uploading}
              >
                Re-upload
              </button>
              <Button
                variant="danger"
                className="apikeys-revoke-btn"
                onClick={handleDisconnect}
                disabled={disconnecting}
              >
                {disconnecting ? "Removing…" : "Remove Holdings"}
              </Button>
            </div>
          </div>
        </div>
      )}

      {importedCount === null && (
        <p className="apikeys-empty">
          Upload a Ledger Live export file (.json) to import your balances.
        </p>
      )}

      {modalContent}
    </div>
  );
}
