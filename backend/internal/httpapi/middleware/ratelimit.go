package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/google/uuid"
)

// RateLimitMiddleware enforces per-token and per-IP rate limits on
// authenticated requests. It runs after both the API token and JWT auth
// middleware so that claims are already attached to the context.
type RateLimitMiddleware struct {
	limiter        *auth.APIRateLimiter
	publicPaths    []string
	publicPrefixes []string
}

// NewRateLimitMiddleware creates a RateLimitMiddleware with the same public
// route lists as AuthMiddleware and APITokenMiddleware.
func NewRateLimitMiddleware(limiter *auth.APIRateLimiter) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		limiter: limiter,
		publicPaths: []string{
			"/api/v1/health",
			"/api/v1/assets",
			"/api/v1/pairs",
			"/api/v1/tickers",
			"/api/v1/news",
			"/api/v1/config",
			"/api/v1/auth/refresh",
			"/api/v1/auth/logout",
		},
		publicPrefixes: []string{
			"/api/v1/ticker/",
			"/api/v1/historical/",
			"/api/v1/tickers/stream",
			"/api/v1/news/stream",
			"/api/v1/auth/google/",
			"/api/v1/auth/local/login",
			"/swagger",
		},
	}
}

// Wrap returns an http.Handler that enforces rate limits on authenticated
// requests. Public paths are exempt. When the limiter is nil (rate limiting
// not configured), all requests pass through unchanged.
func (m *RateLimitMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Graceful bypass: when the limiter is nil, pass through.
		if m.limiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Public routes — exempt from rate limiting.
		if m.isPublic(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Extract claims from context. If no claims are present the request
		// is unauthenticated — let other middleware handle rejection.
		claims, ok := ClaimsFromContext(r.Context())
		if !ok || claims == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Resolve the client IP from RemoteAddr (already rewritten by
		// RealIPMiddleware when behind a trusted proxy).
		ip := clientIP(r)

		// Build identifiers for the rate limiter.
		var userID *uuid.UUID
		if uid, err := uuid.Parse(claims.Sub); err == nil {
			userID = &uid
		}

		result, _ := m.limiter.Check(r.Context(), nil, userID, claims.Role, ip)

		// Always set rate limit headers on authenticated responses.
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", result.Limit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", result.ResetAt.Unix()))

		if !result.Allowed {
			retryAfter := result.ResetAt.Unix() - nowUnix()
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isPublic returns true if the path matches a public route (exact or prefix).
func (m *RateLimitMiddleware) isPublic(path string) bool {
	for _, p := range m.publicPaths {
		if path == p {
			return true
		}
	}
	for _, prefix := range m.publicPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// clientIP extracts the IP portion from r.RemoteAddr, stripping any port.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr might be a bare IP without port.
		return r.RemoteAddr
	}
	return host
}

// nowUnix returns the current Unix timestamp. It is a package-level function
// so tests can override it if needed.
var nowUnix = func() int64 {
	return time.Now().Unix()
}
