import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("../../api/portfolio", () => ({
  getCredentials: vi.fn(() => Promise.resolve([])),
  createCredential: vi.fn(() => Promise.resolve({})),
  deleteCredential: vi.fn(() => Promise.resolve()),
  syncCredential: vi.fn(() => Promise.resolve()),
}));

import { ExchangeCredentialManager } from "./ExchangeCredentialManager";

const noop = vi.fn();

describe("ExchangeCredentialManager", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("modal-only mode (initialShowAdd + onModalClose)", () => {
    it("renders only the modal, not the list panel", async () => {
      const onModalClose = vi.fn();
      render(
        <ExchangeCredentialManager onSynced={noop} initialShowAdd onModalClose={onModalClose} />,
      );

      // Modal should be visible
      expect(screen.getByRole("dialog", { name: "Add Exchange Credential" })).toBeInTheDocument();

      // List panel header should not be present
      expect(screen.queryByText("Exchange Connections")).not.toBeInTheDocument();
      expect(screen.queryByText("No exchange connections yet.")).not.toBeInTheDocument();
    });

    it("calls onModalClose when the close button is clicked", async () => {
      const onModalClose = vi.fn();
      render(
        <ExchangeCredentialManager onSynced={noop} initialShowAdd onModalClose={onModalClose} />,
      );

      fireEvent.click(screen.getByLabelText("Close"));

      await waitFor(() => {
        expect(onModalClose).toHaveBeenCalledOnce();
      });
    });
  });

  describe("normal mode (without initialShowAdd)", () => {
    it("renders the list panel with header", async () => {
      render(<ExchangeCredentialManager onSynced={noop} />);

      await waitFor(() => {
        expect(screen.getByText("Exchange Connections")).toBeInTheDocument();
      });
    });

    it("shows empty message when no credentials exist", async () => {
      render(<ExchangeCredentialManager onSynced={noop} />);

      await waitFor(() => {
        expect(screen.getByText("No exchange connections yet.")).toBeInTheDocument();
      });
    });

    it("does not show the modal by default", async () => {
      render(<ExchangeCredentialManager onSynced={noop} />);

      await waitFor(() => {
        expect(screen.getByText("Exchange Connections")).toBeInTheDocument();
      });

      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
  });
});
