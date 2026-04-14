import { useEffect, useRef, useState } from "react";
import { subscribePortfolioStream } from "../../api/portfolio";
import type { Valuation } from "../../api/portfolio";
import { Button } from "../ui";

type ValuationHeaderProps = {
  valuation: Valuation | null;
  loading: boolean;
  onSyncAll?: () => Promise<void>;
};

function formatCurrency(value: number, currency: string): string {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value);
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export function ValuationHeader({ valuation, loading, onSyncAll }: ValuationHeaderProps) {
  const [liveValue, setLiveValue] = useState<number | null>(null);
  const [liveUpdatedAt, setLiveUpdatedAt] = useState<string | null>(null);
  const [isLive, setIsLive] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    const es = subscribePortfolioStream();
    esRef.current = es;

    const handlePortfolio = (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data) as Valuation;
        setLiveValue(data.total_value);
        setLiveUpdatedAt(data.updated_at);
        setIsLive(true);
      } catch {
        // ignore malformed events
      }
    };

    // Backend sends named "portfolio" events, so we must use addEventListener
    // instead of onmessage (which only catches unnamed events).
    es.addEventListener("portfolio", handlePortfolio as EventListener);

    es.onerror = () => {
      setIsLive(false);
    };

    es.onopen = () => {
      setIsLive(true);
    };

    return () => {
      es.removeEventListener("portfolio", handlePortfolio as EventListener);
      es.close();
      esRef.current = null;
    };
  }, []);

  // Reset live overrides when the parent refreshes valuation data
  // (e.g. after a holding is added or removed).
  useEffect(() => {
    setLiveValue(null);
    setLiveUpdatedAt(null);
  }, [valuation]);

  const displayValue = liveValue ?? valuation?.total_value ?? 0;
  const displayCurrency = valuation?.quote_currency ?? "USD";
  const displayTime = liveUpdatedAt ?? valuation?.updated_at;
  const assetCount = valuation?.assets?.filter((a) => a.priced).length ?? 0;

  const handleSyncAll = async () => {
    if (!onSyncAll || syncing) return;
    setSyncing(true);
    try {
      await onSyncAll();
    } finally {
      setSyncing(false);
    }
  };

  if (loading) {
    return (
      <div className="portfolio-valuation-header">
        <div className="portfolio-valuation-loading">Loading portfolio…</div>
      </div>
    );
  }

  return (
    <div className="portfolio-valuation-header">
      <div className="portfolio-valuation-top">
        <div className="portfolio-valuation-label">Total Portfolio Value</div>
        <div className="portfolio-valuation-stream">
          <span className={`portfolio-stream-dot ${isLive ? "live" : ""}`} />
          <span className="portfolio-stream-label">{isLive ? "Live" : "Offline"}</span>
          {onSyncAll && (
            <Button
              variant="icon"
              className="portfolio-sync-all-btn"
              onClick={handleSyncAll}
              disabled={syncing}
              aria-label="Sync all sources"
              title="Sync all sources"
            >
              {syncing ? "Syncing\u2026" : "\u21BB"}
            </Button>
          )}
        </div>
      </div>
      <div className="portfolio-valuation-value">
        {formatCurrency(displayValue, displayCurrency)}
      </div>
      <div className="portfolio-valuation-meta">
        <span>{assetCount} asset{assetCount !== 1 ? "s" : ""} tracked</span>
        {displayTime && <span>Updated {formatTime(displayTime)}</span>}
      </div>
    </div>
  );
}
