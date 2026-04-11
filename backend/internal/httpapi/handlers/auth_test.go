package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/block-o/exchangely/backend/internal/httpapi/middleware"
	"github.com/google/uuid"
	"pgregory.net/rapid"
)

// newNilAuthMiddleware creates an auth middleware with nil auth service,
// simulating the auth-disabled mode.
func newNilAuthMiddleware() *middleware.AuthMiddleware {
	return middleware.NewAuthMiddleware(nil)
}

// Minimal no-op repository implementations for handler testing

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

// noopSessionRepo satisfies auth.SessionRepository with no-op implementations.
type noopSessionRepo struct{}

func (r *noopSessionRepo) Create(_ context.Context, _ *auth.Session) error { return nil }
func (r *noopSessionRepo) FindByTokenHash(_ context.Context, _ string) (*auth.Session, error) {
	return nil, nil
}
func (r *noopSessionRepo) Delete(_ context.Context, _ uuid.UUID) error           { return nil }
func (r *noopSessionRepo) DeleteAllForUser(_ context.Context, _ uuid.UUID) error { return nil }
func (r *noopSessionRepo) DeleteExpired(_ context.Context) (int64, error)        { return 0, nil }

// newTestAuthService creates a real auth.Service with no-op repos for handler testing.
func newTestAuthService() *auth.Service {
	cfg := auth.Config{
		AuthMode:           "local,sso",
		JWTSecret:          []byte("test-secret-at-least-16-bytes!!"),
		JWTExpiry:          15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         12,
	}
	return auth.NewService(&noopUserRepo{}, &noopSessionRepo{}, cfg)
}

// TestPropertyRequestBodySizeLimit verifies that for any POST request to
// /api/v1/auth/local/login with a body larger than 1KB, the Auth Service
// rejects the request before processing credentials. Payloads at or below
// 1KB are processed normally (returning 401 since credentials are fake,
// but NOT 413).
func TestPropertyRequestBodySizeLimit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		svc := newTestAuthService()
		handler := NewAuthHandler(svc, "development")

		// Generate a payload size between 500 and 2000 bytes.
		size := rapid.IntRange(500, 2000).Draw(t, "payloadSize")

		// Build a JSON-like payload of the target size. We use a padding field
		// to control the total byte count precisely.
		// Minimal JSON skeleton: {"email":"a@b.c","password":"x","pad":""}
		// That skeleton is 43 bytes. We fill "pad" to reach the target size.
		skeleton := `{"email":"a@b.c","password":"x","pad":"`
		suffix := `"}`
		overhead := len(skeleton) + len(suffix)

		var body []byte
		if size > overhead {
			padLen := size - overhead
			pad := make([]byte, padLen)
			for i := range pad {
				pad[i] = 'A'
			}
			body = make([]byte, 0, size)
			body = append(body, skeleton...)
			body = append(body, pad...)
			body = append(body, suffix...)
		} else {
			// For very small target sizes, just use the skeleton (which is valid JSON).
			body = []byte(`{"email":"a@b.c","password":"x"}`)
			size = len(body)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/local/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.LocalLogin(rr, req)

		if size > 1024 {
			// Payloads exceeding 1KB must be rejected with 413.
			if rr.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("payload %d bytes: expected 413, got %d", size, rr.Code)
			}
		} else {
			// Payloads at or below 1KB must NOT get 413.
			// They'll get 401 (invalid credentials) since the user doesn't exist.
			if rr.Code == http.StatusRequestEntityTooLarge {
				t.Fatalf("payload %d bytes: should not be rejected as too large, got 413", size)
			}
		}
	})
}

// TestLogoutClearsCookieAttributes verifies that calling Logout (even without
// a valid cookie) sets a clear-cookie with the correct attributes.
func TestLogoutClearsCookieAttributes(t *testing.T) {
	svc := newTestAuthService()
	handler := NewAuthHandler(svc, "production")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rr := httptest.NewRecorder()

	handler.Logout(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}

	cookies := rr.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "refresh_token" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("expected refresh_token clear cookie, got none")
	}

	if !found.HttpOnly {
		t.Error("cookie HttpOnly should be true")
	}
	if !found.Secure {
		t.Error("cookie Secure should be true in production env")
	}
	if found.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite: got %v, want Lax", found.SameSite)
	}
	if found.Path != "/api/v1/auth" {
		t.Errorf("cookie Path: got %q, want %q", found.Path, "/api/v1/auth")
	}
	if found.MaxAge != -1 {
		t.Errorf("cookie MaxAge: got %d, want -1 (clear)", found.MaxAge)
	}
}

// TestLogoutCookieNotSecureInDev verifies that in development mode the Secure
// flag is not set on the clear cookie.
func TestLogoutCookieNotSecureInDev(t *testing.T) {
	svc := newTestAuthService()
	handler := NewAuthHandler(svc, "development")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rr := httptest.NewRecorder()

	handler.Logout(rr, req)

	cookies := rr.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "refresh_token" {
			if c.Secure {
				t.Error("cookie Secure should be false in development env")
			}
			return
		}
	}
	t.Fatal("expected refresh_token clear cookie")
}

// TestLocalLoginSetsSecurityHeaders verifies that even a failed LocalLogin
// response includes the required security headers.
func TestLocalLoginSetsSecurityHeaders(t *testing.T) {
	svc := newTestAuthService()
	handler := NewAuthHandler(svc, "production")

	body := `{"email":"test@example.com","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/local/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.LocalLogin(rr, req)

	// We expect 401 since the user doesn't exist, but headers should still be set.
	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Cache-Control":          "no-store",
	}
	for h, want := range headers {
		got := rr.Header().Get(h)
		if got != want {
			t.Errorf("header %s: got %q, want %q", h, got, want)
		}
	}
}

// TestLocalLoginRejectsOversizedBody is a specific example test for the 1KB
// body limit (complements the property test above).
func TestLocalLoginRejectsOversizedBody(t *testing.T) {
	svc := newTestAuthService()
	handler := NewAuthHandler(svc, "production")

	// 2KB payload — well above the 1KB limit.
	bigBody := `{"email":"a@b.c","password":"` + strings.Repeat("X", 2000) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/local/login", strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.LocalLogin(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rr.Code)
	}
}

// TestOAuthRedirectURLConstruction verifies that GoogleLogin produces a redirect
// to Google with the correct scopes, client_id, and redirect_uri.
func TestOAuthRedirectURLConstruction(t *testing.T) {
	cfg := auth.Config{
		AuthMode:           "sso",
		GoogleClientID:     "test-client-id-123",
		GoogleClientSecret: "test-secret",
		GoogleRedirectURI:  "http://localhost:8080/api/v1/auth/google/callback",
		JWTSecret:          []byte("test-secret-at-least-16-bytes!!"),
		JWTExpiry:          15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         12,
	}
	svc := auth.NewService(&noopUserRepo{}, &noopSessionRepo{}, cfg)
	handler := NewAuthHandler(svc, "production")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/login", nil)
	rr := httptest.NewRecorder()

	handler.GoogleLogin(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}

	location := rr.Header().Get("Location")
	if location == "" {
		t.Fatal("expected Location header on redirect")
	}

	// Verify the redirect URL contains the expected OAuth parameters.
	checks := []struct {
		name   string
		substr string
	}{
		{"Google auth endpoint", "accounts.google.com/o/oauth2"},
		{"client_id", "client_id=test-client-id-123"},
		{"redirect_uri", "redirect_uri="},
		{"scope includes openid", "openid"},
		{"scope includes email", "email"},
		{"scope includes profile", "profile"},
		{"response_type", "response_type=code"},
		{"state param", "state="},
	}
	for _, tc := range checks {
		if !strings.Contains(location, tc.substr) {
			t.Errorf("redirect URL missing %s (looking for %q in %q)", tc.name, tc.substr, location)
		}
	}

	// Verify CSRF state cookie is set.
	cookies := rr.Result().Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "oauth_state" {
			stateCookie = c
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("expected oauth_state CSRF cookie")
	}
	if !stateCookie.HttpOnly {
		t.Error("CSRF cookie should be HttpOnly")
	}
	if stateCookie.Path != "/api/v1/auth" {
		t.Errorf("CSRF cookie Path: got %q, want %q", stateCookie.Path, "/api/v1/auth")
	}
}

// TestGracefulDegradationWhenAuthDisabled verifies that when the auth service
// is nil (auth disabled), public endpoints still work through the middleware.
func TestGracefulDegradationWhenAuthDisabled(t *testing.T) {
	// Build a minimal mux with a public endpoint and wrap with auth middleware
	// where authService is nil.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/v1/system/tasks", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"tasks": "list"})
	})

	// Import the middleware with nil auth service.
	authMW := newNilAuthMiddleware()
	wrapped := authMW.Wrap(mux)

	// Public endpoint should work.
	t.Run("public endpoint accessible", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})

	// Admin endpoint should also work when auth is disabled (no gating).
	t.Run("admin endpoint accessible when auth disabled", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/system/tasks", nil)
		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 (auth disabled = no gating), got %d", rr.Code)
		}
	})
}

// TestLocalLoginReturns429OnIPBlocked verifies that the handler returns 429
// with a Retry-After header when the service returns ErrIPBlocked.
func TestLocalLoginReturns429OnIPBlocked(t *testing.T) {
	svc := newTestAuthService()
	handler := NewAuthHandler(svc, "production")

	ip := "10.0.0.1"

	// Exhaust the IP rate limit by recording 20 failures directly on the
	// service's rate limiter. We access it via repeated login attempts with
	// different emails from the same IP.
	for i := 0; i < 20; i++ {
		body := `{"email":"attacker` + strings.Repeat("x", i) + `@evil.com","password":"wrong"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/local/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip
		rr := httptest.NewRecorder()
		handler.LocalLogin(rr, req)
	}

	// The next request from the same IP should be blocked with 429.
	body := `{"email":"legit@example.com","password":"anything"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/local/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip
	rr := httptest.NewRecorder()

	handler.LocalLogin(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}

	retryAfter := rr.Header().Get("Retry-After")
	if retryAfter != "900" {
		t.Errorf("Retry-After header: got %q, want %q", retryAfter, "900")
	}

	// Verify the response body is the generic "too many requests" message.
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "too many requests" {
		t.Errorf("error message: got %q, want %q", resp["error"], "too many requests")
	}
}

// --- Mock repositories for API token handler testing ---

// mockAPITokenRepo is an in-memory APITokenRepository for handler tests.
type mockAPITokenRepo struct {
	mu     sync.Mutex
	byID   map[uuid.UUID]*auth.APIToken
	byHash map[string]*auth.APIToken
}

func newMockAPITokenRepo() *mockAPITokenRepo {
	return &mockAPITokenRepo{
		byID:   make(map[uuid.UUID]*auth.APIToken),
		byHash: make(map[string]*auth.APIToken),
	}
}

func (r *mockAPITokenRepo) Create(_ context.Context, token *auth.APIToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *token
	r.byID[cp.ID] = &cp
	r.byHash[cp.TokenHash] = &cp
	return nil
}

func (r *mockAPITokenRepo) FindByTokenHash(_ context.Context, hash string) (*auth.APIToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byHash[hash]
	if !ok {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

func (r *mockAPITokenRepo) ListByUserID(_ context.Context, userID uuid.UUID) ([]auth.APIToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var tokens []auth.APIToken
	for _, t := range r.byID {
		if t.UserID == userID {
			tokens = append(tokens, *t)
		}
	}
	// Sort by created_at desc.
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].CreatedAt.After(tokens[j].CreatedAt)
	})
	return tokens, nil
}

func (r *mockAPITokenRepo) CountActiveByUserID(_ context.Context, userID uuid.UUID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	count := 0
	for _, t := range r.byID {
		if t.UserID == userID && t.RevokedAt == nil && t.ExpiresAt.After(now) {
			count++
		}
	}
	return count, nil
}

func (r *mockAPITokenRepo) Revoke(_ context.Context, id uuid.UUID, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[id]
	if !ok || t.UserID != userID {
		return auth.ErrTokenNotFound
	}
	if t.RevokedAt != nil {
		return nil
	}
	now := time.Now()
	t.RevokedAt = &now
	return nil
}

func (r *mockAPITokenRepo) UpdateLastUsedAt(_ context.Context, id uuid.UUID, t time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tok, ok := r.byID[id]
	if !ok {
		return nil
	}
	tok.LastUsedAt = &t
	return nil
}

// mockUserRepoForTokens is a minimal UserRepository that stores users by ID.
type mockUserRepoForTokens struct {
	mu   sync.Mutex
	byID map[uuid.UUID]*auth.User
}

func newMockUserRepoForTokens() *mockUserRepoForTokens {
	return &mockUserRepoForTokens{byID: make(map[uuid.UUID]*auth.User)}
}

func (r *mockUserRepoForTokens) FindByID(_ context.Context, id uuid.UUID) (*auth.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}
func (r *mockUserRepoForTokens) FindByEmail(_ context.Context, _ string) (*auth.User, error) {
	return nil, nil
}
func (r *mockUserRepoForTokens) FindByGoogleID(_ context.Context, _ string) (*auth.User, error) {
	return nil, nil
}
func (r *mockUserRepoForTokens) Create(_ context.Context, _ *auth.User) error { return nil }
func (r *mockUserRepoForTokens) Update(_ context.Context, _ *auth.User) error { return nil }
func (r *mockUserRepoForTokens) UpdatePasswordHash(_ context.Context, _ uuid.UUID, _ string, _ bool) error {
	return nil
}

func (r *mockUserRepoForTokens) addUser(u *auth.User) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[u.ID] = u
}

// --- Test helpers ---

// newTestAPITokenService creates an APITokenService backed by in-memory mocks.
func newTestAPITokenService() (*auth.APITokenService, *mockAPITokenRepo, *mockUserRepoForTokens) {
	tokenRepo := newMockAPITokenRepo()
	userRepo := newMockUserRepoForTokens()
	svc := auth.NewAPITokenService(tokenRepo, userRepo, auth.DefaultAPITokenConfig())
	return svc, tokenRepo, userRepo
}

// jwtRequest creates an HTTP request with JWT claims in context (simulating
// auth middleware). Auth method is left unset (defaults to JWT session).
func jwtRequest(method, path string, body *bytes.Buffer, userID uuid.UUID, role string) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, body)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	claims := &auth.Claims{
		Sub:   userID.String(),
		Email: "test@exchangely.io",
		Role:  role,
	}
	ctx := middleware.ContextWithClaims(req.Context(), claims)
	return req.WithContext(ctx)
}

// apiTokenRequest creates an HTTP request with claims AND auth method set to
// "api_token", simulating a request authenticated via API token.
func apiTokenRequest(method, path string, body *bytes.Buffer, userID uuid.UUID, role string) *http.Request {
	req := jwtRequest(method, path, body, userID, role)
	ctx := middleware.ContextWithAuthMethod(req.Context(), "api_token")
	return req.WithContext(ctx)
}

// --- Tests ---

// TestCreateAPIToken_201 verifies that POST /api/v1/auth/api-tokens creates a
// token and returns 201 with the raw token in the response.
func TestCreateAPIToken_201(t *testing.T) {
	svc, _, userRepo := newTestAPITokenService()
	userID := uuid.New()
	userRepo.addUser(&auth.User{ID: userID, Email: "dev@exchangely.io", Role: "user"})

	handler := NewAuthHandler(newTestAuthService(), "development").WithAPITokenService(svc)

	body := bytes.NewBufferString(`{"label":"my-key"}`)
	req := jwtRequest(http.MethodPost, "/api/v1/auth/api-tokens", body, userID, "user")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.CreateAPIToken(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	rawToken, ok := resp["token"].(string)
	if !ok || rawToken == "" {
		t.Fatal("expected non-empty 'token' in response")
	}
	if !strings.HasPrefix(rawToken, "exly_") {
		t.Errorf("raw token should start with exly_, got %q", rawToken[:10])
	}
	if resp["label"] != "my-key" {
		t.Errorf("expected label 'my-key', got %v", resp["label"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("expected non-empty 'id' in response")
	}
}

// TestListAPITokens_200 verifies that GET /api/v1/auth/api-tokens returns the
// user's token list with a 200 status.
func TestListAPITokens_200(t *testing.T) {
	svc, _, userRepo := newTestAPITokenService()
	userID := uuid.New()
	userRepo.addUser(&auth.User{ID: userID, Email: "dev@exchangely.io", Role: "user"})

	handler := NewAuthHandler(newTestAuthService(), "development").WithAPITokenService(svc)

	// Create two tokens first.
	for _, label := range []string{"key-1", "key-2"} {
		body := bytes.NewBufferString(`{"label":"` + label + `"}`)
		req := jwtRequest(http.MethodPost, "/api/v1/auth/api-tokens", body, userID, "user")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.CreateAPIToken(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("setup: expected 201, got %d", w.Code)
		}
	}

	// List tokens.
	req := jwtRequest(http.MethodGet, "/api/v1/auth/api-tokens", nil, userID, "user")
	rr := httptest.NewRecorder()
	handler.ListAPITokens(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected 'data' array in response, got %T", resp["data"])
	}
	if len(data) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(data))
	}

	// Verify each token has expected fields.
	for _, item := range data {
		tok, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("expected token object, got %T", item)
		}
		if tok["id"] == nil || tok["label"] == nil || tok["status"] == nil {
			t.Error("token missing required fields (id, label, status)")
		}
		if tok["status"] != "active" {
			t.Errorf("expected status 'active', got %v", tok["status"])
		}
	}
}

// TestRevokeAPIToken_204 verifies that DELETE /api/v1/auth/api-tokens/{id}
// revokes the token and returns 204.
func TestRevokeAPIToken_204(t *testing.T) {
	svc, _, userRepo := newTestAPITokenService()
	userID := uuid.New()
	userRepo.addUser(&auth.User{ID: userID, Email: "dev@exchangely.io", Role: "user"})

	handler := NewAuthHandler(newTestAuthService(), "development").WithAPITokenService(svc)

	// Create a token.
	body := bytes.NewBufferString(`{"label":"to-revoke"}`)
	req := jwtRequest(http.MethodPost, "/api/v1/auth/api-tokens", body, userID, "user")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.CreateAPIToken(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d", w.Code)
	}

	var createResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	tokenID := createResp["id"].(string)

	// Revoke the token.
	req = jwtRequest(http.MethodDelete, "/api/v1/auth/api-tokens/"+tokenID, nil, userID, "user")
	rr := httptest.NewRecorder()
	handler.RevokeAPIToken(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestCreateAPIToken_409_LimitReached verifies that creating a token when the
// user already has 5 active tokens returns 409 Conflict.
func TestCreateAPIToken_409_LimitReached(t *testing.T) {
	svc, _, userRepo := newTestAPITokenService()
	userID := uuid.New()
	userRepo.addUser(&auth.User{ID: userID, Email: "dev@exchangely.io", Role: "user"})

	handler := NewAuthHandler(newTestAuthService(), "development").WithAPITokenService(svc)

	// Create 5 tokens (the maximum).
	for i := 0; i < 5; i++ {
		body := bytes.NewBufferString(`{"label":"key-` + strings.Repeat("x", i+1) + `"}`)
		req := jwtRequest(http.MethodPost, "/api/v1/auth/api-tokens", body, userID, "user")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.CreateAPIToken(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("setup token %d: expected 201, got %d", i+1, w.Code)
		}
	}

	// The 6th token should be rejected with 409.
	body := bytes.NewBufferString(`{"label":"one-too-many"}`)
	req := jwtRequest(http.MethodPost, "/api/v1/auth/api-tokens", body, userID, "user")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.CreateAPIToken(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "token limit reached" {
		t.Errorf("expected error 'token limit reached', got %q", resp["error"])
	}
}

// TestRevokeAPIToken_404_WrongUser verifies that revoking another user's token
// returns 404 Not Found.
func TestRevokeAPIToken_404_WrongUser(t *testing.T) {
	svc, _, userRepo := newTestAPITokenService()
	userA := uuid.New()
	userB := uuid.New()
	userRepo.addUser(&auth.User{ID: userA, Email: "a@exchangely.io", Role: "user"})
	userRepo.addUser(&auth.User{ID: userB, Email: "b@exchangely.io", Role: "user"})

	handler := NewAuthHandler(newTestAuthService(), "development").WithAPITokenService(svc)

	// User A creates a token.
	body := bytes.NewBufferString(`{"label":"a-key"}`)
	req := jwtRequest(http.MethodPost, "/api/v1/auth/api-tokens", body, userA, "user")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.CreateAPIToken(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d", w.Code)
	}

	var createResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	tokenID := createResp["id"].(string)

	// User B tries to revoke User A's token.
	req = jwtRequest(http.MethodDelete, "/api/v1/auth/api-tokens/"+tokenID, nil, userB, "user")
	rr := httptest.NewRecorder()
	handler.RevokeAPIToken(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "not found" {
		t.Errorf("expected error 'not found', got %q", resp["error"])
	}
}

// TestAPITokenEndpoints_401_APITokenAuth verifies that using an API token
// (instead of JWT) on management endpoints returns 401 Unauthorized.
func TestAPITokenEndpoints_401_APITokenAuth(t *testing.T) {
	svc, _, userRepo := newTestAPITokenService()
	userID := uuid.New()
	userRepo.addUser(&auth.User{ID: userID, Email: "dev@exchangely.io", Role: "user"})

	handler := NewAuthHandler(newTestAuthService(), "development").WithAPITokenService(svc)

	tests := []struct {
		name   string
		method string
		path   string
		body   *bytes.Buffer
		fn     func(http.ResponseWriter, *http.Request)
	}{
		{
			name:   "POST create token via API token auth",
			method: http.MethodPost,
			path:   "/api/v1/auth/api-tokens",
			body:   bytes.NewBufferString(`{"label":"sneaky"}`),
			fn:     handler.CreateAPIToken,
		},
		{
			name:   "GET list tokens via API token auth",
			method: http.MethodGet,
			path:   "/api/v1/auth/api-tokens",
			body:   nil,
			fn:     handler.ListAPITokens,
		},
		{
			name:   "DELETE revoke token via API token auth",
			method: http.MethodDelete,
			path:   "/api/v1/auth/api-tokens/" + uuid.New().String(),
			body:   nil,
			fn:     handler.RevokeAPIToken,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := apiTokenRequest(tc.method, tc.path, tc.body, userID, "user")
			if tc.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			rr := httptest.NewRecorder()
			tc.fn(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d; body: %s", rr.Code, rr.Body.String())
			}

			var resp map[string]string
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if resp["error"] != "unauthorized" {
				t.Errorf("expected error 'unauthorized', got %q", resp["error"])
			}
		})
	}
}
