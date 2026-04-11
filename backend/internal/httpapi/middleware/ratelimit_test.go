package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/google/uuid"
)

// Mock RateLimitRepository for middleware tests

// mockMWRateLimitRepo is a configurable mock of auth.RateLimitRepository that
// returns preset counts. It follows the same pattern as mockRateLimitRepo in
// apiratelimit_test.go but lives in the middleware package.
type mockMWRateLimitRepo struct {
	tokenCount int
	ipCount    int
}

func (m *mockMWRateLimitRepo) CheckAndIncrement(_ context.Context, _ *uuid.UUID, _ *uuid.UUID, _ string, _ time.Duration) (int, error) {
	return m.tokenCount, nil
}

func (m *mockMWRateLimitRepo) CheckIPAndIncrement(_ context.Context, _ string, _ time.Duration) (int, error) {
	return m.ipCount, nil
}

func (m *mockMWRateLimitRepo) PruneExpired(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

// newTestRateLimitMiddleware creates a RateLimitMiddleware backed by a mock
// repo with the given token and IP counts.
func newTestRateLimitMiddleware(tokenCount, ipCount int) *RateLimitMiddleware {
	repo := &mockMWRateLimitRepo{tokenCount: tokenCount, ipCount: ipCount}
	cfg := auth.APIRateLimitConfig{
		UserLimit:    100,
		PremiumLimit: 500,
		AdminLimit:   1000,
		IPLimit:      200,
		Window:       time.Minute,
	}
	limiter := auth.NewAPIRateLimiter(repo, cfg)
	return NewRateLimitMiddleware(limiter)
}

// requestWithClaims creates an HTTP request with auth claims attached to the
// context, simulating what the auth middleware would do.
func requestWithClaims(method, path string, userID uuid.UUID, role string) *http.Request {
	claims := &auth.Claims{
		Sub:   userID.String(),
		Email: "test@exchangely.io",
		Role:  role,
	}
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "192.168.1.100:12345"
	ctx := context.WithValue(req.Context(), claimsKey, claims)
	return req.WithContext(ctx)
}

// TestRateLimitMiddleware_HeadersOnSuccess verifies that X-RateLimit-Limit,
// X-RateLimit-Remaining, and X-RateLimit-Reset headers are set on every
// authenticated response that is under the rate limit.
func TestRateLimitMiddleware_HeadersOnSuccess(t *testing.T) {
	// tokenCount=10, ipCount=5 — well under limits (user=100, ip=200).
	mw := newTestRateLimitMiddleware(10, 5)

	handler := mw.Wrap(okHandler)

	userID := uuid.New()
	req := requestWithClaims(http.MethodGet, "/api/v1/auth/me", userID, "user")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Verify X-RateLimit-Limit header.
	limitHeader := rr.Header().Get("X-RateLimit-Limit")
	if limitHeader == "" {
		t.Fatal("missing X-RateLimit-Limit header")
	}
	limitVal, err := strconv.Atoi(limitHeader)
	if err != nil {
		t.Fatalf("X-RateLimit-Limit not a number: %s", limitHeader)
	}
	if limitVal != 100 {
		t.Fatalf("X-RateLimit-Limit = %d, want 100", limitVal)
	}

	// Verify X-RateLimit-Remaining header.
	remainingHeader := rr.Header().Get("X-RateLimit-Remaining")
	if remainingHeader == "" {
		t.Fatal("missing X-RateLimit-Remaining header")
	}
	remainingVal, err := strconv.Atoi(remainingHeader)
	if err != nil {
		t.Fatalf("X-RateLimit-Remaining not a number: %s", remainingHeader)
	}
	// remaining = min(100-10, 200-5) = min(90, 195) = 90
	if remainingVal != 90 {
		t.Fatalf("X-RateLimit-Remaining = %d, want 90", remainingVal)
	}

	// Verify X-RateLimit-Reset header is a valid Unix timestamp in the future.
	resetHeader := rr.Header().Get("X-RateLimit-Reset")
	if resetHeader == "" {
		t.Fatal("missing X-RateLimit-Reset header")
	}
	resetVal, err := strconv.ParseInt(resetHeader, 10, 64)
	if err != nil {
		t.Fatalf("X-RateLimit-Reset not a number: %s", resetHeader)
	}
	if resetVal <= time.Now().Unix() {
		t.Fatalf("X-RateLimit-Reset %d should be in the future", resetVal)
	}
}

// TestRateLimitMiddleware_HeadersForPremiumRole verifies that the rate limit
// headers reflect the premium tier limit when the user has a premium role.
func TestRateLimitMiddleware_HeadersForPremiumRole(t *testing.T) {
	mw := newTestRateLimitMiddleware(50, 10)
	handler := mw.Wrap(okHandler)

	userID := uuid.New()
	req := requestWithClaims(http.MethodGet, "/api/v1/auth/me", userID, "premium")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	limitHeader := rr.Header().Get("X-RateLimit-Limit")
	limitVal, _ := strconv.Atoi(limitHeader)
	if limitVal != 500 {
		t.Fatalf("X-RateLimit-Limit = %d, want 500 for premium", limitVal)
	}

	remainingHeader := rr.Header().Get("X-RateLimit-Remaining")
	remainingVal, _ := strconv.Atoi(remainingHeader)
	// remaining = min(500-50, 200-10) = min(450, 190) = 190
	if remainingVal != 190 {
		t.Fatalf("X-RateLimit-Remaining = %d, want 190 for premium", remainingVal)
	}
}

// TestRateLimitMiddleware_429WithRetryAfter verifies that when the rate limit
// is exceeded, the middleware returns 429 with a Retry-After header.
func TestRateLimitMiddleware_429WithRetryAfter(t *testing.T) {
	// Override nowUnix so Retry-After is deterministic.
	fixedNow := time.Now().Unix()
	origNowUnix := nowUnix
	nowUnix = func() int64 { return fixedNow }
	defer func() { nowUnix = origNowUnix }()

	// tokenCount=101 exceeds user limit of 100.
	mw := newTestRateLimitMiddleware(101, 5)
	handler := mw.Wrap(okHandler)

	userID := uuid.New()
	req := requestWithClaims(http.MethodGet, "/api/v1/auth/me", userID, "user")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}

	// Verify Retry-After header is present and positive.
	retryAfter := rr.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("missing Retry-After header on 429 response")
	}
	retryVal, err := strconv.ParseInt(retryAfter, 10, 64)
	if err != nil {
		t.Fatalf("Retry-After not a number: %s", retryAfter)
	}
	if retryVal < 1 {
		t.Fatalf("Retry-After = %d, want >= 1", retryVal)
	}

	// Rate limit headers should still be present on 429 responses.
	if rr.Header().Get("X-RateLimit-Limit") == "" {
		t.Fatal("missing X-RateLimit-Limit header on 429 response")
	}
	if rr.Header().Get("X-RateLimit-Remaining") == "" {
		t.Fatal("missing X-RateLimit-Remaining header on 429 response")
	}
	if rr.Header().Get("X-RateLimit-Reset") == "" {
		t.Fatal("missing X-RateLimit-Reset header on 429 response")
	}

	// Remaining should be 0 when over limit.
	remainingVal, _ := strconv.Atoi(rr.Header().Get("X-RateLimit-Remaining"))
	if remainingVal != 0 {
		t.Fatalf("X-RateLimit-Remaining = %d, want 0 when over limit", remainingVal)
	}
}

// TestRateLimitMiddleware_429OnIPLimitExceeded verifies that exceeding the
// per-IP limit also triggers a 429 response, even if the per-token limit is
// not exceeded.
func TestRateLimitMiddleware_429OnIPLimitExceeded(t *testing.T) {
	// tokenCount=50 (under user limit 100), ipCount=201 (over IP limit 200).
	mw := newTestRateLimitMiddleware(50, 201)
	handler := mw.Wrap(okHandler)

	userID := uuid.New()
	req := requestWithClaims(http.MethodGet, "/api/v1/auth/me", userID, "user")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on IP limit exceeded, got %d", rr.Code)
	}

	retryAfter := rr.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("missing Retry-After header on IP-based 429 response")
	}
}

// TestRateLimitMiddleware_PublicPathExemption verifies that public paths are
// exempt from rate limiting — they pass through even without claims in context.
func TestRateLimitMiddleware_PublicPathExemption(t *testing.T) {
	// Use counts that would trigger 429 on non-public paths.
	mw := newTestRateLimitMiddleware(9999, 9999)
	handler := mw.Wrap(okHandler)

	publicPaths := []string{
		"/api/v1/health",
		"/api/v1/assets",
		"/api/v1/pairs",
		"/api/v1/tickers",
		"/api/v1/news",
		"/api/v1/config",
		"/api/v1/auth/refresh",
		"/api/v1/auth/logout",
	}
	publicPrefixPaths := []string{
		"/api/v1/ticker/BTC-USD",
		"/api/v1/historical/ETH-EUR",
		"/api/v1/tickers/stream",
		"/api/v1/news/stream",
		"/api/v1/auth/google/login",
		"/api/v1/auth/local/login",
		"/swagger",
		"/swagger/openapi.yaml",
	}

	for _, p := range append(publicPaths, publicPrefixPaths...) {
		t.Run(fmt.Sprintf("public_%s", p), func(t *testing.T) {
			// No claims in context — public paths should still pass.
			req := httptest.NewRequest(http.MethodGet, p, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("public path %s: expected 200, got %d", p, rr.Code)
			}

			// Public paths should NOT have rate limit headers.
			if rr.Header().Get("X-RateLimit-Limit") != "" {
				t.Fatalf("public path %s should not have X-RateLimit-Limit header", p)
			}
		})
	}
}

// TestRateLimitMiddleware_JWTSessionRateLimiting verifies that when a request
// is authenticated via JWT session (no API token), the rate limiter still
// applies based on the user's role and user ID from the JWT claims.
func TestRateLimitMiddleware_JWTSessionRateLimiting(t *testing.T) {
	// Under limit: tokenCount=20, ipCount=10.
	mw := newTestRateLimitMiddleware(20, 10)

	// Track that the next handler is called (proving the request was allowed).
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler := mw.Wrap(next)

	userID := uuid.New()
	// Simulate a JWT-authenticated request (claims in context, no API token).
	req := requestWithClaims(http.MethodGet, "/api/v1/auth/me", userID, "user")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !nextCalled {
		t.Fatal("expected next handler to be called for JWT session under rate limit")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Verify rate limit headers are present for JWT session requests.
	if rr.Header().Get("X-RateLimit-Limit") == "" {
		t.Fatal("missing X-RateLimit-Limit header on JWT session request")
	}
	if rr.Header().Get("X-RateLimit-Remaining") == "" {
		t.Fatal("missing X-RateLimit-Remaining header on JWT session request")
	}
	if rr.Header().Get("X-RateLimit-Reset") == "" {
		t.Fatal("missing X-RateLimit-Reset header on JWT session request")
	}

	limitVal, _ := strconv.Atoi(rr.Header().Get("X-RateLimit-Limit"))
	if limitVal != 100 {
		t.Fatalf("X-RateLimit-Limit = %d, want 100 for user role", limitVal)
	}
}

// TestRateLimitMiddleware_JWTSessionOverLimit verifies that JWT session
// requests are also rate limited and receive 429 when over the limit.
func TestRateLimitMiddleware_JWTSessionOverLimit(t *testing.T) {
	// tokenCount=101 exceeds user limit of 100.
	mw := newTestRateLimitMiddleware(101, 5)
	handler := mw.Wrap(okHandler)

	userID := uuid.New()
	req := requestWithClaims(http.MethodGet, "/api/v1/auth/me", userID, "user")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for JWT session over limit, got %d", rr.Code)
	}
}

// TestRateLimitMiddleware_NilLimiterBypass verifies that when the limiter is
// nil (rate limiting not configured), all requests pass through unchanged.
func TestRateLimitMiddleware_NilLimiterBypass(t *testing.T) {
	mw := NewRateLimitMiddleware(nil)
	handler := mw.Wrap(okHandler)

	userID := uuid.New()
	req := requestWithClaims(http.MethodGet, "/api/v1/auth/me", userID, "user")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("nil limiter: expected 200, got %d", rr.Code)
	}

	// No rate limit headers should be set when limiter is nil.
	if rr.Header().Get("X-RateLimit-Limit") != "" {
		t.Fatal("nil limiter should not set X-RateLimit-Limit header")
	}
}

// TestRateLimitMiddleware_NoClaimsPassThrough verifies that unauthenticated
// requests (no claims in context) pass through without rate limiting.
func TestRateLimitMiddleware_NoClaimsPassThrough(t *testing.T) {
	mw := newTestRateLimitMiddleware(9999, 9999)
	handler := mw.Wrap(okHandler)

	// Request to a non-public path without claims.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("no claims: expected 200 (pass through), got %d", rr.Code)
	}

	// No rate limit headers when unauthenticated.
	if rr.Header().Get("X-RateLimit-Limit") != "" {
		t.Fatal("unauthenticated request should not have X-RateLimit-Limit header")
	}
}
