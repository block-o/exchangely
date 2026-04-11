package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/google/uuid"
	"pgregory.net/rapid"
)

// testJWTSecret is the shared secret used for both token issuance and middleware validation.
var testJWTSecret = []byte("test-secret-at-least-16-bytes!!")

// newTestAuthService creates a real auth.Service with minimal mock repos for middleware testing.
// The JWT secret matches testJWTSecret so tokens issued via auth.IssueAccessToken are accepted.
func newTestAuthService() *auth.Service {
	cfg := auth.Config{
		AuthMode:           "local,sso",
		JWTSecret:          testJWTSecret,
		JWTExpiry:          15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         12,
	}
	return auth.NewService(&noopUserRepo{}, &noopSessionRepo{}, cfg)
}

// okHandler is a simple handler that always returns 200 OK.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// =============================================================================
// Property 8: Role-based access control for system endpoints
// =============================================================================

// TestPropertyRoleBasedAccessControl verifies Property 8.
//
// For any authenticated user and any /api/v1/system/* endpoint, the Auth
// Middleware SHALL allow the request if and only if the user's role is "admin".
// Users with role "user" SHALL receive HTTP 403 Forbidden.
//
// **Validates: Requirements 6.1, 6.2**
func TestPropertyRoleBasedAccessControl(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		svc := newTestAuthService()
		mw := NewAuthMiddleware(svc)
		handler := mw.Wrap(okHandler)

		// Generate a random system endpoint path.
		suffix := rapid.StringMatching(`[a-z][a-z0-9\-]{0,20}`).Draw(t, "endpointSuffix")
		path := fmt.Sprintf("/api/v1/system/%s", suffix)

		// Generate a random role.
		role := rapid.SampledFrom([]string{"admin", "user"}).Draw(t, "role")

		// Create a user with the generated role and issue a JWT.
		user := auth.User{
			ID:    uuid.New(),
			Email: rapid.StringMatching(`[a-z]{1,8}@[a-z]{1,6}\.[a-z]{2,4}`).Draw(t, "email"),
			Role:  role,
		}
		token, err := auth.IssueAccessToken(user, testJWTSecret, 15*time.Minute)
		if err != nil {
			t.Fatalf("IssueAccessToken failed: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if role == "admin" {
			if rr.Code != http.StatusOK {
				t.Fatalf("admin role on %s: expected 200, got %d", path, rr.Code)
			}
		} else {
			if rr.Code != http.StatusForbidden {
				t.Fatalf("user role on %s: expected 403, got %d", path, rr.Code)
			}
		}
	})
}

// =============================================================================
// Public routes return 200 without any token
// =============================================================================

// TestPropertyPublicRoutesNoTokenRequired verifies that public routes return 200
// without any authentication token.
func TestPropertyPublicRoutesNoTokenRequired(t *testing.T) {
	svc := newTestAuthService()
	mw := NewAuthMiddleware(svc)
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
		"/api/v1/auth/google/callback",
		"/api/v1/auth/local/login",
		"/swagger",
		"/swagger/openapi.yaml",
	}

	for _, p := range append(publicPaths, publicPrefixPaths...) {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("public route %s without token: expected 200, got %d", p, rr.Code)
		}
	}
}

// =============================================================================
// Protected routes return 401 without any token
// =============================================================================

// TestPropertyProtectedRoutesRequireToken verifies that protected (non-public)
// routes return 401 when no Bearer token is provided.
func TestPropertyProtectedRoutesRequireToken(t *testing.T) {
	svc := newTestAuthService()
	mw := NewAuthMiddleware(svc)
	handler := mw.Wrap(okHandler)

	protectedPaths := []string{
		"/api/v1/system/sync-status",
		"/api/v1/system/tasks",
		"/api/v1/some/other/protected",
		"/api/v1/auth/me",
		"/api/v1/auth/local/change-password",
	}

	for _, p := range protectedPaths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("protected route %s without token: expected 401, got %d", p, rr.Code)
		}
	}
}

// =============================================================================
// Minimal no-op repository implementations for middleware testing
// =============================================================================

// noopUserRepo satisfies auth.UserRepository with no-op implementations.
// The middleware only needs auth.Service.ValidateAccessToken which doesn't
// touch repositories.
type noopUserRepo struct{}

func (r *noopUserRepo) FindByID(_ context.Context, _ uuid.UUID) (*auth.User, error) {
	return nil, nil
}
func (r *noopUserRepo) FindByEmail(_ context.Context, _ string) (*auth.User, error) {
	return nil, nil
}
func (r *noopUserRepo) FindByGoogleID(_ context.Context, _ string) (*auth.User, error) {
	return nil, nil
}
func (r *noopUserRepo) Create(_ context.Context, _ *auth.User) error { return nil }
func (r *noopUserRepo) Update(_ context.Context, _ *auth.User) error { return nil }
func (r *noopUserRepo) UpdatePasswordHash(_ context.Context, _ uuid.UUID, _ string, _ bool) error {
	return nil
}

// noopSessionRepo satisfies auth.SessionRepository with no-op implementations.
type noopSessionRepo struct{}

func (r *noopSessionRepo) Create(_ context.Context, _ *auth.Session) error { return nil }
func (r *noopSessionRepo) FindByTokenHash(_ context.Context, _ string) (*auth.Session, error) {
	return nil, nil
}
func (r *noopSessionRepo) Delete(_ context.Context, _ uuid.UUID) error           { return nil }
func (r *noopSessionRepo) DeleteAllForUser(_ context.Context, _ uuid.UUID) error { return nil }
func (r *noopSessionRepo) DeleteExpired(_ context.Context) (int64, error)        { return 0, nil }
