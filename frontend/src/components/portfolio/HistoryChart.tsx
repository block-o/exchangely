import { useCallback, useEffect, useState } from "react";
import { getHistory } from "../../api/portfolio";
import type { HistoricalPoint } from "../../api/portfolio";

type HistoryChartProps = {
  quoteCurrency: string;
  refreshKey?: number;
};

const RANGES = ["1d", "7d", "30d", "1y"] as const;
type Range = (typeof RANGES)[number];

function formatValue(value: number, currency: string): string {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency,
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(value);
}

function formatTimestamp(ts: number, range: Range): string {
  const d = new Date(ts * 1000);
  if (range === "1d") return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  if (range === "7d") return d.toLocaleDateString(undefined, { weekday: "short", hour: "2-digit" });
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

export function HistoryChart({ quoteCurrency, refreshKey }: HistoryChartProps) {
  const [range, setRange] = useState<Range>("7d");
  const [data, setData] = useState<HistoricalPoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchData = useCallback(async (r: Range) => {
    setLoading(true);
    setError(null);
    try {
      const points = await getHistory(r, quoteCurrency);
      setData(points);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load history");
      setData([]);
    } finally {
      setLoading(false);
    }
  }, [quoteCurrency]);

  useEffect(() => {
    fetchData(range);
  }, [range, fetchData, refreshKey]);

  const handleRange = (r: Range) => setRange(r);

  const minVal = data.length > 0 ? Math.min(...data.map((p) => p.value)) : 0;
  const maxVal = data.length > 0 ? Math.max(...data.map((p) => p.value)) : 0;
  const spread = maxVal - minVal || 1;

  // Build SVG polyline points
  const chartWidth = 600;
  const chartHeight = 200;
  const points = data.map((p, i) => {
    const x = data.length > 1 ? (i / (data.length - 1)) * chartWidth : chartWidth / 2;
    const y = chartHeight - ((p.value - minVal) / spread) * (chartHeight - 20) - 10;
    return `${x},${y}`;
  }).join(" ");

  const isPositive = data.length >= 2 && data[data.length - 1].value >= data[0].value;
  const lineColor = isPositive ? "var(--color-up)" : "var(--color-down)";

  return (
    <div className="portfolio-history">
      <div className="portfolio-history-header">
        <h3 className="settings-panel-title" style={{ marginBottom: 0 }}>History</h3>
        <div className="toggle-group portfolio-range-toggle">
          {RANGES.map((r) => (
            <button key={r} className={range === r ? "active" : ""} onClick={() => handleRange(r)}>
              {r}
            </button>
          ))}
        </div>
      </div>

      {loading ? (
        <div className="portfolio-history-loading">Loading chart…</div>
      ) : error ? (
        <div className="portfolio-history-error">{error}</div>
      ) : data.length === 0 ? (
        <div className="portfolio-history-empty">No historical data available</div>
      ) : (
        <div className="portfolio-history-chart">
          <div className="portfolio-history-labels">
            <span>{formatValue(maxVal, quoteCurrency)}</span>
            <span>{formatValue(minVal, quoteCurrency)}</span>
          </div>
          <svg
            viewBox={`0 0 ${chartWidth} ${chartHeight}`}
            className="portfolio-history-svg"
            preserveAspectRatio="none"
            role="img"
            aria-label="Historical portfolio value chart"
          >
            <defs>
              <linearGradient id="historyFill" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={lineColor} stopOpacity="0.3" />
                <stop offset="100%" stopColor={lineColor} stopOpacity="0" />
              </linearGradient>
            </defs>
            {data.length > 1 && (
              <>
                <polygon
                  points={`0,${chartHeight} ${points} ${chartWidth},${chartHeight}`}
                  fill="url(#historyFill)"
                />
                <polyline
                  points={points}
                  fill="none"
                  stroke={lineColor}
                  strokeWidth="2"
                  vectorEffect="non-scaling-stroke"
                />
              </>
            )}
          </svg>
          <div className="portfolio-history-time-labels">
            <span>{data.length > 0 ? formatTimestamp(data[0].timestamp, range) : ""}</span>
            <span>{data.length > 1 ? formatTimestamp(data[data.length - 1].timestamp, range) : ""}</span>
          </div>
        </div>
      )}
    </div>
  );
}
