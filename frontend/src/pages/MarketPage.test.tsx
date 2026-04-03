import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { MarketPage } from "./MarketPage";
import { SettingsProvider } from "../app/settings";
import * as pairsApi from "../api/pairs";
import * as systemApi from "../api/system";

vi.mock("../api/pairs");
vi.mock("../api/system");
vi.mock("../api/historical", () => ({ fetchHistorical: vi.fn().mockResolvedValue({ data: [] }) }));

describe("MarketPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  it("filters pairs by quote currency and displays base", async () => {
    vi.mocked(pairsApi.fetchPairs).mockResolvedValue({
      data: [
        { symbol: "BTCEUR", base: "BTC", quote: "EUR" },
        { symbol: "BTCUSDT", base: "BTC", quote: "USDT" },
        { symbol: "ETHEUR", base: "ETH", quote: "EUR" },
      ]
    });
    vi.mocked(systemApi.fetchTicker).mockResolvedValue({
      pair: "BTCEUR", price: 50000, variation_24h: 1.5, source: "mock", last_update_unix: Date.now() / 1000
    });

    render(
      <SettingsProvider>
        <MarketPage />
      </SettingsProvider>
    );

    // Assert EUR headers and base symbols output
    await waitFor(() => {
      expect(screen.getByText("Price (EUR)")).toBeInTheDocument();
      expect(screen.getByText("BTC")).toBeInTheDocument();
      expect(screen.getByText("ETH")).toBeInTheDocument();
    });

    // USDT pair should not be displayed
    expect(screen.queryByText("BTCUSDT")).not.toBeInTheDocument();
  });
});
