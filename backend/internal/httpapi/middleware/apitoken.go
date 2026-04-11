package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/golang-jwt/jwt/v5"
)

// authMethodKey is an unexported context key that records how the request was
// authenticated ("jwt" or "api_token"). Token management endpoints use this to
// reject API-token-based auth (Requirement 12.4).
const authMethodKey contextKey = "auth_method"

// AuthMethodFromContext returns the authentication method used for the current
// request ("jwt" or "api_token"). Returns empty string and false if not set.
func AuthMethodFromContext(ctx context.Context) (string, bool) {
	method, ok := ctx.Value(authMethodKey).(string)
	return method, ok
}

// ContextWithAuthMethod returns a new context with the given auth method
// attached. This is exported so that handler-level tests in other packages can
// simulate API-token or JWT authentication without the full middleware chain.
func ContextWithAuthMethod(ctx context.Context, method string) context.Context {
	return context.WithValue(ctx, authMethodKey, method)
}

// APITokenMiddleware validates API tokens (exly_-prefixed Bearer tokens or
// X-API-Key header) and attaches user identity to the request context. It runs
// before the JWT auth middleware so that API-token requests bypass JWT parsing.
type APITokenMiddleware struct {
	tokenService   *auth.APITokenService
	publicPaths    []string
	publicPrefixes []string
}

// NewAPITokenMiddleware creates an APITokenMiddleware with the same public
// route lists as AuthMiddleware.
func NewAPITokenMiddleware(tokenService *auth.APITokenService) *APITokenMiddleware {
	return &APITokenMiddleware{
		tokenService: tokenService,
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

// Wrap returns an http.Handler that checks for API tokens before delegating to
// the next handler (typically the JWT auth middleware).
func (m *APITokenMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Graceful bypass: when the token service is nil (API tokens not
		// configured), pass through all requests unchanged.
		if m.tokenService == nil {
			next.ServeHTTP(w, r)
			return
		}

		path := r.URL.Path

		// Public routes — no token check needed.
		if m.isPublic(path) {
			next.ServeHTTP(w, r)
			return
		}

		// Try to extract an API token from the request.
		rawToken := m.extractAPIToken(r)
		if rawToken == "" {
			// No API token found — pass through to JWT middleware.
			next.ServeHTTP(w, r)
			return
		}

		// Validate the API token.
		token, user, err := m.tokenService.ValidateToken(r.Context(), rawToken)
		if err != nil {
			slog.Info("auth event",
				"event", "api_token_rejected",
				"error", err.Error(),
			)
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// Build Claims from the validated token + user so downstream handlers
		// see the same context shape as JWT-authenticated requests.
		claims := &auth.Claims{
			Sub:   user.ID.String(),
			Email: user.Email,
			Role:  user.Role,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: user.ID.String(),
			},
		}

		// Attach claims and auth method to context.
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		ctx = context.WithValue(ctx, authMethodKey, "api_token")

		// Update last_used_at asynchronously (non-blocking).
		m.tokenService.TouchLastUsed(r.Context(), token.ID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractAPIToken checks for an API token in the Authorization Bearer header
// (exly_ prefix) or the X-API-Key header. Returns empty string if no API token
// is found.
func (m *APITokenMiddleware) extractAPIToken(r *http.Request) string {
	// Check Authorization: Bearer <token> for exly_ prefix.
	bearer := extractBearerToken(r)
	if bearer != "" && strings.HasPrefix(bearer, "exly_") {
		return bearer
	}

	// Check X-API-Key header.
	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "" {
		return apiKey
	}

	return ""
}

// isPublic returns true if the path matches a public route (exact or prefix).
func (m *APITokenMiddleware) isPublic(path string) bool {
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
