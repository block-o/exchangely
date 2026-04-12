import type { AssetValuation } from "../../api/portfolio";

type AllocationChartProps = {
  assets: AssetValuation[];
};

const COLORS = [
  "#00f0ff", "#10b981", "#f59e0b", "#8b5cf6",
  "#ef4444", "#ec4899", "#06b6d4", "#84cc16",
  "#f97316", "#6366f1", "#14b8a6", "#e11d48",
];

function buildSlices(assets: AssetValuation[]) {
  const priced = assets.filter((a) => a.priced && a.allocation_pct > 0);
  priced.sort((a, b) => b.allocation_pct - a.allocation_pct);
  return priced;
}

function buildConicGradient(slices: AssetValuation[]): string {
  if (slices.length === 0) return "conic-gradient(var(--color-bg-card) 0deg 360deg)";
  const segments: string[] = [];
  let cumulative = 0;
  slices.forEach((s, i) => {
    const color = COLORS[i % COLORS.length];
    const start = cumulative;
    cumulative += s.allocation_pct;
    segments.push(`${color} ${start}% ${cumulative}%`);
  });
  return `conic-gradient(${segments.join(", ")})`;
}

export function AllocationChart({ assets }: AllocationChartProps) {
  const slices = buildSlices(assets);

  if (slices.length === 0) {
    return (
      <div className="portfolio-allocation">
        <h3 className="settings-panel-title">Allocation</h3>
        <div className="portfolio-allocation-empty">No priced assets</div>
      </div>
    );
  }

  return (
    <div className="portfolio-allocation">
      <h3 className="settings-panel-title">Allocation</h3>
      <div className="portfolio-allocation-content">
        <div
          className="portfolio-pie"
          style={{ background: buildConicGradient(slices) }}
          role="img"
          aria-label="Portfolio allocation pie chart"
        >
          <div className="portfolio-pie-center" />
        </div>
        <div className="portfolio-allocation-legend">
          {slices.map((s, i) => (
            <div key={s.asset_symbol} className="portfolio-legend-item">
              <span
                className="portfolio-legend-dot"
                style={{ background: COLORS[i % COLORS.length] }}
              />
              <span className="portfolio-legend-symbol">{s.asset_symbol}</span>
              <span className="portfolio-legend-pct">{s.allocation_pct.toFixed(1)}%</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
