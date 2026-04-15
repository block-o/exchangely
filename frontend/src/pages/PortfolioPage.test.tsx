import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PortfolioPage } from "./PortfolioPage";
import type { User } from "../types/auth";

// --- Mock useAuth ---
const defaultUser: User = {
  id: "u-1",
  email: "alice@example.com",
  name: "Alice",
  avatar_url: "",
  role: "user",
  has_google: false,
  has_password: true,
  must_change_password: false,
};

let mockAuth = {
  user: defaultUser as User | null,
  isAuthenticated: true,
  isLoading: false,
  authEnabled: true,
  authMethods: { google: false, local: true },
  login: vi.fn(),
  logout: vi.fn(),
  refreshToken: vi.fn(),
};

vi.mock("../app/auth", () => ({
  useAuth: () => mockAuth,
}));

let mockSettings = { quoteCurrency: "USD", setQuoteCurrency: vi.fn(), theme: "dark", setTheme: vi.fn() };

vi.mock("../app/settings", () => ({
  useSettings: () => mockSettings,
}));

// --- Mock portfolio API ---
const mockGetValuation = vi.fn();
const mockGetHoldings = vi.fn();
const mockSyncAll = vi.fn();
const mockTriggerRecompute = vi.fn();
const mockSubscribePortfolioStream = vi.fn();

vi.mock("../api/portfolio", () => ({
  getValuation: (...args: unknown[]) => mockGetValuation(...args),
  getHoldings: (...args: unknown[]) => mockGetHoldings(...args),
  syncAll: (...args: unknown[]) => mockSyncAll(...args),
  triggerRecompute: (...args: unknown[]) => mockTriggerRecompute(...args),
  subscribePortfolioStream: (...args: unknown[]) => mockSubscribePortfolioStream(...args),
  getCredentials: vi.fn(() => Promise.resolve([])),
  createCredential: vi.fn(() => Promise.resolve({})),
  deleteCredential: vi.fn(() => Promise.resolve()),
  syncCredential: vi.fn(() => Promise.resolve()),
  getWallets: vi.fn(() => Promise.resolve([])),
  createWallet: vi.fn(() => Promise.resolve({})),
  deleteWallet: vi.fn(() => Promise.resolve()),
  syncWallet: vi.fn(() => Promise.resolve()),
}));

vi.mock("../api/assets", () => ({
  fetchAssets: vi.fn(() => Promise.resolve({ data: [] })),
}));

vi.mock("../components/portfolio/HistoryChart", () => ({
  HistoryChart: () => <div data-testid="history-chart">HistoryChart</div>,
}));

// --- Helpers ---

const emptyValuation = { total_value: 0, quote_currency: "USD", assets: [], updated_at: "2026-01-01T00:00:00Z" };

const populatedValuation = {
  total_value: 50000,
  quote_currency: "USD",
  assets: [
    { asset_symbol: "BTC", quantity: 1, current_price: 50000, current_value: 50000, allocation_pct: 100, priced: true, source: "manual" },
  ],
  updated_at: "2026-01-01T00:00:00Z",
};

const sampleHoldings = [
  { id: "h-1", user_id: "u-1", asset_symbol: "BTC", quantity: 1, quote_currency: "USD", source: "manual", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];

function setupEmpty() {
  mockGetValuation.mockResolvedValue(emptyValuation);
  mockGetHoldings.mockResolvedValue([]);
}

function setupPopulated() {
  mockGetValuation.mockResolvedValue(populatedValuation);
  mockGetHoldings.mockResolvedValue(sampleHoldings);
}

describe("PortfolioPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuth = {
      user: defaultUser,
      isAuthenticated: true,
      isLoading: false,
      authEnabled: true,
      authMethods: { google: false, local: true },
      login: vi.fn(),
      logout: vi.fn(),
      refreshToken: vi.fn(),
    };
    mockSettings = { quoteCurrency: "USD", setQuoteCurrency: vi.fn(), theme: "dark", setTheme: vi.fn() };
    mockTriggerRecompute.mockResolvedValue(undefined);
    mockSubscribePortfolioStream.mockReturnValue({
      close: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      onopen: null,
      onerror: null,
    });
  });

  describe("PANEL_OPTIONS tabs", () => {
    it("renders all four tabs in the correct order", async () => {
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByRole("tablist")).toBeInTheDocument();
      });

      const tabs = screen.getAllByRole("tab");
      expect(tabs).toHaveLength(4);
      expect(tabs[0]).toHaveTextContent("Holdings");
      expect(tabs[1]).toHaveTextContent("P&L");
      expect(tabs[2]).toHaveTextContent("Transactions");
      expect(tabs[3]).toHaveTextContent("Sources");
    });

    it("starts with Holdings tab active", async () => {
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        const holdingsTab = screen.getByRole("tab", { name: "Holdings" });
        expect(holdingsTab).toHaveAttribute("aria-selected", "true");
      });
    });

    it("Sources tab is last (rightmost)", async () => {
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        const tabs = screen.getAllByRole("tab");
        expect(tabs[tabs.length - 1]).toHaveTextContent("Sources");
      });
    });
  });

  describe("tab switching", () => {
    it("shows EmptyState on Overview when portfolio is empty", async () => {
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByText("Your portfolio is empty")).toBeInTheDocument();
      });
    });

    it("shows valuation and holdings on Holdings tab when portfolio has data", async () => {
      setupPopulated();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByText("Total Portfolio Value")).toBeInTheDocument();
      });
      expect(screen.getAllByText("BTC").length).toBeGreaterThanOrEqual(1);
    });

    it("switches to Sources tab and shows all source panels", async () => {
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Sources" })).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole("tab", { name: "Sources" }));

      await waitFor(() => {
        expect(screen.getByText("Exchange Connections")).toBeInTheDocument();
      });
      expect(screen.getByText("Linked Wallets")).toBeInTheDocument();
      expect(screen.queryByText("Your portfolio is empty")).not.toBeInTheDocument();
    });

    it("EmptyState 'Add Holding' card switches to Sources tab", async () => {
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByText("Your portfolio is empty")).toBeInTheDocument();
      });

      const cards = screen.getAllByRole("button");
      const addCard = cards.find((b) => b.textContent?.includes("Add Holding"));
      expect(addCard).toBeTruthy();
      fireEvent.click(addCard!);

      await waitFor(() => {
        const sourcesTab = screen.getByRole("tab", { name: "Sources" });
        expect(sourcesTab).toHaveAttribute("aria-selected", "true");
      });
    });
  });

  describe("tab content isolation", () => {
    it("only one panel is visible at a time", async () => {
      setupPopulated();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByText("Total Portfolio Value")).toBeInTheDocument();
      });
      expect(screen.queryByText("Exchange Connections")).not.toBeInTheDocument();

      fireEvent.click(screen.getByRole("tab", { name: "Sources" }));
      await waitFor(() => {
        expect(screen.getByText("Exchange Connections")).toBeInTheDocument();
      });
      expect(screen.queryByText("Total Portfolio Value")).not.toBeInTheDocument();
    });
  });

  describe("unauthenticated redirect", () => {
    it("redirects to #login when not authenticated", async () => {
      mockAuth = { ...mockAuth, user: null, isAuthenticated: false, isLoading: false };
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(window.location.hash).toBe("#login");
      });
    });
  });

  describe("data fetching", () => {
    it("fetches valuation and holdings on mount", async () => {
      setupPopulated();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(mockGetValuation).toHaveBeenCalledWith("USD");
        expect(mockGetHoldings).toHaveBeenCalled();
      });
    });

    it("shows error alert when fetch fails", async () => {
      mockGetValuation.mockRejectedValue(new Error("Network error"));
      mockGetHoldings.mockResolvedValue([]);
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByRole("alert")).toHaveTextContent("Network error");
      });
    });
  });

  describe("currency resync notification", () => {
    it("shows warning banner and recalculating tab labels when currency changes", async () => {
      setupEmpty();
      const { rerender } = render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByRole("tablist")).toBeInTheDocument();
      });

      // No banner initially
      expect(screen.queryByText(/recalculated/)).not.toBeInTheDocument();

      // Simulate currency change
      mockSettings.quoteCurrency = "EUR";
      rerender(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByText(/recalculated in the new currency/)).toBeInTheDocument();
      });

      // Tab labels should show recalculating badge
      const tabs = screen.getAllByRole("tab");
      const txTab = tabs.find((t) => t.textContent?.includes("Transactions"));
      const pnlTab = tabs.find((t) => t.textContent?.includes("P&L"));
      expect(txTab).toHaveTextContent("Transactions (recalculating…)");
      expect(pnlTab).toHaveTextContent("P&L (recalculating…)");

      // triggerRecompute should have been called
      expect(mockTriggerRecompute).toHaveBeenCalled();
    });

    it("clears resync banner when SSE event fires", async () => {
      setupEmpty();

      // Capture the SSE event listeners so we can fire them
      const listeners: Record<string, (() => void)[]> = {};
      mockSubscribePortfolioStream.mockReturnValue({
        close: vi.fn(),
        addEventListener: vi.fn((event: string, handler: () => void) => {
          if (!listeners[event]) listeners[event] = [];
          listeners[event].push(handler);
        }),
        removeEventListener: vi.fn(),
      });

      const { rerender } = render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByRole("tablist")).toBeInTheDocument();
      });

      // Trigger currency change
      mockSettings.quoteCurrency = "EUR";
      rerender(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByText(/recalculated in the new currency/)).toBeInTheDocument();
      });

      // Simulate SSE completion event
      if (listeners["portfolio"]) {
        listeners["portfolio"].forEach((fn) => fn());
      }

      await waitFor(() => {
        expect(screen.queryByText(/recalculated in the new currency/)).not.toBeInTheDocument();
      });

      // Tab labels should be back to normal
      const tabs = screen.getAllByRole("tab");
      const txTab = tabs.find((t) => t.textContent?.includes("Transactions"));
      expect(txTab).toHaveTextContent("Transactions");
      expect(txTab?.textContent).not.toContain("recalculating");
    });

    it("displays stale data during resync rather than empty states", async () => {
      setupPopulated();
      const { rerender } = render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByText("Total Portfolio Value")).toBeInTheDocument();
      });

      // Trigger currency change — stale data should remain visible
      mockSettings.quoteCurrency = "EUR";
      rerender(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByText(/recalculated in the new currency/)).toBeInTheDocument();
      });

      // The existing portfolio data should still be visible
      expect(screen.getByText("Total Portfolio Value")).toBeInTheDocument();
    });
  });
});
