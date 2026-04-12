import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { EmptyState } from "./EmptyState";

describe("EmptyState", () => {
  it("renders the empty portfolio message", () => {
    render(
      <EmptyState
        onAddHolding={vi.fn()}
        onManageExchanges={vi.fn()}
        onManageWallets={vi.fn()}
      />,
    );

    expect(screen.getByText("Your portfolio is empty")).toBeInTheDocument();
  });

  it("renders three action cards: Add Holding, Connect Exchange, Link Wallet", () => {
    render(
      <EmptyState
        onAddHolding={vi.fn()}
        onManageExchanges={vi.fn()}
        onManageWallets={vi.fn()}
      />,
    );

    expect(screen.getByText("Add Holding")).toBeInTheDocument();
    expect(screen.getByText("Connect Exchange")).toBeInTheDocument();
    expect(screen.getByText("Link Wallet")).toBeInTheDocument();
  });

  it("calls onAddHolding when Add Holding card is clicked", () => {
    const onAddHolding = vi.fn();
    render(
      <EmptyState
        onAddHolding={onAddHolding}
        onManageExchanges={vi.fn()}
        onManageWallets={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByText("Add Holding"));
    expect(onAddHolding).toHaveBeenCalledOnce();
  });

  it("calls onManageExchanges when Connect Exchange card is clicked", () => {
    const onManageExchanges = vi.fn();
    render(
      <EmptyState
        onAddHolding={vi.fn()}
        onManageExchanges={onManageExchanges}
        onManageWallets={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByText("Connect Exchange"));
    expect(onManageExchanges).toHaveBeenCalledOnce();
  });

  it("calls onManageWallets when Link Wallet card is clicked", () => {
    const onManageWallets = vi.fn();
    render(
      <EmptyState
        onAddHolding={vi.fn()}
        onManageExchanges={vi.fn()}
        onManageWallets={onManageWallets}
      />,
    );

    fireEvent.click(screen.getByText("Link Wallet"));
    expect(onManageWallets).toHaveBeenCalledOnce();
  });

  it("shows hint text for each action card", () => {
    render(
      <EmptyState
        onAddHolding={vi.fn()}
        onManageExchanges={vi.fn()}
        onManageWallets={vi.fn()}
      />,
    );

    expect(screen.getByText("Enter positions manually")).toBeInTheDocument();
    expect(screen.getByText("Sync from Binance, Kraken, Coinbase")).toBeInTheDocument();
    expect(screen.getByText("Track ETH, SOL, BTC on-chain")).toBeInTheDocument();
  });
});
