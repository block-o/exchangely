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
      expect(screen.getByRole("dialog", { name: "Add Exchange" })).toBeInTheDocument();

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

  describe("exchange-specific labels", () => {
    it("shows 'API Key' and 'API Secret' without hints when Binance is selected (default)", () => {
      render(
        <ExchangeCredentialManager onSynced={noop} initialShowAdd onModalClose={vi.fn()} />,
      );

      expect(screen.getByLabelText("API Key")).toBeInTheDocument();
      expect(screen.getByLabelText("API Secret")).toBeInTheDocument();
      expect(screen.queryByText(/Found in Kraken/)).not.toBeInTheDocument();
      expect(screen.queryByText(/CDP Portal/)).not.toBeInTheDocument();
    });

    it("shows 'API Key' and 'Private Key' when Kraken is selected", () => {
      render(
        <ExchangeCredentialManager onSynced={noop} initialShowAdd onModalClose={vi.fn()} />,
      );

      fireEvent.click(screen.getByRole("tab", { name: "Kraken" }));

      expect(screen.getByLabelText("API Key")).toBeInTheDocument();
      expect(screen.getByLabelText("Private Key")).toBeInTheDocument();
      expect(screen.queryByLabelText("API Secret")).not.toBeInTheDocument();
      expect(screen.getByText(/Found in Kraken/)).toBeInTheDocument();
      expect(screen.getByText(/base64 string/)).toBeInTheDocument();
      // Kraken uses a single-line input, not a textarea
      const secretField = screen.getByLabelText("Private Key");
      expect(secretField.tagName).toBe("INPUT");
    });

    it("shows 'API Key Name' and 'Private Key' with hints when Coinbase is selected", () => {
      render(
        <ExchangeCredentialManager onSynced={noop} initialShowAdd onModalClose={vi.fn()} />,
      );

      fireEvent.click(screen.getByRole("tab", { name: "Coinbase" }));

      expect(screen.getByLabelText("API Key Name")).toBeInTheDocument();
      expect(screen.getByLabelText("Private Key")).toBeInTheDocument();
      expect(screen.queryByLabelText("API Key")).not.toBeInTheDocument();
      expect(screen.queryByLabelText("API Secret")).not.toBeInTheDocument();
      expect(screen.getByText(/CDP Portal/)).toBeInTheDocument();
      expect(screen.getByText(/PEM-encoded/)).toBeInTheDocument();
      // Private Key field should be a textarea for multiline PEM input
      const secretField = screen.getByLabelText("Private Key");
      expect(secretField.tagName).toBe("TEXTAREA");
    });

    it("updates labels and hints when switching between exchanges", () => {
      render(
        <ExchangeCredentialManager onSynced={noop} initialShowAdd onModalClose={vi.fn()} />,
      );

      // Default: Binance — no hints
      expect(screen.getByLabelText("API Key")).toBeInTheDocument();
      expect(screen.getByLabelText("API Secret")).toBeInTheDocument();
      expect(screen.queryByText(/Found in Kraken/)).not.toBeInTheDocument();

      // Switch to Coinbase — hints appear
      fireEvent.click(screen.getByRole("tab", { name: "Coinbase" }));
      expect(screen.getByLabelText("API Key Name")).toBeInTheDocument();
      expect(screen.getByLabelText("Private Key")).toBeInTheDocument();
      expect(screen.getByText(/CDP Portal/)).toBeInTheDocument();

      // Switch to Kraken — different hints
      fireEvent.click(screen.getByRole("tab", { name: "Kraken" }));
      expect(screen.getByLabelText("API Key")).toBeInTheDocument();
      expect(screen.getByLabelText("Private Key")).toBeInTheDocument();
      expect(screen.getByText(/Found in Kraken/)).toBeInTheDocument();
      expect(screen.queryByText(/CDP Portal/)).not.toBeInTheDocument();

      // Back to Binance — hints gone
      fireEvent.click(screen.getByRole("tab", { name: "Binance" }));
      expect(screen.getByLabelText("API Key")).toBeInTheDocument();
      expect(screen.getByLabelText("API Secret")).toBeInTheDocument();
      expect(screen.queryByText(/Found in Kraken/)).not.toBeInTheDocument();
      expect(screen.queryByText(/CDP Portal/)).not.toBeInTheDocument();
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
