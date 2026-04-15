import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PnLTab } from "./PnLTab";
import type { PnLSnapshot } from "../../api/portfolio";

// Mock the portfolio API module
const mockGetPnL = vi.fn();

vi.mock("../../api/portfolio", () => ({
  getPnL: (...args: unknown[]) => mockGetPnL(...args),
  getTransactions: vi.fn(() => Promise.resolve({ data: [], total: 0, page: 1, page_size: 1 })),
}));

function makeSnapshot(overrides: Partial<PnLSnapshot> = {}): PnLSnapshot {
  return {
    id: "snap-1",
    user_id: "u-1",
    reference_currency: "USD",
    total_realized: 1750.0,
    total_unrealized: 920.0,
    total_pnl: 2670.0,
    has_approximate: false,
    excluded_count: 0,
    assets: [
      {
        asset_symbol: "BTC",
        realized_pnl: 1000.0,
        unrealized_pnl: 500.0,
        total_pnl: 1500.0,
        transaction_count: 12,
      },
      {
        asset_symbol: "ETH",
        realized_pnl: 750.0,
        unrealized_pnl: 420.0,
        total_pnl: 1170.0,
        transaction_count: 8,
      },
    ],
    computed_at: "2026-03-15T12:00:00Z",
    ...overrides,
  };
}

describe("PnLTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("summary display", () => {
    it("renders summary with realized, unrealized, and total P&L values", async () => {
      mockGetPnL.mockResolvedValue(makeSnapshot());

      render(<PnLTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("Realized P&L")).toBeInTheDocument();
      });
      expect(screen.getByText("Unrealized P&L")).toBeInTheDocument();
      expect(screen.getByText("Total P&L")).toBeInTheDocument();
      expect(screen.getByText("$1,750.00")).toBeInTheDocument();
      expect(screen.getByText("$920.00")).toBeInTheDocument();
      expect(screen.getByText("$2,670.00")).toBeInTheDocument();
    });

    it("renders per-asset breakdown table", async () => {
      mockGetPnL.mockResolvedValue(makeSnapshot());

      render(<PnLTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("BTC")).toBeInTheDocument();
      });
      expect(screen.getByText("ETH")).toBeInTheDocument();
      // Check transaction counts are displayed
      expect(screen.getByText("12")).toBeInTheDocument();
      expect(screen.getByText("8")).toBeInTheDocument();
    });
  });

  describe("approximate values notice", () => {
    it("shows approximate values notice when has_approximate is true", async () => {
      mockGetPnL.mockResolvedValue(makeSnapshot({ has_approximate: true }));

      render(<PnLTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("Approximate Values")).toBeInTheDocument();
      });
      expect(
        screen.getByText(/hourly or daily candle prices/),
      ).toBeInTheDocument();
    });

    it("hides approximate values notice when has_approximate is false", async () => {
      mockGetPnL.mockResolvedValue(makeSnapshot({ has_approximate: false }));

      render(<PnLTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("BTC")).toBeInTheDocument();
      });
      expect(screen.queryByText("Approximate Values")).not.toBeInTheDocument();
    });
  });

  describe("excluded transactions notice", () => {
    it("shows excluded transactions notice when excluded_count > 0", async () => {
      mockGetPnL.mockResolvedValue(makeSnapshot({ excluded_count: 3 }));

      render(<PnLTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("Excluded Transactions")).toBeInTheDocument();
      });
      expect(
        screen.getByText(/3 transactions were excluded/),
      ).toBeInTheDocument();
    });

    it("hides excluded transactions notice when excluded_count is 0", async () => {
      mockGetPnL.mockResolvedValue(makeSnapshot({ excluded_count: 0 }));

      render(<PnLTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("BTC")).toBeInTheDocument();
      });
      expect(
        screen.queryByText("Excluded Transactions"),
      ).not.toBeInTheDocument();
    });
  });

  describe("loading and empty states", () => {
    it("shows loading spinner", () => {
      mockGetPnL.mockReturnValue(new Promise(() => {})); // never resolves

      render(<PnLTab quoteCurrency="USD" />);

      expect(screen.getByText("Loading P&L…")).toBeInTheDocument();
    });

    it("shows empty state when no snapshot", async () => {
      mockGetPnL.mockResolvedValue(null);

      render(<PnLTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText(/no transactions available/i)).toBeInTheDocument();
      });
    });
  });
});
