package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/block-o/exchangely/backend/internal/auth"
)

// contextKey is an unexported type for context keys in this package,
// preventing collisions with keys defined in other packages.
type contextKey string

const claimsKey contextKey = "auth_claims"

// ClaimsFromContext extracts the auth claims from the request context.
// Returns nil and false if no claims are present.
func ClaimsFromContext(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(*auth.Claims)
	return claims, ok
}

// ContextWithClaims returns a new context with the given claims attached.
// This is exported so that handler-level tests in other packages can simulate
// an authenticated request without routing through the full middleware chain.
func ContextWithClaims(ctx context.Context, claims *auth.Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// AuthMiddleware intercepts HTTP requests to enforce authentication and
// role-based access control. Public routes are whitelisted; admin routes
// require role=admin; everything else requires a valid JWT.
type AuthMiddleware struct {
	authService    *auth.Service
	publicPaths    []string // exact-match public routes
	publicPrefixes []string // prefix-match public routes
	adminPrefixes  []string // prefix-match admin-only routes
}

// NewAuthMiddleware creates an AuthMiddleware with the default route
// classification from the design document.
func NewAuthMiddleware(authService *auth.Service) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
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
		adminPrefixes: []string{
			"/api/v1/system/",
		},
	}
}

// Wrap returns an http.Handler that enforces authentication and authorization
// rules before delegating to the next handler.
func (m *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Graceful bypass: when auth is disabled (no service or no JWT secret),
		// pass through all requests to preserve backward compatibility.
		if m.authService == nil {
			next.ServeHTTP(w, r)
			return
		}

		path := r.URL.Path

		// Public routes — no token required.
		if m.isPublic(path) {
			next.ServeHTTP(w, r)
			return
		}

		// If claims are already in context (e.g. set by APITokenMiddleware),
		// skip JWT validation and proceed directly to admin-only checks.
		if existing, ok := ClaimsFromContext(r.Context()); ok && existing != nil {
			if m.isAdminOnly(path) {
				if existing.Role != "admin" {
					writeJSONError(w, http.StatusForbidden, "forbidden")
					return
				}
			}
			// Mark as JWT auth method if not already set (preserves api_token method).
			if _, methodSet := AuthMethodFromContext(r.Context()); !methodSet {
				ctx := context.WithValue(r.Context(), authMethodKey, "jwt")
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// Extract Bearer token from Authorization header.
		// Fall back to ?token= query parameter for SSE/EventSource connections
		// which cannot set custom headers.
		token := extractBearerToken(r)
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token == "" {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// Validate the JWT.
		claims, err := m.authService.ValidateAccessToken(token)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// Admin-only routes — check role.
		if m.isAdminOnly(path) {
			if claims.Role != "admin" {
				writeJSONError(w, http.StatusForbidden, "forbidden")
				return
			}
		}

		// Attach claims and auth method to context and proceed.
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		ctx = context.WithValue(ctx, authMethodKey, "jwt")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// isPublic returns true if the path matches a public route (exact or prefix).
func (m *AuthMiddleware) isPublic(path string) bool {
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

// isAdminOnly returns true if the path matches an admin-only prefix.
func (m *AuthMiddleware) isAdminOnly(path string) bool {
	for _, prefix := range m.adminPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// extractBearerToken pulls the token from the "Authorization: Bearer <token>" header.
func extractBearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// writeJSONError writes a JSON error response with the given status code.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
