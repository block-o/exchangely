import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createHolding } from "../../api/portfolio";
import { fetchAssets } from "../../api/assets";
import type { CreateHoldingRequest } from "../../api/portfolio";
import type { Asset } from "../../types/api";
import { Modal, Input, Alert, Button } from "../ui";

type AddHoldingModalProps = {
  quoteCurrency: string;
  onClose: () => void;
  onCreated: () => void;
};

export function AddHoldingModal({ quoteCurrency, onClose, onCreated }: AddHoldingModalProps) {
  const [assets, setAssets] = useState<Asset[]>([]);
  const [symbol, setSymbol] = useState("");
  const [search, setSearch] = useState("");
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [quantity, setQuantity] = useState("");
  const [avgBuyPrice, setAvgBuyPrice] = useState("");
  const [notes, setNotes] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    fetchAssets()
      .then((res) => setAssets(res.data.filter((a) => a.type === "crypto")))
      .catch(() => {});
  }, []);

  const filtered = useMemo(() => {
    if (!search.trim()) return assets;
    const q = search.toLowerCase();
    return assets.filter(
      (a) => a.symbol.toLowerCase().includes(q) || a.name.toLowerCase().includes(q),
    );
  }, [assets, search]);

  const selectAsset = useCallback((a: Asset) => {
    setSymbol(a.symbol);
    setSearch(a.symbol);
    setDropdownOpen(false);
  }, []);

  const showDropdown = dropdownOpen && search.trim().length > 0 && !symbol && filtered.length > 0;
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setDropdownOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    if (!symbol) {
      setError("Please select an asset");
      return;
    }

    const qty = parseFloat(quantity);
    if (isNaN(qty) || qty <= 0) {
      setError("Quantity must be a positive number");
      return;
    }

    const req: CreateHoldingRequest = {
      asset_symbol: symbol,
      quantity: qty,
      quote_currency: quoteCurrency,
    };

    if (avgBuyPrice.trim()) {
      const abp = parseFloat(avgBuyPrice);
      if (isNaN(abp) || abp < 0) {
        setError("Average buy price must be a non-negative number");
        return;
      }
      req.avg_buy_price = abp;
    }

    if (notes.trim()) {
      req.notes = notes.trim();
    }

    setSubmitting(true);
    try {
      await createHolding(req);
      onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create holding");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal title="Add Holding" onClose={onClose} style={{ maxWidth: 440 }}>
      <form onSubmit={handleSubmit}>
        <label htmlFor="holding-symbol" className="ui-input-label">Asset</label>
        <div className="asset-picker" ref={dropdownRef}>
          <input
            ref={inputRef}
            id="holding-symbol"
            className="ui-input"
            type="text"
            required
            autoComplete="off"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setSymbol("");
              setDropdownOpen(true);
            }}
            placeholder="Start typing… e.g. BTC, Ethereum"
            autoFocus
          />
          {showDropdown && (
            <ul className="asset-picker-dropdown" role="listbox">
              {filtered.slice(0, 5).map((a) => (
                <li
                  key={a.symbol}
                  role="option"
                  aria-selected={a.symbol === symbol}
                  className={`asset-picker-option${a.symbol === symbol ? " selected" : ""}`}
                  onClick={() => selectAsset(a)}
                >
                  <span className="asset-picker-symbol">{a.symbol}</span>
                  <span className="asset-picker-name">{a.name}</span>
                </li>
              ))}
            </ul>
          )}
        </div>

        <Input
          label="Quantity"
          id="holding-qty"
          type="number"
          required
          min="0"
          step="any"
          value={quantity}
          onChange={(e) => setQuantity(e.target.value)}
          placeholder="e.g. 0.5"
          style={{ marginTop: 8 }}
        />

        <Input
          label={`Average Buy Price (${quoteCurrency}) optional`}
          id="holding-abp"
          type="number"
          min="0"
          step="any"
          value={avgBuyPrice}
          onChange={(e) => setAvgBuyPrice(e.target.value)}
          placeholder="e.g. 42000"
          style={{ marginTop: 8 }}
        />

        <Input
          label="Notes optional"
          id="holding-notes"
          type="text"
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          placeholder="e.g. Cold storage"
          style={{ marginTop: 8 }}
        />

        {error && (
          <div style={{ marginTop: 12 }}><Alert level="error">{error}</Alert></div>
        )}

        <Button
          type="submit"
          variant="primary"
          disabled={submitting || !symbol || !quantity.trim()}
          style={{ width: "100%", marginTop: 16 }}
        >
          {submitting ? "Adding…" : "Add Holding"}
        </Button>
      </form>
    </Modal>
  );
}
