package wallet

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// --- Interface compliance ---

func TestWalletConnectorInterfaceCompliance(t *testing.T) {
	var _ WalletConnector = (*EthereumConnector)(nil)
	var _ WalletConnector = (*SolanaConnector)(nil)
	var _ WalletConnector = (*BitcoinConnector)(nil)
}

// --- Chain name tests ---

func TestChainNames(t *testing.T) {
	eth := NewEthereumConnector("", "", nil)
	if eth.Chain() != "ethereum" {
		t.Errorf("expected ethereum, got %s", eth.Chain())
	}
	sol := NewSolanaConnector("", nil)
	if sol.Chain() != "solana" {
		t.Errorf("expected solana, got %s", sol.Chain())
	}
	btc := NewBitcoinConnector("", nil)
	if btc.Chain() != "bitcoin" {
		t.Errorf("expected bitcoin, got %s", btc.Chain())
	}
}

// --- Default URL tests ---

func TestDefaultURLs(t *testing.T) {
	eth := NewEthereumConnector("", "key", nil)
	if eth.baseURL != "https://api.etherscan.io" {
		t.Errorf("expected default etherscan URL, got %s", eth.baseURL)
	}
	sol := NewSolanaConnector("", nil)
	if sol.rpcURL != "https://api.mainnet-beta.solana.com" {
		t.Errorf("expected default solana RPC URL, got %s", sol.rpcURL)
	}
	btc := NewBitcoinConnector("", nil)
	if btc.baseURL != "https://blockstream.info/api" {
		t.Errorf("expected default blockstream URL, got %s", btc.baseURL)
	}
}

// --- Ethereum connector tests ---

func TestEthereumETHBalanceParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		if action == "balance" {
			// 1.5 ETH in wei = 1500000000000000000
			_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":"1500000000000000000"}`))
			return
		}
		// ERC-20 token tx endpoint: no transactions
		_, _ = w.Write([]byte(`{"status":"0","message":"No transactions found","result":[]}`))
	}))
	defer server.Close()

	c := NewEthereumConnector(server.URL, "test-key", server.Client())
	balances, err := c.FetchBalances(context.Background(), "0x1234567890abcdef1234567890abcdef12345678")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected 1 balance, got %d: %+v", len(balances), balances)
	}
	if balances[0].Asset != "ETH" {
		t.Errorf("expected ETH, got %s", balances[0].Asset)
	}
	if math.Abs(balances[0].Quantity-1.5) > 1e-10 {
		t.Errorf("expected 1.5 ETH, got %f", balances[0].Quantity)
	}
}

func TestEthereumERC20TokenAggregation(t *testing.T) {
	addr := "0xaabbccddee1234567890aabbccddee1234567890"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		if action == "balance" {
			// 2 ETH in wei
			_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":"2000000000000000000"}`))
			return
		}
		// ERC-20 token transfers: receive 100 USDC, send 30 USDC, receive 50 LINK
		_, _ = w.Write([]byte(`{
			"status": "1",
			"message": "OK",
			"result": [
				{
					"tokenSymbol": "USDC",
					"tokenDecimal": "6",
					"to": "` + strings.ToLower(addr) + `",
					"from": "0x0000000000000000000000000000000000000001",
					"value": "100000000"
				},
				{
					"tokenSymbol": "USDC",
					"tokenDecimal": "6",
					"to": "0x0000000000000000000000000000000000000002",
					"from": "` + strings.ToLower(addr) + `",
					"value": "30000000"
				},
				{
					"tokenSymbol": "LINK",
					"tokenDecimal": "18",
					"to": "` + strings.ToLower(addr) + `",
					"from": "0x0000000000000000000000000000000000000003",
					"value": "50000000000000000000"
				}
			]
		}`))
	}))
	defer server.Close()

	c := NewEthereumConnector(server.URL, "test-key", server.Client())
	balances, err := c.FetchBalances(context.Background(), addr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byAsset := make(map[string]float64)
	for _, b := range balances {
		byAsset[b.Asset] = b.Quantity
	}

	if math.Abs(byAsset["ETH"]-2.0) > 1e-10 {
		t.Errorf("expected ETH=2.0, got %f", byAsset["ETH"])
	}
	// 100 - 30 = 70 USDC
	if math.Abs(byAsset["USDC"]-70.0) > 1e-4 {
		t.Errorf("expected USDC=70.0, got %f", byAsset["USDC"])
	}
	if math.Abs(byAsset["LINK"]-50.0) > 1e-10 {
		t.Errorf("expected LINK=50.0, got %f", byAsset["LINK"])
	}
}

func TestEthereumZeroBalanceFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		if action == "balance" {
			// 0 ETH
			_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":"0"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"0","message":"No transactions found","result":[]}`))
	}))
	defer server.Close()

	c := NewEthereumConnector(server.URL, "test-key", server.Client())
	balances, err := c.FetchBalances(context.Background(), "0x1234567890abcdef1234567890abcdef12345678")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 0 {
		t.Errorf("expected 0 balances for zero ETH, got %d: %+v", len(balances), balances)
	}
}

func TestEthereumRetryOnServerError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewEthereumConnector(server.URL, "test-key", server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := c.FetchBalances(ctx, "0x1234567890abcdef1234567890abcdef12345678")
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if !strings.Contains(err.Error(), "max retries exceeded") {
		t.Errorf("expected max retries error, got: %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestEthereumNoRetryOnClientError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	c := NewEthereumConnector(server.URL, "test-key", server.Client())
	_, err := c.FetchBalances(context.Background(), "0x1234567890abcdef1234567890abcdef12345678")
	if err == nil {
		t.Fatal("expected error on 403")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Errorf("expected client error, got: %v", err)
	}
	if attempts.Load() != 1 {
		t.Errorf("expected exactly 1 attempt (no retry on 4xx), got %d", attempts.Load())
	}
}

func TestEthereumEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		if action == "balance" {
			_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":"0"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":[]}`))
	}))
	defer server.Close()

	c := NewEthereumConnector(server.URL, "test-key", server.Client())
	balances, err := c.FetchBalances(context.Background(), "0x1234567890abcdef1234567890abcdef12345678")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 0 {
		t.Errorf("expected 0 balances, got %d", len(balances))
	}
}

func TestEthereumRetryThenSuccess(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		action := r.URL.Query().Get("action")
		if action == "balance" {
			if n < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":"1000000000000000000"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"0","message":"No transactions found","result":[]}`))
	}))
	defer server.Close()

	c := NewEthereumConnector(server.URL, "test-key", server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	balances, err := c.FetchBalances(ctx, "0x1234567890abcdef1234567890abcdef12345678")
	if err != nil {
		t.Fatalf("expected success on third attempt, got: %v", err)
	}
	if len(balances) != 1 || balances[0].Asset != "ETH" {
		t.Fatalf("unexpected balances: %+v", balances)
	}
}

// --- Solana connector tests ---

func TestSolanaSOLBalanceParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 2.5 SOL = 2500000000 lamports
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"value":2500000000}}`))
	}))
	defer server.Close()

	c := NewSolanaConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "SomeValidSolanaAddress1234567890abcdef")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected 1 balance, got %d: %+v", len(balances), balances)
	}
	if balances[0].Asset != "SOL" {
		t.Errorf("expected SOL, got %s", balances[0].Asset)
	}
	if math.Abs(balances[0].Quantity-2.5) > 1e-10 {
		t.Errorf("expected 2.5 SOL, got %f", balances[0].Quantity)
	}
}

func TestSolanaSPLTokenParsing(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// SOL balance: 1 SOL
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"value":1000000000}}`))
			return
		}
		// SPL token accounts
		_, _ = w.Write([]byte(`{
			"jsonrpc": "2.0",
			"id": 1,
			"result": {
				"value": [
					{
						"account": {
							"data": {
								"parsed": {
									"info": {
										"mint": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
										"tokenAmount": {
											"uiAmount": 150.5
										}
									}
								}
							}
						}
					},
					{
						"account": {
							"data": {
								"parsed": {
									"info": {
										"mint": "So11111111111111111111111111111111111111112",
										"tokenAmount": {
											"uiAmount": 25.0
										}
									}
								}
							}
						}
					}
				]
			}
		}`))
	}))
	defer server.Close()

	c := NewSolanaConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "SomeValidSolanaAddress")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byAsset := make(map[string]float64)
	for _, b := range balances {
		byAsset[b.Asset] = b.Quantity
	}

	if math.Abs(byAsset["SOL"]-1.0) > 1e-10 {
		t.Errorf("expected SOL=1.0, got %f", byAsset["SOL"])
	}
	// SPL tokens use mint address as asset identifier
	if math.Abs(byAsset["EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"]-150.5) > 1e-10 {
		t.Errorf("expected USDC mint=150.5, got %f", byAsset["EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"])
	}
	if math.Abs(byAsset["So11111111111111111111111111111111111111112"]-25.0) > 1e-10 {
		t.Errorf("expected wrapped SOL mint=25.0, got %f", byAsset["So11111111111111111111111111111111111111112"])
	}
}

func TestSolanaZeroBalanceFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 0 lamports
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"value":0}}`))
	}))
	defer server.Close()

	c := NewSolanaConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "SomeValidSolanaAddress")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 0 {
		t.Errorf("expected 0 balances for zero SOL, got %d: %+v", len(balances), balances)
	}
}

func TestSolanaRetryOnServerError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewSolanaConnector(server.URL, server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := c.FetchBalances(ctx, "SomeValidSolanaAddress")
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if !strings.Contains(err.Error(), "max retries exceeded") {
		t.Errorf("expected max retries error, got: %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestSolanaNoRetryOnClientError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	c := NewSolanaConnector(server.URL, server.Client())
	_, err := c.FetchBalances(context.Background(), "SomeValidSolanaAddress")
	if err == nil {
		t.Fatal("expected error on 400")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Errorf("expected client error, got: %v", err)
	}
	if attempts.Load() != 1 {
		t.Errorf("expected exactly 1 attempt (no retry on 4xx), got %d", attempts.Load())
	}
}

func TestSolanaEmptyResponse(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"value":0}}`))
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"value":[]}}`))
	}))
	defer server.Close()

	c := NewSolanaConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "SomeValidSolanaAddress")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 0 {
		t.Errorf("expected 0 balances, got %d", len(balances))
	}
}

func TestSolanaRetryThenSuccess(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"value":5000000000}}`))
	}))
	defer server.Close()

	c := NewSolanaConnector(server.URL, server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	balances, err := c.FetchBalances(ctx, "SomeValidSolanaAddress")
	if err != nil {
		t.Fatalf("expected success on third attempt, got: %v", err)
	}
	if len(balances) != 1 || balances[0].Asset != "SOL" {
		t.Fatalf("unexpected balances: %+v", balances)
	}
}

// --- Bitcoin connector tests ---

func TestBitcoinBTCBalanceParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1.5 BTC = 150000000 satoshis funded, 0 spent (chain_stats only)
		_, _ = w.Write([]byte(`{
			"chain_stats": {
				"funded_txo_sum": 150000000,
				"spent_txo_sum": 0
			},
			"mempool_stats": {
				"funded_txo_sum": 0,
				"spent_txo_sum": 0
			}
		}`))
	}))
	defer server.Close()

	c := NewBitcoinConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "bc1qtest1234567890")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected 1 balance, got %d: %+v", len(balances), balances)
	}
	if balances[0].Asset != "BTC" {
		t.Errorf("expected BTC, got %s", balances[0].Asset)
	}
	if math.Abs(balances[0].Quantity-1.5) > 1e-10 {
		t.Errorf("expected 1.5 BTC, got %f", balances[0].Quantity)
	}
}

func TestBitcoinBalanceWithMempoolAndSpent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// chain: 200000000 funded - 50000000 spent = 150000000 confirmed
		// mempool: 10000000 funded - 5000000 spent = 5000000 unconfirmed
		// total: 155000000 satoshis = 1.55 BTC
		_, _ = w.Write([]byte(`{
			"chain_stats": {
				"funded_txo_sum": 200000000,
				"spent_txo_sum": 50000000
			},
			"mempool_stats": {
				"funded_txo_sum": 10000000,
				"spent_txo_sum": 5000000
			}
		}`))
	}))
	defer server.Close()

	c := NewBitcoinConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "bc1qtest1234567890")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected 1 balance, got %d", len(balances))
	}
	if math.Abs(balances[0].Quantity-1.55) > 1e-10 {
		t.Errorf("expected 1.55 BTC, got %f", balances[0].Quantity)
	}
}

func TestBitcoinZeroBalanceFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"chain_stats": {
				"funded_txo_sum": 0,
				"spent_txo_sum": 0
			},
			"mempool_stats": {
				"funded_txo_sum": 0,
				"spent_txo_sum": 0
			}
		}`))
	}))
	defer server.Close()

	c := NewBitcoinConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "bc1qtest1234567890")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 0 {
		t.Errorf("expected 0 balances for zero BTC, got %d: %+v", len(balances), balances)
	}
}

func TestBitcoinRetryOnServerError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewBitcoinConnector(server.URL, server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := c.FetchBalances(ctx, "bc1qtest1234567890")
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if !strings.Contains(err.Error(), "max retries exceeded") {
		t.Errorf("expected max retries error, got: %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestBitcoinNoRetryOnClientError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := NewBitcoinConnector(server.URL, server.Client())
	_, err := c.FetchBalances(context.Background(), "bc1qtest1234567890")
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Errorf("expected client error, got: %v", err)
	}
	if attempts.Load() != 1 {
		t.Errorf("expected exactly 1 attempt (no retry on 4xx), got %d", attempts.Load())
	}
}

func TestBitcoinEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"chain_stats": {
				"funded_txo_sum": 0,
				"spent_txo_sum": 0
			},
			"mempool_stats": {
				"funded_txo_sum": 0,
				"spent_txo_sum": 0
			}
		}`))
	}))
	defer server.Close()

	c := NewBitcoinConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "bc1qtest1234567890")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 0 {
		t.Errorf("expected 0 balances, got %d", len(balances))
	}
}

func TestBitcoinRetryThenSuccess(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{
			"chain_stats": {
				"funded_txo_sum": 100000000,
				"spent_txo_sum": 0
			},
			"mempool_stats": {
				"funded_txo_sum": 0,
				"spent_txo_sum": 0
			}
		}`))
	}))
	defer server.Close()

	c := NewBitcoinConnector(server.URL, server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	balances, err := c.FetchBalances(ctx, "bc1qtest1234567890")
	if err != nil {
		t.Fatalf("expected success on third attempt, got: %v", err)
	}
	if len(balances) != 1 || balances[0].Asset != "BTC" {
		t.Fatalf("unexpected balances: %+v", balances)
	}
	if math.Abs(balances[0].Quantity-1.0) > 1e-10 {
		t.Errorf("expected 1.0 BTC, got %f", balances[0].Quantity)
	}
}
