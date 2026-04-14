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

vi.mock("../app/settings", () => ({
  useSettings: () => ({ quoteCurrency: "USD", setQuoteCurrency: vi.fn(), theme: "dark", setTheme: vi.fn() }),
}));

// --- Mock portfolio API ---
const mockGetValuation = vi.fn();
const mockGetHoldings = vi.fn();
const mockSyncAll = vi.fn();

vi.mock("../api/portfolio", () => ({
  getValuation: (...args: unknown[]) => mockGetValuation(...args),
  getHoldings: (...args: unknown[]) => mockGetHoldings(...args),
  syncAll: (...args: unknown[]) => mockSyncAll(...args),
  getCredentials: vi.fn(() => Promise.resolve([])),
  createCredential: vi.fn(() => Promise.resolve({})),
  deleteCredential: vi.fn(() => Promise.resolve()),
  syncCredential: vi.fn(() => Promise.resolve()),
  getWallets: vi.fn(() => Promise.resolve([])),
  createWallet: vi.fn(() => Promise.resolve({})),
  deleteWallet: vi.fn(() => Promise.resolve()),
  syncWallet: vi.fn(() => Promise.resolve()),
  subscribePortfolioStream: vi.fn(() => ({
    close: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    onopen: null,
    onerror: null,
  })),
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
  });

  describe("PANEL_OPTIONS tabs", () => {
    it("renders all five tabs in the correct order", async () => {
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByRole("tablist")).toBeInTheDocument();
      });

      const tabs = screen.getAllByRole("tab");
      expect(tabs).toHaveLength(5);
      expect(tabs[0]).toHaveTextContent("Overview");
      expect(tabs[1]).toHaveTextContent("Exchanges");
      expect(tabs[2]).toHaveTextContent("Wallets");
      expect(tabs[3]).toHaveTextContent("Ledger");
      expect(tabs[4]).toHaveTextContent("Others");
    });

    it("starts with Overview tab active", async () => {
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        const overviewTab = screen.getByRole("tab", { name: "Overview" });
        expect(overviewTab).toHaveAttribute("aria-selected", "true");
      });
    });

    it("Others tab is last (rightmost)", async () => {
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        const tabs = screen.getAllByRole("tab");
        expect(tabs[tabs.length - 1]).toHaveTextContent("Others");
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

    it("shows valuation and holdings on Overview when portfolio has data", async () => {
      setupPopulated();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByText("Total Portfolio Value")).toBeInTheDocument();
      });
      // BTC appears in both allocation chart and holdings table
      expect(screen.getAllByText("BTC").length).toBeGreaterThanOrEqual(1);
    });

    it("switches to Exchanges tab and shows exchange panel", async () => {
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Exchanges" })).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole("tab", { name: "Exchanges" }));

      await waitFor(() => {
        expect(screen.getByText("Exchange Connections")).toBeInTheDocument();
      });
      expect(screen.queryByText("Your portfolio is empty")).not.toBeInTheDocument();
    });

    it("switches to Wallets tab and shows wallet panel", async () => {
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Wallets" })).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole("tab", { name: "Wallets" }));

      await waitFor(() => {
        expect(screen.getByText("Linked Wallets")).toBeInTheDocument();
      });
    });

    it("switches to Others tab and shows manual holding form", async () => {
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Others" })).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole("tab", { name: "Others" }));

      await waitFor(() => {
        expect(screen.getByLabelText("Asset")).toBeInTheDocument();
      });
      expect(screen.getByLabelText("Quantity")).toBeInTheDocument();
      expect(screen.getByText("Track a position that isn't linked to an exchange or wallet.")).toBeInTheDocument();
    });

    it("EmptyState 'Add Holding' card switches to Others tab", async () => {
      setupEmpty();
      render(<PortfolioPage />);

      await waitFor(() => {
        expect(screen.getByText("Your portfolio is empty")).toBeInTheDocument();
      });

      // Click the "Add Holding" card in EmptyState
      const cards = screen.getAllByRole("button");
      const addCard = cards.find((b) => b.textContent?.includes("Add Holding"));
      expect(addCard).toBeTruthy();
      fireEvent.click(addCard!);

      await waitFor(() => {
        const othersTab = screen.getByRole("tab", { name: "Others" });
        expect(othersTab).toHaveAttribute("aria-selected", "true");
      });
      expect(screen.getByLabelText("Asset")).toBeInTheDocument();
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
      expect(screen.queryByText("Linked Wallets")).not.toBeInTheDocument();
      expect(screen.queryByLabelText("Asset")).not.toBeInTheDocument();

      fireEvent.click(screen.getByRole("tab", { name: "Exchanges" }));
      await waitFor(() => {
        expect(screen.getByText("Exchange Connections")).toBeInTheDocument();
      });
      expect(screen.queryByText("Total Portfolio Value")).not.toBeInTheDocument();
      expect(screen.queryByText("Linked Wallets")).not.toBeInTheDocument();
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
});
