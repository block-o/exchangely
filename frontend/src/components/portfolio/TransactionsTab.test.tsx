import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TransactionsTab } from "./TransactionsTab";
import type { Transaction, TransactionListResponse } from "../../api/portfolio";

// Mock the portfolio API module
const mockGetTransactions = vi.fn();
const mockUpdateTransaction = vi.fn();

vi.mock("../../api/portfolio", () => ({
  getTransactions: (...args: unknown[]) => mockGetTransactions(...args),
  updateTransaction: (...args: unknown[]) => mockUpdateTransaction(...args),
  getCredentials: vi.fn(() => Promise.resolve([])),
  getWallets: vi.fn(() => Promise.resolve([])),
}));

function makeTx(overrides: Partial<Transaction> = {}): Transaction {
  return {
    id: "tx-1",
    user_id: "u-1",
    asset_symbol: "BTC",
    quantity: 0.5,
    type: "buy",
    timestamp: "2026-03-15T10:30:00Z",
    source: "binance",
    source_ref: "cred-1",
    reference_value: 25000,
    reference_currency: "USD",
    resolution: "exact",
    manually_edited: false,
    notes: "",
    fee: null,
    fee_currency: "",
    created_at: "2026-03-15T10:30:00Z",
    updated_at: "2026-03-15T10:30:00Z",
    ...overrides,
  };
}

function makeResponse(txs: Transaction[], total?: number): TransactionListResponse {
  return {
    data: txs,
    total: total ?? txs.length,
    page: 1,
    page_size: 20,
  };
}

describe("TransactionsTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("rendering transaction rows", () => {
    it("renders transaction rows with correct data", async () => {
      const txs = [
        makeTx({ id: "tx-1", asset_symbol: "BTC", quantity: 0.5, type: "buy", source: "binance" }),
        makeTx({ id: "tx-2", asset_symbol: "ETH", quantity: 10, type: "sell", source: "kraken" }),
      ];
      mockGetTransactions.mockResolvedValue(makeResponse(txs));

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("BTC")).toBeInTheDocument();
      });
      expect(screen.getByText("ETH")).toBeInTheDocument();
      expect(screen.getByText("Buy")).toBeInTheDocument();
      expect(screen.getByText("Sell")).toBeInTheDocument();
      expect(screen.getByText("Binance")).toBeInTheDocument();
      expect(screen.getByText("Kraken")).toBeInTheDocument();
    });

    it("shows total transaction count", async () => {
      mockGetTransactions.mockResolvedValue(makeResponse([makeTx()], 5));

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("5 transactions")).toBeInTheDocument();
      });
    });
  });

  describe("resolution badges", () => {
    it("shows ~1h badge for hourly resolution", async () => {
      mockGetTransactions.mockResolvedValue(
        makeResponse([makeTx({ resolution: "hourly" })]),
      );

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("~1h")).toBeInTheDocument();
      });
      expect(screen.getByTitle("Price resolved from the nearest hourly candle")).toBeInTheDocument();
    });

    it("shows ~1d badge for daily resolution", async () => {
      mockGetTransactions.mockResolvedValue(
        makeResponse([makeTx({ resolution: "daily" })]),
      );

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("~1d")).toBeInTheDocument();
      });
      expect(screen.getByTitle("Price resolved from the nearest daily candle (less precise)")).toBeInTheDocument();
    });

    it("shows warning icon for unresolvable resolution", async () => {
      mockGetTransactions.mockResolvedValue(
        makeResponse([makeTx({ resolution: "unresolvable", reference_value: null })]),
      );

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("⚠️")).toBeInTheDocument();
      });
      // The tooltip includes the asset and currency context.
      const icon = screen.getByText("⚠️");
      expect(icon.getAttribute("title")).toContain("No BTC/USD price data");
    });

    it("shows edit badge for manually_edited transactions", async () => {
      mockGetTransactions.mockResolvedValue(
        makeResponse([makeTx({ manually_edited: true })]),
      );

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByTitle("Manually edited")).toBeInTheDocument();
      });
    });
  });

  describe("edit flow", () => {
    it("opens edit modal, submits changes, and refreshes data", async () => {
      const tx = makeTx({ id: "tx-edit", asset_symbol: "BTC", reference_value: 25000 });
      mockGetTransactions.mockResolvedValue(makeResponse([tx]));
      mockUpdateTransaction.mockResolvedValue(undefined);

      render(<TransactionsTab quoteCurrency="USD" />);

      // Wait for the table to render
      await waitFor(() => {
        expect(screen.getByText("BTC")).toBeInTheDocument();
      });

      // Click the edit button
      fireEvent.click(screen.getByRole("button", { name: /edit btc transaction/i }));

      // Modal should appear
      await waitFor(() => {
        expect(screen.getByRole("dialog")).toBeInTheDocument();
      });

      // Fill in the value field
      const valueInput = screen.getByLabelText("Value (USD)");
      fireEvent.change(valueInput, { target: { value: "30000" } });

      // Fill in the notes field
      const notesInput = screen.getByLabelText("Notes");
      fireEvent.change(notesInput, { target: { value: "Corrected value" } });

      // Submit the form
      fireEvent.click(screen.getByRole("button", { name: "Save Changes" }));

      // Verify updateTransaction was called with correct args
      await waitFor(() => {
        expect(mockUpdateTransaction).toHaveBeenCalledWith("tx-edit", {
          reference_value: 30000,
          notes: "Corrected value",
        });
      });

      // After successful update, getTransactions should be called again to refresh
      expect(mockGetTransactions.mock.calls.length).toBeGreaterThanOrEqual(2);
    });
  });

  describe("loading and empty states", () => {
    it("shows spinner while loading", () => {
      mockGetTransactions.mockReturnValue(new Promise(() => {})); // never resolves

      render(<TransactionsTab quoteCurrency="USD" />);

      expect(screen.getByText("Loading transactions…")).toBeInTheDocument();
    });

    it("shows empty message when no transactions exist", async () => {
      mockGetTransactions.mockResolvedValue(makeResponse([]));

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText(/connect an exchange/i)).toBeInTheDocument();
      });
    });

    it("shows error message when fetch fails", async () => {
      mockGetTransactions.mockRejectedValue(new Error("Network error"));

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("Network error")).toBeInTheDocument();
      });
    });
  });

  describe("fee column", () => {
    it("renders Fee column header", async () => {
      mockGetTransactions.mockResolvedValue(makeResponse([makeTx()]));

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("Fee")).toBeInTheDocument();
      });
    });

    it("shows formatted fee when present", async () => {
      const tx = makeTx({ id: "tx-fee", fee: 1.25, fee_currency: "EUR" });
      mockGetTransactions.mockResolvedValue(makeResponse([tx]));

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("BTC")).toBeInTheDocument();
      });
      // The fee should be formatted as a currency value
      const cells = document.querySelectorAll("td");
      const feeCell = Array.from(cells).find((c) => c.textContent?.includes("1.25"));
      expect(feeCell).toBeTruthy();
    });

    it("shows dash when fee is null", async () => {
      const tx = makeTx({ id: "tx-nofee", fee: null, fee_currency: "" });
      mockGetTransactions.mockResolvedValue(makeResponse([tx]));

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("BTC")).toBeInTheDocument();
      });
      // There should be dash characters for the fee column
      const cells = document.querySelectorAll("td");
      const dashCells = Array.from(cells).filter((c) => c.textContent === "—");
      expect(dashCells.length).toBeGreaterThanOrEqual(1);
    });

    it("shows dash when fee is zero", async () => {
      const tx = makeTx({ id: "tx-zerofee", fee: 0, fee_currency: "USD" });
      mockGetTransactions.mockResolvedValue(makeResponse([tx]));

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("BTC")).toBeInTheDocument();
      });
      const cells = document.querySelectorAll("td");
      const dashCells = Array.from(cells).filter((c) => c.textContent === "—");
      expect(dashCells.length).toBeGreaterThanOrEqual(1);
    });

    it("uses quoteCurrency as fallback when fee_currency is empty", async () => {
      const tx = makeTx({ id: "tx-fallback", fee: 5.0, fee_currency: "" });
      mockGetTransactions.mockResolvedValue(makeResponse([tx]));

      render(<TransactionsTab quoteCurrency="EUR" />);

      await waitFor(() => {
        expect(screen.getByText("BTC")).toBeInTheDocument();
      });
      const cells = document.querySelectorAll("td");
      const feeCell = Array.from(cells).find((c) => c.textContent?.includes("5.00"));
      expect(feeCell).toBeTruthy();
    });
  });

  describe("pagination", () => {
    it("shows pagination controls when total exceeds page size", async () => {
      mockGetTransactions.mockResolvedValue({
        data: [makeTx()],
        total: 40,
        page: 1,
        page_size: 20,
      });

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("Page 1 of 2")).toBeInTheDocument();
      });
      expect(screen.getByRole("button", { name: /prev/i })).toBeDisabled();
      expect(screen.getByRole("button", { name: /next/i })).not.toBeDisabled();
    });

    it("navigates to next page on click", async () => {
      mockGetTransactions.mockResolvedValue({
        data: [makeTx()],
        total: 40,
        page: 1,
        page_size: 20,
      });

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("Page 1 of 2")).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      await waitFor(() => {
        expect(mockGetTransactions).toHaveBeenCalledWith({ page: 2, page_size: 20 });
      });
    });

    it("does not show pagination when all items fit on one page", async () => {
      mockGetTransactions.mockResolvedValue(makeResponse([makeTx()], 1));

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("BTC")).toBeInTheDocument();
      });
      expect(screen.queryByText(/Page/)).not.toBeInTheDocument();
    });
  });

  describe("transaction type badges", () => {
    it("renders fee type badge with default variant", async () => {
      const tx = makeTx({ id: "tx-fee-type", type: "fee", asset_symbol: "BTC" });
      mockGetTransactions.mockResolvedValue(makeResponse([tx]));

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("BTC")).toBeInTheDocument();
      });
      // Both the column header and the badge contain "Fee"; verify the badge exists
      const badges = screen.getAllByText("Fee");
      expect(badges.length).toBe(2); // header + badge
      const badge = badges.find((el) => el.classList.contains("ui-badge"));
      expect(badge).toBeTruthy();
    });

    it("renders transfer type badge with default variant", async () => {
      const tx = makeTx({ id: "tx-transfer", type: "transfer", asset_symbol: "ETH" });
      mockGetTransactions.mockResolvedValue(makeResponse([tx]));

      render(<TransactionsTab quoteCurrency="USD" />);

      await waitFor(() => {
        expect(screen.getByText("Transfer")).toBeInTheDocument();
      });
    });
  });

  describe("refreshKey prop", () => {
    it("re-fetches transactions when refreshKey changes", async () => {
      mockGetTransactions.mockResolvedValue(makeResponse([makeTx()]));

      const { rerender } = render(<TransactionsTab quoteCurrency="USD" refreshKey={1} />);

      await waitFor(() => {
        expect(mockGetTransactions).toHaveBeenCalledTimes(1);
      });

      rerender(<TransactionsTab quoteCurrency="USD" refreshKey={2} />);

      await waitFor(() => {
        expect(mockGetTransactions).toHaveBeenCalledTimes(2);
      });
    });
  });
});
