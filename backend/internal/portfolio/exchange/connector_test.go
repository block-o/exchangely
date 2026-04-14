package exchange

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// --- Binance connector tests ---

func TestBinanceRequestSigning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/sapi/v1/simple-earn/") {
			_, _ = w.Write([]byte(`{"rows":[],"total":0}`))
			return
		}
		apiKey := r.Header.Get("X-MBX-APIKEY")
		if apiKey != "test-api-key" {
			t.Errorf("expected X-MBX-APIKEY=test-api-key, got %q", apiKey)
		}
		sig := r.URL.Query().Get("signature")
		if sig == "" {
			t.Error("expected signature parameter in query string, got empty")
		}
		ts := r.URL.Query().Get("timestamp")
		if ts == "" {
			t.Error("expected timestamp parameter in query string, got empty")
		}
		_, _ = w.Write([]byte(`{"balances":[{"asset":"BTC","free":"1.5","locked":"0.5"}]}`))
	}))
	defer server.Close()

	c := NewBinanceConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "test-api-key", "test-secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 || balances[0].Asset != "BTC" {
		t.Fatalf("unexpected balances: %+v", balances)
	}
}

func TestBinanceBalanceParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/sapi/v1/simple-earn/") {
			_, _ = w.Write([]byte(`{"rows":[],"total":0}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"balances": [
				{"asset": "BTC", "free": "1.25", "locked": "0.75"},
				{"asset": "ETH", "free": "10.0", "locked": "0.0"},
				{"asset": "SOL", "free": "100.5", "locked": "5.5"}
			]
		}`))
	}))
	defer server.Close()

	c := NewBinanceConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "key", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 3 {
		t.Fatalf("expected 3 balances, got %d", len(balances))
	}

	byAsset := make(map[string]float64)
	for _, b := range balances {
		byAsset[b.Asset] = b.Quantity
	}
	if byAsset["BTC"] != 2.0 {
		t.Errorf("expected BTC=2.0 (free+locked), got %f", byAsset["BTC"])
	}
	if byAsset["ETH"] != 10.0 {
		t.Errorf("expected ETH=10.0, got %f", byAsset["ETH"])
	}
	if byAsset["SOL"] != 106.0 {
		t.Errorf("expected SOL=106.0, got %f", byAsset["SOL"])
	}
}

func TestBinanceZeroBalanceFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/sapi/v1/simple-earn/") {
			_, _ = w.Write([]byte(`{"rows":[],"total":0}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"balances": [
				{"asset": "BTC", "free": "1.0", "locked": "0.0"},
				{"asset": "ETH", "free": "0.0", "locked": "0.0"},
				{"asset": "DOGE", "free": "0.00000000", "locked": "0.00000000"}
			]
		}`))
	}))
	defer server.Close()

	c := NewBinanceConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "key", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected 1 non-zero balance, got %d: %+v", len(balances), balances)
	}
	if balances[0].Asset != "BTC" {
		t.Errorf("expected BTC, got %s", balances[0].Asset)
	}
}

// --- Kraken connector tests ---

func TestKrakenRequestAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("API-Key")
		if apiKey != "test-kraken-key" {
			t.Errorf("expected API-Key=test-kraken-key, got %q", apiKey)
		}
		apiSign := r.Header.Get("API-Sign")
		if apiSign == "" {
			t.Error("expected API-Sign header to be set, got empty")
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("expected form content type, got %q", r.Header.Get("Content-Type"))
		}
		_, _ = w.Write([]byte(`{"error":[],"result":{"XXBT":"1.5","XETH":"10.0"}}`))
	}))
	defer server.Close()

	// Kraken expects base64-encoded secret
	c := NewKrakenConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "test-kraken-key", "dGVzdC1zZWNyZXQ=")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 2 {
		t.Fatalf("expected 2 balances, got %d", len(balances))
	}
}

func TestKrakenBalanceParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"error": [],
			"result": {
				"XXBT": "2.5",
				"XETH": "15.0",
				"ZUSD": "1000.0",
				"SOL": "50.0"
			}
		}`))
	}))
	defer server.Close()

	c := NewKrakenConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "key", "dGVzdC1zZWNyZXQ=")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 4 {
		t.Fatalf("expected 4 balances, got %d", len(balances))
	}

	byAsset := make(map[string]float64)
	for _, b := range balances {
		byAsset[b.Asset] = b.Quantity
	}
	// Kraken normalizes XXBT -> BTC, XETH -> ETH, ZUSD -> USD
	if byAsset["BTC"] != 2.5 {
		t.Errorf("expected BTC=2.5, got %f", byAsset["BTC"])
	}
	if byAsset["ETH"] != 15.0 {
		t.Errorf("expected ETH=15.0, got %f", byAsset["ETH"])
	}
	if byAsset["USD"] != 1000.0 {
		t.Errorf("expected USD=1000.0, got %f", byAsset["USD"])
	}
	if byAsset["SOL"] != 50.0 {
		t.Errorf("expected SOL=50.0, got %f", byAsset["SOL"])
	}
}

func TestKrakenZeroBalanceFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"error": [],
			"result": {
				"XXBT": "1.0",
				"XETH": "0.0000000000",
				"ZUSD": "0"
			}
		}`))
	}))
	defer server.Close()

	c := NewKrakenConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "key", "dGVzdC1zZWNyZXQ=")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected 1 non-zero balance, got %d: %+v", len(balances), balances)
	}
	if balances[0].Asset != "BTC" {
		t.Errorf("expected BTC, got %s", balances[0].Asset)
	}
}

// --- Coinbase connector tests ---

// coinbaseTestKey generates a fresh EC key pair and returns the PEM string
// for use as the apiSecret in Coinbase connector tests.
func coinbaseTestPEM() string {
	_, pemStr := generateTestECKey()
	return pemStr
}

func TestCoinbaseRequestAuth(t *testing.T) {
	pemKey := coinbaseTestPEM()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("expected Authorization: Bearer header, got %q", auth)
		}
		_, _ = w.Write([]byte(`{
			"accounts": [{"currency": "BTC", "available_balance": {"value": "1.0"}}],
			"cursor": ""
		}`))
	}))
	defer server.Close()

	c := NewCoinbaseConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "organizations/test/apiKeys/test-key", pemKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 || balances[0].Asset != "BTC" {
		t.Fatalf("unexpected balances: %+v", balances)
	}
}

func TestCoinbaseBalanceParsing(t *testing.T) {
	pemKey := coinbaseTestPEM()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"accounts": [
				{"currency": "BTC", "available_balance": {"value": "2.5"}},
				{"currency": "ETH", "available_balance": {"value": "30.0"}},
				{"currency": "USDC", "available_balance": {"value": "5000.0"}}
			],
			"cursor": ""
		}`))
	}))
	defer server.Close()

	c := NewCoinbaseConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "key", pemKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 3 {
		t.Fatalf("expected 3 balances, got %d", len(balances))
	}

	byAsset := make(map[string]float64)
	for _, b := range balances {
		byAsset[b.Asset] = b.Quantity
	}
	if byAsset["BTC"] != 2.5 {
		t.Errorf("expected BTC=2.5, got %f", byAsset["BTC"])
	}
	if byAsset["ETH"] != 30.0 {
		t.Errorf("expected ETH=30.0, got %f", byAsset["ETH"])
	}
	if byAsset["USDC"] != 5000.0 {
		t.Errorf("expected USDC=5000.0, got %f", byAsset["USDC"])
	}
}

func TestCoinbaseZeroBalanceFiltering(t *testing.T) {
	pemKey := coinbaseTestPEM()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"accounts": [
				{"currency": "BTC", "available_balance": {"value": "1.0"}},
				{"currency": "ETH", "available_balance": {"value": "0.0"}},
				{"currency": "DOGE", "available_balance": {"value": "0.00000000"}}
			],
			"cursor": ""
		}`))
	}))
	defer server.Close()

	c := NewCoinbaseConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "key", pemKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected 1 non-zero balance, got %d: %+v", len(balances), balances)
	}
	if balances[0].Asset != "BTC" {
		t.Errorf("expected BTC, got %s", balances[0].Asset)
	}
}

func TestCoinbasePagination(t *testing.T) {
	pemKey := coinbaseTestPEM()
	page := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		if page == 1 {
			_, _ = w.Write([]byte(`{
				"accounts": [{"currency": "BTC", "available_balance": {"value": "1.0"}}],
				"cursor": "page2"
			}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"accounts": [{"currency": "ETH", "available_balance": {"value": "5.0"}}],
			"cursor": ""
		}`))
	}))
	defer server.Close()

	c := NewCoinbaseConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "key", pemKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 2 {
		t.Fatalf("expected 2 balances across pages, got %d", len(balances))
	}
	if page != 2 {
		t.Errorf("expected 2 page requests, got %d", page)
	}
}

// --- Retry behavior tests ---

func TestBinanceRetryOnServerError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	c := NewBinanceConnector(server.URL, server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := c.FetchBalances(ctx, "key", "secret")
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

func TestKrakenRetryOnServerError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	c := NewKrakenConnector(server.URL, server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := c.FetchBalances(ctx, "key", "dGVzdC1zZWNyZXQ=")
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

func TestCoinbaseRetryOnServerError(t *testing.T) {
	pemKey := coinbaseTestPEM()
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	c := NewCoinbaseConnector(server.URL, server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := c.FetchBalances(ctx, "key", pemKey)
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

// --- Client error (4xx) does NOT retry ---

func TestBinanceNoRetryOnClientError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	c := NewBinanceConnector(server.URL, server.Client())
	_, err := c.FetchBalances(context.Background(), "key", "secret")
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

func TestKrakenNoRetryOnClientError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := NewKrakenConnector(server.URL, server.Client())
	_, err := c.FetchBalances(context.Background(), "key", "dGVzdC1zZWNyZXQ=")
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Errorf("expected client error, got: %v", err)
	}
	if attempts.Load() != 1 {
		t.Errorf("expected exactly 1 attempt (no retry on 4xx), got %d", attempts.Load())
	}
}

func TestCoinbaseNoRetryOnClientError(t *testing.T) {
	pemKey := coinbaseTestPEM()
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	c := NewCoinbaseConnector(server.URL, server.Client())
	_, err := c.FetchBalances(context.Background(), "key", pemKey)
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

// --- Connector name tests ---

func TestConnectorNames(t *testing.T) {
	b := NewBinanceConnector("", nil)
	if b.Name() != "binance" {
		t.Errorf("expected binance, got %s", b.Name())
	}
	k := NewKrakenConnector("", nil)
	if k.Name() != "kraken" {
		t.Errorf("expected kraken, got %s", k.Name())
	}
	cb := NewCoinbaseConnector("", nil)
	if cb.Name() != "coinbase" {
		t.Errorf("expected coinbase, got %s", cb.Name())
	}
}

// --- Interface compliance ---

func TestConnectorInterfaceCompliance(t *testing.T) {
	var _ Connector = (*BinanceConnector)(nil)
	var _ Connector = (*KrakenConnector)(nil)
	var _ Connector = (*CoinbaseConnector)(nil)
}

// --- Retry then success ---

func TestBinanceRetryThenSuccess(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/sapi/v1/simple-earn/") {
			_, _ = w.Write([]byte(`{"rows":[],"total":0}`))
			return
		}
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"balances":[{"asset":"BTC","free":"1.0","locked":"0.0"}]}`))
	}))
	defer server.Close()

	c := NewBinanceConnector(server.URL, server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	balances, err := c.FetchBalances(ctx, "key", "secret")
	if err != nil {
		t.Fatalf("expected success on third attempt, got: %v", err)
	}
	if len(balances) != 1 || balances[0].Asset != "BTC" {
		t.Fatalf("unexpected balances: %+v", balances)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

// --- Binance signature verification ---

func TestBinanceSignatureIsHMACSHA256(t *testing.T) {
	var capturedSig string
	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/sapi/v1/simple-earn/") {
			_, _ = w.Write([]byte(`{"rows":[],"total":0}`))
			return
		}
		capturedSig = r.URL.Query().Get("signature")
		// Remove signature from query to get the signed payload
		q := r.URL.Query()
		q.Del("signature")
		capturedQuery = q.Encode()
		_, _ = w.Write([]byte(`{"balances":[]}`))
	}))
	defer server.Close()

	c := NewBinanceConnector(server.URL, server.Client())
	_, _ = c.FetchBalances(context.Background(), "key", "secret")

	if capturedSig == "" {
		t.Fatal("signature was not set")
	}
	// Signature should be a hex string (64 chars for SHA256)
	if len(capturedSig) != 64 {
		t.Errorf("expected 64-char hex signature, got %d chars: %s", len(capturedSig), capturedSig)
	}
	if capturedQuery == "" {
		t.Error("expected query parameters to be present")
	}
}

// --- Kraken API error handling ---

func TestKrakenAPIErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":["EAPI:Invalid key"],"result":{}}`))
	}))
	defer server.Close()

	c := NewKrakenConnector(server.URL, server.Client())
	_, err := c.FetchBalances(context.Background(), "key", "dGVzdC1zZWNyZXQ=")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if !strings.Contains(err.Error(), "EAPI:Invalid key") {
		t.Errorf("expected API error message, got: %v", err)
	}
}

// --- Coinbase JSON decode error ---

func TestCoinbaseMalformedJSON(t *testing.T) {
	pemKey := coinbaseTestPEM()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{invalid-json`))
	}))
	defer server.Close()

	c := NewCoinbaseConnector(server.URL, server.Client())
	_, err := c.FetchBalances(context.Background(), "key", pemKey)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode error") {
		t.Errorf("expected decode error, got: %v", err)
	}
}

func TestCoinbaseInvalidPEMKey(t *testing.T) {
	c := NewCoinbaseConnector("https://api.coinbase.com", nil)
	_, err := c.FetchBalances(context.Background(), "key", "not-a-pem-key")
	if err == nil {
		t.Fatal("expected error for invalid PEM key")
	}
	if !strings.Contains(err.Error(), "no PEM block") {
		t.Errorf("expected PEM parse error, got: %v", err)
	}
}

func TestCoinbasePEMWithEscapedNewlines(t *testing.T) {
	_, realPEM := generateTestECKey()
	// Simulate a key copied from a JSON/.env file with literal \n sequences.
	escaped := strings.ReplaceAll(realPEM, "\n", `\n`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("expected Bearer auth, got %q", auth)
		}
		_, _ = w.Write([]byte(`{"accounts":[{"currency":"BTC","available_balance":{"value":"1.0"}}],"cursor":""}`))
	}))
	defer server.Close()

	c := NewCoinbaseConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "organizations/test/apiKeys/key", escaped)
	if err != nil {
		t.Fatalf("expected escaped \\n PEM to work, got: %v", err)
	}
	if len(balances) != 1 || balances[0].Asset != "BTC" {
		t.Fatalf("unexpected balances: %+v", balances)
	}
}

func TestCoinbaseJWTContainsExpectedClaims(t *testing.T) {
	key, pemStr := generateTestECKey()
	c := NewCoinbaseConnector("https://api.coinbase.com", nil)

	token, err := c.buildJWT("organizations/test-org/apiKeys/test-key", pemStr, "GET", "/api/v3/brokerage/accounts")
	if err != nil {
		t.Fatalf("unexpected error building JWT: %v", err)
	}

	parsed, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		return &key.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("failed to parse JWT: %v", err)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("expected MapClaims")
	}

	if claims["sub"] != "organizations/test-org/apiKeys/test-key" {
		t.Errorf("expected sub=organizations/test-org/apiKeys/test-key, got %v", claims["sub"])
	}
	if claims["iss"] != "cdp" {
		t.Errorf("expected iss=cdp, got %v", claims["iss"])
	}
	uri, _ := claims["uri"].(string)
	if !strings.Contains(uri, "GET api.coinbase.com/api/v3/brokerage/accounts") {
		t.Errorf("expected uri to contain request path, got %v", uri)
	}

	kid, _ := parsed.Header["kid"].(string)
	if kid != "organizations/test-org/apiKeys/test-key" {
		t.Errorf("expected kid header to match apiKey, got %v", kid)
	}
	nonce, _ := parsed.Header["nonce"].(string)
	if nonce == "" {
		t.Error("expected nonce header to be set")
	}
}

// --- Default URL tests ---

func TestDefaultURLs(t *testing.T) {
	b := NewBinanceConnector("", nil)
	if b.baseURL != "https://api.binance.com" {
		t.Errorf("expected default binance URL, got %s", b.baseURL)
	}
	k := NewKrakenConnector("", nil)
	if k.baseURL != "https://api.kraken.com" {
		t.Errorf("expected default kraken URL, got %s", k.baseURL)
	}
	cb := NewCoinbaseConnector("", nil)
	if cb.baseURL != "https://api.coinbase.com" {
		t.Errorf("expected default coinbase URL, got %s", cb.baseURL)
	}
}

// --- Verify all connectors handle empty response gracefully ---

func TestEmptyBalanceResponses(t *testing.T) {
	t.Run("binance empty", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasPrefix(r.URL.Path, "/sapi/v1/simple-earn/"):
				_, _ = w.Write([]byte(`{"rows":[],"total":0}`))
			default:
				_, _ = w.Write([]byte(`{"balances":[]}`))
			}
		}))
		defer server.Close()
		c := NewBinanceConnector(server.URL, server.Client())
		balances, err := c.FetchBalances(context.Background(), "key", "secret")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(balances) != 0 {
			t.Errorf("expected 0 balances, got %d", len(balances))
		}
	})

	t.Run("kraken empty", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"error":[],"result":{}}`))
		}))
		defer server.Close()
		c := NewKrakenConnector(server.URL, server.Client())
		balances, err := c.FetchBalances(context.Background(), "key", "dGVzdC1zZWNyZXQ=")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(balances) != 0 {
			t.Errorf("expected 0 balances, got %d", len(balances))
		}
	})

	t.Run("coinbase empty", func(t *testing.T) {
		pemKey := coinbaseTestPEM()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"accounts":[],"cursor":""}`))
		}))
		defer server.Close()
		c := NewCoinbaseConnector(server.URL, server.Client())
		balances, err := c.FetchBalances(context.Background(), "key", pemKey)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(balances) != 0 {
			t.Errorf("expected 0 balances, got %d", len(balances))
		}
	})
}

// --- Binance earn position tests ---

func TestBinanceEarnPositionsMerged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/sapi/v1/simple-earn/flexible/position"):
			_, _ = w.Write([]byte(`{"rows":[{"asset":"BTC","totalAmount":"0.5"},{"asset":"ETH","totalAmount":"2.0"}],"total":2}`))
		case strings.HasPrefix(r.URL.Path, "/sapi/v1/simple-earn/locked/position"):
			_, _ = w.Write([]byte(`{"rows":[{"asset":"BTC","totalAmount":"1.0"}],"total":1}`))
		default:
			_, _ = w.Write([]byte(`{"balances":[{"asset":"BTC","free":"1.0","locked":"0.0"},{"asset":"SOL","free":"50.0","locked":"0.0"}]}`))
		}
	}))
	defer server.Close()

	c := NewBinanceConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "key", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byAsset := make(map[string]float64)
	for _, b := range balances {
		byAsset[b.Asset] = b.Quantity
	}

	// BTC: 1.0 spot + 0.5 flexible + 1.0 locked = 2.5
	if byAsset["BTC"] != 2.5 {
		t.Errorf("expected BTC=2.5 (spot+earn), got %f", byAsset["BTC"])
	}
	// ETH: 0 spot + 2.0 flexible = 2.0
	if byAsset["ETH"] != 2.0 {
		t.Errorf("expected ETH=2.0 (earn only), got %f", byAsset["ETH"])
	}
	// SOL: 50.0 spot only
	if byAsset["SOL"] != 50.0 {
		t.Errorf("expected SOL=50.0 (spot only), got %f", byAsset["SOL"])
	}
}

func TestBinanceEarnEndpointFailureGraceful(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/sapi/v1/simple-earn/") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"balances":[{"asset":"BTC","free":"1.0","locked":"0.0"}]}`))
	}))
	defer server.Close()

	c := NewBinanceConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "key", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should still return spot balances even if earn fails.
	if len(balances) != 1 || balances[0].Asset != "BTC" {
		t.Fatalf("expected spot balances despite earn failure, got: %+v", balances)
	}
}

func TestBinanceEarnEndpointSigning(t *testing.T) {
	var earnSigs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/sapi/v1/simple-earn/") {
			sig := r.URL.Query().Get("signature")
			if sig == "" {
				t.Error("expected signature on earn endpoint")
			}
			apiKey := r.Header.Get("X-MBX-APIKEY")
			if apiKey != "test-key" {
				t.Errorf("expected X-MBX-APIKEY=test-key on earn endpoint, got %q", apiKey)
			}
			earnSigs = append(earnSigs, sig)
			_, _ = w.Write([]byte(`{"rows":[],"total":0}`))
			return
		}
		_, _ = w.Write([]byte(`{"balances":[]}`))
	}))
	defer server.Close()

	c := NewBinanceConnector(server.URL, server.Client())
	_, _ = c.FetchBalances(context.Background(), "test-key", "test-secret")

	// Should have called both flexible and locked endpoints.
	if len(earnSigs) != 2 {
		t.Errorf("expected 2 earn endpoint calls (flexible + locked), got %d", len(earnSigs))
	}
}

func TestBinanceEarnEmptyRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/sapi/v1/simple-earn/") {
			_, _ = w.Write([]byte(`{"rows":[],"total":0}`))
			return
		}
		_, _ = w.Write([]byte(`{"balances":[{"asset":"BTC","free":"1.0","locked":"0.0"}]}`))
	}))
	defer server.Close()

	c := NewBinanceConnector(server.URL, server.Client())
	balances, err := c.FetchBalances(context.Background(), "key", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 || balances[0].Asset != "BTC" || balances[0].Quantity != 1.0 {
		t.Fatalf("expected only spot BTC=1.0, got: %+v", balances)
	}
}

func TestMergeBalances(t *testing.T) {
	spot := []Balance{{Asset: "BTC", Quantity: 1.0}, {Asset: "ETH", Quantity: 5.0}}
	earn := []Balance{{Asset: "BTC", Quantity: 0.5}, {Asset: "SOL", Quantity: 10.0}}

	merged := mergeBalances(spot, earn)
	byAsset := make(map[string]float64)
	for _, b := range merged {
		byAsset[b.Asset] = b.Quantity
	}

	if byAsset["BTC"] != 1.5 {
		t.Errorf("expected BTC=1.5, got %f", byAsset["BTC"])
	}
	if byAsset["ETH"] != 5.0 {
		t.Errorf("expected ETH=5.0, got %f", byAsset["ETH"])
	}
	if byAsset["SOL"] != 10.0 {
		t.Errorf("expected SOL=10.0, got %f", byAsset["SOL"])
	}
}

func TestMergeBalancesEmptyEarn(t *testing.T) {
	spot := []Balance{{Asset: "BTC", Quantity: 1.0}}
	merged := mergeBalances(spot, nil)
	if len(merged) != 1 || merged[0].Asset != "BTC" || merged[0].Quantity != 1.0 {
		t.Fatalf("expected unchanged spot balances, got: %+v", merged)
	}
}
