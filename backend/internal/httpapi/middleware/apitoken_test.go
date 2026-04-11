package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/google/uuid"
	"pgregory.net/rapid"
)

// newTestAPITokenMiddleware creates an APITokenMiddleware backed by a real
// APITokenService with no-op repositories. This ensures the middleware is
// non-nil (so it doesn't gracefully bypass everything) while keeping the
// test free of database dependencies.
func newTestAPITokenMiddleware() *APITokenMiddleware {
	tokenSvc := auth.NewAPITokenService(
		&noopAPITokenRepo{},
		&noopUserRepo{},
		auth.DefaultAPITokenConfig(),
	)
	return NewAPITokenMiddleware(tokenSvc)
}

// TestPropertyPublicPathExemption verifies that for any path in the configured
// public paths list, the API token middleware passes the request through without
// requiring authentication. For any non-public path without a valid token, the
// middleware passes through to the next handler (JWT middleware). However, when
// a non-public path carries an invalid exly_ token, the middleware returns 401.
func TestPropertyPublicPathExemption(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mw := newTestAPITokenMiddleware()
		handler := mw.Wrap(okHandler)

		// Public exact-match paths
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

		// Public prefix-match paths (generate random suffixes)
		prefixBases := []string{
			"/api/v1/ticker/",
			"/api/v1/historical/",
			"/api/v1/tickers/stream",
			"/api/v1/news/stream",
			"/api/v1/auth/google/",
			"/api/v1/auth/local/login",
			"/swagger",
		}

		// Pick a random public exact path and verify it passes through.
		exactIdx := rapid.IntRange(0, len(publicPaths)-1).Draw(t, "exactPathIdx")
		exactPath := publicPaths[exactIdx]

		req := httptest.NewRequest(http.MethodGet, exactPath, nil)
		// Deliberately set an invalid exly_ token — public paths should still pass.
		req.Header.Set("Authorization", "Bearer exly_invalid_token_abc123")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("public exact path %s: expected 200, got %d", exactPath, rr.Code)
		}

		// Pick a random public prefix and append a random suffix.
		prefixIdx := rapid.IntRange(0, len(prefixBases)-1).Draw(t, "prefixIdx")
		suffix := rapid.StringMatching(`[a-zA-Z0-9\-]{1,20}`).Draw(t, "suffix")
		prefixPath := prefixBases[prefixIdx] + suffix

		req = httptest.NewRequest(http.MethodGet, prefixPath, nil)
		req.Header.Set("Authorization", "Bearer exly_invalid_token_abc123")
		rr = httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("public prefix path %s: expected 200, got %d", prefixPath, rr.Code)
		}

		// Non-public path with invalid exly_ token should get 401
		nonPublicSuffix := rapid.StringMatching(`[a-z]{1,15}`).Draw(t, "nonPublicSuffix")
		nonPublicPath := fmt.Sprintf("/api/v1/private/%s", nonPublicSuffix)

		req = httptest.NewRequest(http.MethodGet, nonPublicPath, nil)
		req.Header.Set("Authorization", "Bearer exly_bogus_token_xyz789")
		rr = httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("non-public path %s with invalid exly_ token: expected 401, got %d", nonPublicPath, rr.Code)
		}
	})
}

// No-op repository implementations for middleware-level testing

// noopUserRepo satisfies auth.UserRepository with no-op implementations.
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
func (r *noopUserRepo) ListWithFilters(_ context.Context, _ string, _ string, _ string, _ int, _ int) ([]auth.User, int, error) {
	return []auth.User{}, 0, nil
}
func (r *noopUserRepo) UpdateRole(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (r *noopUserRepo) UpdateDisabled(_ context.Context, _ uuid.UUID, _ bool) error {
	return nil
}
func (r *noopUserRepo) SetMustChangePassword(_ context.Context, _ uuid.UUID, _ bool) error {
	return nil
}

// noopAPITokenRepo satisfies auth.APITokenRepository with no-op implementations.
// FindByTokenHash always returns nil (token not found), which causes ValidateToken
// to return ErrTokenUnauthorized for any token — exactly what we need to verify
// that non-public paths reject invalid tokens.
type noopAPITokenRepo struct{}

func (r *noopAPITokenRepo) Create(_ context.Context, _ *auth.APIToken) error { return nil }
func (r *noopAPITokenRepo) FindByTokenHash(_ context.Context, _ string) (*auth.APIToken, error) {
	return nil, nil
}
func (r *noopAPITokenRepo) ListByUserID(_ context.Context, _ uuid.UUID) ([]auth.APIToken, error) {
	return nil, nil
}
func (r *noopAPITokenRepo) CountActiveByUserID(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}
func (r *noopAPITokenRepo) Revoke(_ context.Context, _ uuid.UUID, _ uuid.UUID) error { return nil }
func (r *noopAPITokenRepo) UpdateLastUsedAt(_ context.Context, _ uuid.UUID, _ time.Time) error {
	return nil
}

// Unit tests for API token middleware

// validatingAPITokenRepo is a mock that returns a pre-configured token and
// user from FindByTokenHash when the hash matches. This lets us test the
// middleware's happy path without a real database.
type validatingAPITokenRepo struct {
	noopAPITokenRepo
	token *auth.APIToken
}

func (r *validatingAPITokenRepo) FindByTokenHash(_ context.Context, hash string) (*auth.APIToken, error) {
	if r.token != nil && r.token.TokenHash == hash {
		return r.token, nil
	}
	return nil, nil
}

// validatingUserRepo returns a pre-configured user when FindByID matches.
type validatingUserRepo struct {
	noopUserRepo
	user *auth.User
}

func (r *validatingUserRepo) FindByID(_ context.Context, id uuid.UUID) (*auth.User, error) {
	if r.user != nil && r.user.ID == id {
		return r.user, nil
	}
	return nil, nil
}

// sha256Hex computes the hex-encoded SHA-256 hash of s, matching the auth
// package's unexported hashToken function.
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// newValidatingAPITokenMiddleware creates an APITokenMiddleware backed by mock
// repos that recognise a single valid token and user.
func newValidatingAPITokenMiddleware(token *auth.APIToken, user *auth.User) *APITokenMiddleware {
	tokenSvc := auth.NewAPITokenService(
		&validatingAPITokenRepo{token: token},
		&validatingUserRepo{user: user},
		auth.DefaultAPITokenConfig(),
	)
	return NewAPITokenMiddleware(tokenSvc)
}

// testTokenAndUser returns a valid APIToken and User pair suitable for unit
// tests. The token hash corresponds to the raw token "exly_test_valid_token".
func testTokenAndUser() (rawToken string, token *auth.APIToken, user *auth.User) {
	rawToken = "exly_test_valid_token"
	userID := uuid.New()
	user = &auth.User{
		ID:    userID,
		Email: "dev@exchangely.io",
		Role:  "user",
	}
	token = &auth.APIToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: sha256Hex(rawToken),
		Label:     "test-key",
		Prefix:    rawToken[:8],
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(90 * 24 * time.Hour),
	}
	return rawToken, token, user
}

// TestAPITokenMiddleware_XAPIKeyHeader verifies that the middleware extracts
// tokens from the X-API-Key header and authenticates the request.
func TestAPITokenMiddleware_XAPIKeyHeader(t *testing.T) {
	rawToken, token, user := testTokenAndUser()
	mw := newValidatingAPITokenMiddleware(token, user)

	// Handler that checks claims were attached.
	handler := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok || claims == nil {
			t.Fatal("expected claims in context")
		}
		if claims.Email != user.Email {
			t.Fatalf("expected email %s, got %s", user.Email, claims.Email)
		}
		method, ok := AuthMethodFromContext(r.Context())
		if !ok || method != "api_token" {
			t.Fatalf("expected auth_method api_token, got %q", method)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("X-API-Key", rawToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("X-API-Key valid token: expected 200, got %d", rr.Code)
	}
}

// TestAPITokenMiddleware_BearerExlyPrefix verifies that Bearer tokens with the
// exly_ prefix are routed through API token validation instead of JWT parsing.
func TestAPITokenMiddleware_BearerExlyPrefix(t *testing.T) {
	rawToken, token, user := testTokenAndUser()
	mw := newValidatingAPITokenMiddleware(token, user)

	handler := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok || claims == nil {
			t.Fatal("expected claims in context from exly_ Bearer token")
		}
		if claims.Role != user.Role {
			t.Fatalf("expected role %s, got %s", user.Role, claims.Role)
		}
		method, _ := AuthMethodFromContext(r.Context())
		if method != "api_token" {
			t.Fatalf("expected auth_method api_token, got %q", method)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Bearer exly_ valid token: expected 200, got %d", rr.Code)
	}
}

// TestAPITokenMiddleware_PassThroughToJWT verifies that requests without any
// API token (no exly_ Bearer, no X-API-Key) are passed through to the next
// handler (typically the JWT middleware) without interference.
func TestAPITokenMiddleware_PassThroughToJWT(t *testing.T) {
	mw := newTestAPITokenMiddleware()

	// The next handler simulates the JWT middleware returning 200.
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		// No claims should have been set by the API token middleware.
		_, ok := ClaimsFromContext(r.Context())
		if ok {
			t.Fatal("API token middleware should not set claims when no API token is present")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.Wrap(next)

	t.Run("no auth header at all", func(t *testing.T) {
		nextCalled = false
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if !nextCalled {
			t.Fatal("expected next handler to be called when no API token present")
		}
		if rr.Code != http.StatusOK {
			t.Fatalf("pass-through: expected 200, got %d", rr.Code)
		}
	})

	t.Run("Bearer JWT token without exly_ prefix", func(t *testing.T) {
		nextCalled = false
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiJ9.some.jwt")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if !nextCalled {
			t.Fatal("expected next handler to be called for non-exly_ Bearer token")
		}
		if rr.Code != http.StatusOK {
			t.Fatalf("JWT pass-through: expected 200, got %d", rr.Code)
		}
	})
}

// TestAPITokenMiddleware_InvalidToken401 verifies that an invalid API token
// (one that doesn't match any stored hash) returns 401 Unauthorized.
func TestAPITokenMiddleware_InvalidToken401(t *testing.T) {
	mw := newTestAPITokenMiddleware() // noopAPITokenRepo returns nil for all lookups

	handler := mw.Wrap(okHandler)

	t.Run("invalid exly_ Bearer token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer exly_this_token_does_not_exist")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("invalid Bearer exly_ token: expected 401, got %d", rr.Code)
		}
	})

	t.Run("invalid X-API-Key token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("X-API-Key", "some_random_invalid_key")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("invalid X-API-Key token: expected 401, got %d", rr.Code)
		}
	})
}

// TestPropertyDisabledUserAPITokenRejection verifies that disabled users are
// rejected by the API token middleware with HTTP 401, and claims are not
// attached to the request context.
func TestPropertyDisabledUserAPITokenRejection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random user with a valid API token.
		userID := uuid.New()
		email := rapid.StringMatching(`[a-z]{5,10}@example\.com`).Draw(t, "email")
		role := rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "role")

		// Create a user that is disabled.
		user := &auth.User{
			ID:       userID,
			Email:    email,
			Role:     role,
			Disabled: true, // Account is disabled
		}

		// Generate a valid API token.
		rawToken := "exly_" + rapid.StringN(32, 32, -1).Draw(t, "token")
		tokenHash := sha256.Sum256([]byte(rawToken))
		tokenHashStr := hex.EncodeToString(tokenHash[:])

		token := &auth.APIToken{
			ID:        uuid.New(),
			UserID:    userID,
			TokenHash: tokenHashStr,
			ExpiresAt: time.Now().Add(24 * time.Hour),
			RevokedAt: nil,
		}

		// Create middleware with validating repos that return the token and user.
		tokenRepo := &validatingAPITokenRepo{token: token}
		userRepo := &validatingUserRepo{user: user}
		tokenSvc := auth.NewAPITokenService(tokenRepo, userRepo, auth.DefaultAPITokenConfig())
		mw := NewAPITokenMiddleware(tokenSvc)

		// Create a handler that checks if claims were attached.
		claimsAttached := false
		handler := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := ClaimsFromContext(r.Context()); ok {
				claimsAttached = true
			}
			w.WriteHeader(http.StatusOK)
		}))

		// Make a request to a protected endpoint with the valid token.
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+rawToken)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		// Verify that the request was rejected with 401.
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for disabled user, got %d", rr.Code)
		}

		// Verify that claims were not attached to the context.
		if claimsAttached {
			t.Fatalf("claims should not be attached for disabled user")
		}
	})
}
