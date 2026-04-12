import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ValuationHeader } from "./ValuationHeader";
import type { Valuation } from "../../api/portfolio";

// Mock the SSE subscription to avoid real EventSource usage
vi.mock("../../api/portfolio", () => ({
  subscribePortfolioStream: vi.fn(() => ({
    close: vi.fn(),
    onopen: null,
    onmessage: null,
    onerror: null,
  })),
}));

function makeValuation(overrides: Partial<Valuation> = {}): Valuation {
  return {
    total_value: 42000.5,
    quote_currency: "USD",
    assets: [
      { asset_symbol: "BTC", quantity: 1, current_price: 42000.5, current_value: 42000.5, allocation_pct: 100, priced: true, source: "manual" },
    ],
    updated_at: "2024-01-15T10:30:00Z",
    ...overrides,
  };
}

describe("ValuationHeader", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows loading state when loading is true", () => {
    render(<ValuationHeader valuation={null} loading={true} />);

    expect(screen.getByText("Loading portfolio…")).toBeInTheDocument();
  });

  it("displays total portfolio value formatted as currency", () => {
    render(<ValuationHeader valuation={makeValuation()} loading={false} />);

    // The formatted value should contain the number (locale-dependent formatting)
    expect(screen.getByText("Total Portfolio Value")).toBeInTheDocument();
    // The value element should exist with a currency-formatted string
    const valueEl = document.querySelector(".portfolio-valuation-value");
    expect(valueEl).toBeTruthy();
    expect(valueEl!.textContent).toContain("42,000.50");
  });

  it("displays asset count", () => {
    const valuation = makeValuation({
      assets: [
        { asset_symbol: "BTC", quantity: 1, current_price: 40000, current_value: 40000, allocation_pct: 80, priced: true, source: "manual" },
        { asset_symbol: "ETH", quantity: 10, current_price: 2000, current_value: 20000, allocation_pct: 20, priced: true, source: "manual" },
      ],
    });

    render(<ValuationHeader valuation={valuation} loading={false} />);

    expect(screen.getByText("2 assets tracked")).toBeInTheDocument();
  });

  it("uses singular 'asset' for single priced asset", () => {
    render(<ValuationHeader valuation={makeValuation()} loading={false} />);

    expect(screen.getByText("1 asset tracked")).toBeInTheDocument();
  });

  it("excludes unpriced assets from count", () => {
    const valuation = makeValuation({
      assets: [
        { asset_symbol: "BTC", quantity: 1, current_price: 40000, current_value: 40000, allocation_pct: 100, priced: true, source: "manual" },
        { asset_symbol: "OBSCURE", quantity: 100, current_price: 0, current_value: 0, allocation_pct: 0, priced: false, source: "manual" },
      ],
    });

    render(<ValuationHeader valuation={valuation} loading={false} />);

    expect(screen.getByText("1 asset tracked")).toBeInTheDocument();
  });

  it("shows stream status indicator", () => {
    const { container } = render(
      <ValuationHeader valuation={makeValuation()} loading={false} />,
    );

    const dot = container.querySelector(".portfolio-stream-dot");
    expect(dot).toBeTruthy();
  });
});
