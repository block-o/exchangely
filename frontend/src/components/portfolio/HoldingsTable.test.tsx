import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { HoldingsTable } from "./HoldingsTable";
import type { AssetValuation, Holding } from "../../api/portfolio";

vi.mock("../../api/portfolio", async () => {
  const actual = await vi.importActual<typeof import("../../api/portfolio")>("../../api/portfolio");
  return { ...actual, deleteHolding: vi.fn(() => Promise.resolve()) };
});

import { deleteHolding } from "../../api/portfolio";
const mockDeleteHolding = vi.mocked(deleteHolding);

function makeAsset(overrides: Partial<AssetValuation> = {}): AssetValuation {
  return {
    asset_symbol: "BTC",
    quantity: 1,
    current_price: 50000,
    current_value: 50000,
    allocation_pct: 100,
    priced: true,
    source: "manual",
    ...overrides,
  };
}

function makeHolding(overrides: Partial<Holding> = {}): Holding {
  return {
    id: "h1",
    user_id: "u1",
    asset_symbol: "BTC",
    quantity: 1,
    quote_currency: "USD",
    source: "manual",
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

const noop = vi.fn();

describe("HoldingsTable", () => {
  it("returns null when assets array is empty", () => {
    const { container } = render(
      <HoldingsTable assets={[]} holdings={[]} quoteCurrency="USD" onDeleted={noop} />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("sorts assets by current_value descending", () => {
    const assets: AssetValuation[] = [
      makeAsset({ asset_symbol: "DOGE", current_value: 100 }),
      makeAsset({ asset_symbol: "BTC", current_value: 50000 }),
      makeAsset({ asset_symbol: "ETH", current_value: 5000 }),
    ];

    render(<HoldingsTable assets={assets} holdings={[]} quoteCurrency="USD" onDeleted={noop} />);

    const rows = screen.getAllByRole("row");
    const symbols = rows.slice(1).map((row) => {
      const nameEl = row.querySelector(".asset-name");
      return nameEl?.textContent;
    });

    expect(symbols).toEqual(["BTC", "ETH", "DOGE"]);
  });

  it("displays positive P&L with text-up class", () => {
    const assets = [
      makeAsset({ asset_symbol: "BTC", unrealized_pnl: 5000, unrealized_pnl_pct: 10 }),
    ];

    const { container } = render(
      <HoldingsTable assets={assets} holdings={[]} quoteCurrency="USD" onDeleted={noop} />,
    );

    const upElements = container.querySelectorAll(".text-up");
    expect(upElements.length).toBeGreaterThan(0);
  });

  it("displays negative P&L with text-down class", () => {
    const assets = [
      makeAsset({ asset_symbol: "ETH", unrealized_pnl: -200, unrealized_pnl_pct: -5 }),
    ];

    const { container } = render(
      <HoldingsTable assets={assets} holdings={[]} quoteCurrency="USD" onDeleted={noop} />,
    );

    const downElements = container.querySelectorAll(".text-down");
    expect(downElements.length).toBeGreaterThan(0);
  });

  it("displays dash for nil P&L with text-muted class", () => {
    const assets = [makeAsset({ asset_symbol: "SOL" })];

    const { container } = render(
      <HoldingsTable assets={assets} holdings={[]} quoteCurrency="USD" onDeleted={noop} />,
    );

    const mutedElements = container.querySelectorAll(".text-muted");
    expect(mutedElements.length).toBeGreaterThan(0);
    const dashEl = Array.from(mutedElements).find((el) => el.textContent === "—");
    expect(dashEl).toBeTruthy();
  });

  it("shows source label badges", () => {
    const assets = [
      makeAsset({ asset_symbol: "BTC", source: "binance" }),
      makeAsset({ asset_symbol: "ETH", source: "manual", current_value: 100 }),
    ];

    render(<HoldingsTable assets={assets} holdings={[]} quoteCurrency="USD" onDeleted={noop} />);

    expect(screen.getByText("Binance")).toBeInTheDocument();
    expect(screen.getByText("Manual")).toBeInTheDocument();
  });

  it("shows unpriced badge for unpriced assets", () => {
    const assets = [
      makeAsset({ asset_symbol: "OBSCURE", priced: false, current_price: 0, current_value: 0 }),
    ];

    render(<HoldingsTable assets={assets} holdings={[]} quoteCurrency="USD" onDeleted={noop} />);

    expect(screen.getByText("unpriced")).toBeInTheDocument();
  });

  it("does not show unpriced badge for priced assets", () => {
    const assets = [makeAsset({ asset_symbol: "BTC", priced: true })];

    render(<HoldingsTable assets={assets} holdings={[]} quoteCurrency="USD" onDeleted={noop} />);

    expect(screen.queryByText("unpriced")).not.toBeInTheDocument();
  });

  it("displays P&L percentage for holdings with pnl_pct", () => {
    const assets = [
      makeAsset({ asset_symbol: "BTC", unrealized_pnl: 1000, unrealized_pnl_pct: 12.34 }),
    ];

    render(<HoldingsTable assets={assets} holdings={[]} quoteCurrency="USD" onDeleted={noop} />);

    expect(screen.getByText("+12.34%")).toBeInTheDocument();
  });

  it("shows delete button when holding ID is available", () => {
    const assets = [makeAsset({ asset_symbol: "BTC", source: "manual" })];
    const holdings = [makeHolding({ id: "h1", asset_symbol: "BTC", source: "manual" })];

    render(<HoldingsTable assets={assets} holdings={holdings} quoteCurrency="USD" onDeleted={noop} />);

    expect(screen.getByLabelText("Remove BTC holding")).toBeInTheDocument();
  });

  it("calls deleteHolding and onDeleted when delete button is clicked", async () => {
    const onDeleted = vi.fn();
    mockDeleteHolding.mockResolvedValue(undefined);

    const assets = [makeAsset({ asset_symbol: "ETH", source: "manual" })];
    const holdings = [makeHolding({ id: "h2", asset_symbol: "ETH", source: "manual" })];

    render(<HoldingsTable assets={assets} holdings={holdings} quoteCurrency="USD" onDeleted={onDeleted} />);

    fireEvent.click(screen.getByLabelText("Remove ETH holding"));

    await waitFor(() => {
      expect(mockDeleteHolding).toHaveBeenCalledWith("h2");
      expect(onDeleted).toHaveBeenCalledOnce();
    });
  });
});
