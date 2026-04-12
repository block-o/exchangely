import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import * as client from "./client";

// Re-import after mocking
vi.mock("./client", async () => {
  const actual = await vi.importActual<typeof import("./client")>("./client");
  return {
    ...actual,
    API_BASE_URL: "http://localhost:8080/api/v1",
    authGet: vi.fn(),
    authPost: vi.fn(),
    authFetch: vi.fn(),
    authEventSource: vi.fn(),
  };
});

import {
  getHoldings,
  createHolding,
  updateHolding,
  deleteHolding,
  getValuation,
  getHistory,
  getCredentials,
  createCredential,
  deleteCredential,
  syncCredential,
  getWallets,
  createWallet,
  deleteWallet,
  syncWallet,
  uploadLedgerExport,
  disconnectLedger,
  subscribePortfolioStream,
} from "./portfolio";

const mockAuthGet = vi.mocked(client.authGet);
const mockAuthPost = vi.mocked(client.authPost);
const mockAuthFetch = vi.mocked(client.authFetch);
const mockAuthEventSource = vi.mocked(client.authEventSource);

beforeEach(() => {
  vi.clearAllMocks();
});

describe("portfolio API client", () => {
  describe("getHoldings", () => {
    it("calls authGet and unwraps data array", async () => {
      const holdings = [{ id: "h1", asset_symbol: "BTC", quantity: 1.5 }];
      mockAuthGet.mockResolvedValue({ data: holdings });

      const result = await getHoldings();

      expect(mockAuthGet).toHaveBeenCalledWith("/portfolio/holdings");
      expect(result).toEqual(holdings);
    });
  });

  describe("createHolding", () => {
    it("calls authPost with the request body", async () => {
      const req = { asset_symbol: "ETH", quantity: 10 };
      const created = { id: "h2", ...req };
      mockAuthPost.mockResolvedValue(created);

      const result = await createHolding(req);

      expect(mockAuthPost).toHaveBeenCalledWith("/portfolio/holdings", req);
      expect(result).toEqual(created);
    });
  });

  describe("updateHolding", () => {
    it("calls authFetch with PUT method and correct URL", async () => {
      const req = { asset_symbol: "ETH", quantity: 20 };
      const updated = { id: "h2", ...req };
      mockAuthFetch.mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(updated),
      } as Response);

      const result = await updateHolding("h2", req);

      expect(mockAuthFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/portfolio/holdings/h2",
        expect.objectContaining({
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          credentials: "include",
          body: JSON.stringify(req),
        }),
      );
      expect(result).toEqual(updated);
    });

    it("throws on non-ok response", async () => {
      mockAuthFetch.mockResolvedValue({ ok: false, status: 404 } as Response);

      await expect(updateHolding("h2", { asset_symbol: "ETH", quantity: 1 }))
        .rejects.toThrow("request failed: 404");
    });
  });

  describe("deleteHolding", () => {
    it("calls authFetch with DELETE method", async () => {
      mockAuthFetch.mockResolvedValue({ ok: true, status: 200 } as Response);

      await deleteHolding("h3");

      expect(mockAuthFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/portfolio/holdings/h3",
        expect.objectContaining({ method: "DELETE", credentials: "include" }),
      );
    });

    it("accepts 204 as success", async () => {
      mockAuthFetch.mockResolvedValue({ ok: false, status: 204 } as Response);

      await expect(deleteHolding("h3")).resolves.toBeUndefined();
    });

    it("throws on error status", async () => {
      mockAuthFetch.mockResolvedValue({ ok: false, status: 500 } as Response);

      await expect(deleteHolding("h3")).rejects.toThrow("request failed: 500");
    });
  });

  describe("getValuation", () => {
    it("calls authGet with quote param", async () => {
      const val = { total_value: 50000, quote_currency: "EUR", assets: [], updated_at: "" };
      mockAuthGet.mockResolvedValue(val);

      const result = await getValuation("EUR");

      expect(mockAuthGet).toHaveBeenCalledWith("/portfolio/valuation?quote=EUR");
      expect(result).toEqual(val);
    });

    it("calls authGet without params when quote is omitted", async () => {
      mockAuthGet.mockResolvedValue({ total_value: 0, quote_currency: "USD", assets: [], updated_at: "" });

      await getValuation();

      expect(mockAuthGet).toHaveBeenCalledWith("/portfolio/valuation");
    });
  });

  describe("getHistory", () => {
    it("passes range and quote as query params", async () => {
      mockAuthGet.mockResolvedValue({ data: [] });

      await getHistory("7d", "USD");

      expect(mockAuthGet).toHaveBeenCalledWith("/portfolio/history?range=7d&quote=USD");
    });

    it("works with no params", async () => {
      mockAuthGet.mockResolvedValue({ data: [] });

      await getHistory();

      expect(mockAuthGet).toHaveBeenCalledWith("/portfolio/history");
    });
  });

  describe("getCredentials", () => {
    it("calls authGet and unwraps data", async () => {
      const creds = [{ id: "c1", exchange: "binance" }];
      mockAuthGet.mockResolvedValue({ data: creds });

      const result = await getCredentials();

      expect(mockAuthGet).toHaveBeenCalledWith("/portfolio/credentials");
      expect(result).toEqual(creds);
    });
  });

  describe("createCredential", () => {
    it("calls authPost with credential request", async () => {
      const req = { exchange: "binance", api_key: "key", api_secret: "secret" };
      mockAuthPost.mockResolvedValue({ id: "c2", exchange: "binance" });

      await createCredential(req);

      expect(mockAuthPost).toHaveBeenCalledWith("/portfolio/credentials", req);
    });
  });

  describe("deleteCredential", () => {
    it("calls authFetch with DELETE", async () => {
      mockAuthFetch.mockResolvedValue({ ok: true, status: 200 } as Response);

      await deleteCredential("c1");

      expect(mockAuthFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/portfolio/credentials/c1",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
  });

  describe("syncCredential", () => {
    it("calls authPost for sync endpoint", async () => {
      mockAuthPost.mockResolvedValue(undefined);

      await syncCredential("c1");

      expect(mockAuthPost).toHaveBeenCalledWith("/portfolio/credentials/c1/sync");
    });
  });

  describe("getWallets", () => {
    it("calls authGet and unwraps data", async () => {
      const wallets = [{ id: "w1", chain: "ethereum" }];
      mockAuthGet.mockResolvedValue({ data: wallets });

      const result = await getWallets();

      expect(mockAuthGet).toHaveBeenCalledWith("/portfolio/wallets");
      expect(result).toEqual(wallets);
    });
  });

  describe("createWallet", () => {
    it("calls authPost with wallet request", async () => {
      const req = { chain: "ethereum", address: "0xabc" };
      mockAuthPost.mockResolvedValue({ id: "w2" });

      await createWallet(req);

      expect(mockAuthPost).toHaveBeenCalledWith("/portfolio/wallets", req);
    });
  });

  describe("deleteWallet", () => {
    it("calls authFetch with DELETE", async () => {
      mockAuthFetch.mockResolvedValue({ ok: true, status: 200 } as Response);

      await deleteWallet("w1");

      expect(mockAuthFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/portfolio/wallets/w1",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
  });

  describe("syncWallet", () => {
    it("calls authPost for wallet sync", async () => {
      mockAuthPost.mockResolvedValue(undefined);

      await syncWallet("w1");

      expect(mockAuthPost).toHaveBeenCalledWith("/portfolio/wallets/w1/sync");
    });
  });

  describe("uploadLedgerExport", () => {
    it("uploads file via multipart form", async () => {
      mockAuthFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ imported: 3 }),
      } as unknown as Response);

      const file = new File(['{"accounts":[]}'], "export.json", { type: "application/json" });
      const result = await uploadLedgerExport(file);

      expect(result.imported).toBe(3);
      expect(mockAuthFetch).toHaveBeenCalledTimes(1);
    });
  });

  describe("disconnectLedger", () => {
    it("calls authFetch with DELETE", async () => {
      mockAuthFetch.mockResolvedValue({ ok: true, status: 200 } as Response);

      await disconnectLedger();

      expect(mockAuthFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/portfolio/ledger",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
  });

  describe("subscribePortfolioStream", () => {
    it("calls authEventSource with the stream URL", () => {
      const fakeEs = {} as EventSource;
      mockAuthEventSource.mockReturnValue(fakeEs);

      const result = subscribePortfolioStream();

      expect(mockAuthEventSource).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/portfolio/stream",
      );
      expect(result).toBe(fakeEs);
    });
  });
});
