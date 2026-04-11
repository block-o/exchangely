package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/google/uuid"
)

// BenchmarkAPITokenMiddleware_ValidToken measures the full middleware
// pass-through with a valid API token: header extraction, SHA-256 hash,
// repository lookup, claims attachment, and context propagation.
func BenchmarkAPITokenMiddleware_ValidToken(b *testing.B) {
	rawToken, token, user := testTokenAndUser()
	mw := newValidatingAPITokenMiddleware(token, user)
	handler := mw.Wrap(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d", rr.Code)
		}
	}
}

// BenchmarkAPITokenMiddleware_PublicPath measures the public path bypass
// latency — the middleware should short-circuit without any token validation.
func BenchmarkAPITokenMiddleware_PublicPath(b *testing.B) {
	mw := newTestAPITokenMiddleware()
	handler := mw.Wrap(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d", rr.Code)
		}
	}
}

// BenchmarkRateLimitMiddleware_UnderLimit measures rate limit middleware
// overhead on allowed requests, including rate-limit header writing.
func BenchmarkRateLimitMiddleware_UnderLimit(b *testing.B) {
	// tokenCount=10, ipCount=5 — well under limits (user=100, ip=200).
	mw := newTestRateLimitMiddleware(10, 5)
	handler := mw.Wrap(okHandler)

	userID := uuid.New()
	claims := &auth.Claims{
		Sub:   userID.String(),
		Email: "bench@exchangely.io",
		Role:  "user",
	}
	baseReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	baseReq.RemoteAddr = "192.168.1.100:12345"
	ctx := context.WithValue(baseReq.Context(), claimsKey, claims)
	req := baseReq.WithContext(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d", rr.Code)
		}
	}
}

// BenchmarkFullChain measures the combined APITokenMW → RateLimitMW chain
// with a valid token under the rate limit. This represents the realistic
// hot path for an API-token-authenticated request.
func BenchmarkFullChain(b *testing.B) {
	// Set up API token middleware with a valid token.
	rawToken, token, user := testTokenAndUser()
	apiTokenMW := newValidatingAPITokenMiddleware(token, user)

	// Set up rate limit middleware with counts well under limits.
	rateLimitMW := newTestRateLimitMiddleware(10, 5)

	// Chain: APITokenMW → RateLimitMW → okHandler
	handler := apiTokenMW.Wrap(rateLimitMW.Wrap(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.RemoteAddr = "192.168.1.100:12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d", rr.Code)
		}
	}
}
