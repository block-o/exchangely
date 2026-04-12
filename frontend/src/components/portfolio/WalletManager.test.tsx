import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("../../api/portfolio", () => ({
  getWallets: vi.fn(() => Promise.resolve([])),
  createWallet: vi.fn(() => Promise.resolve({})),
  deleteWallet: vi.fn(() => Promise.resolve()),
  syncWallet: vi.fn(() => Promise.resolve()),
}));

import { WalletManager } from "./WalletManager";

const noop = vi.fn();

describe("WalletManager", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("modal-only mode (initialShowAdd + onModalClose)", () => {
    it("renders only the modal, not the list panel", async () => {
      const onModalClose = vi.fn();
      render(
        <WalletManager onSynced={noop} initialShowAdd onModalClose={onModalClose} />,
      );

      // Modal should be visible
      expect(screen.getByRole("dialog", { name: "Link Wallet" })).toBeInTheDocument();

      // List panel header should not be present
      expect(screen.queryByText("Linked Wallets")).not.toBeInTheDocument();
      expect(screen.queryByText("No wallets linked yet.")).not.toBeInTheDocument();
    });

    it("calls onModalClose when the close button is clicked", async () => {
      const onModalClose = vi.fn();
      render(
        <WalletManager onSynced={noop} initialShowAdd onModalClose={onModalClose} />,
      );

      fireEvent.click(screen.getByLabelText("Close"));

      await waitFor(() => {
        expect(onModalClose).toHaveBeenCalledOnce();
      });
    });
  });

  describe("normal mode (without initialShowAdd)", () => {
    it("renders the list panel with header", async () => {
      render(<WalletManager onSynced={noop} />);

      await waitFor(() => {
        expect(screen.getByText("Linked Wallets")).toBeInTheDocument();
      });
    });

    it("shows empty message when no wallets exist", async () => {
      render(<WalletManager onSynced={noop} />);

      await waitFor(() => {
        expect(screen.getByText("No wallets linked yet.")).toBeInTheDocument();
      });
    });

    it("does not show the modal by default", async () => {
      render(<WalletManager onSynced={noop} />);

      await waitFor(() => {
        expect(screen.getByText("Linked Wallets")).toBeInTheDocument();
      });

      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
  });
});
