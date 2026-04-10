package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// =============================================================================
// Minimal no-op repository implementations for handler testing
// =============================================================================

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
		JWTSecret:          []byte("test-secret-at-least-16-bytes!!"),
		JWTExpiry:          15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         12,
	}
	return auth.NewService(&noopUserRepo{}, &noopSessionRepo{}, cfg)
}

// =============================================================================
// Property 17: Request body size limit enforcement
// =============================================================================

// TestPropertyRequestBodySizeLimit verifies Property 17.
//
// For any POST request to /api/v1/auth/local/login with a body larger than 1KB,
// the Auth Service SHALL reject the request before processing credentials.
// Payloads at or below 1KB SHALL be processed normally (returning 401 since
// credentials are fake, but NOT 413).
//
// **Validates: Requirements 12.8**
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

// =============================================================================
// Unit Tests for Task 7.6: Auth handler cookie attributes, security headers,
// OAuth redirect URL, and graceful degradation.
// Requirements: 4.3, 12.7
// =============================================================================

// TestAuthMethodsReturnsSecurityHeaders verifies that the AuthMethods endpoint
// (public, no auth needed) sets the required security headers.
// Validates: Requirement 12.7
func TestAuthMethodsReturnsSecurityHeaders(t *testing.T) {
	svc := newTestAuthService()
	handler := NewAuthHandler(svc, "production")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/methods", nil)
	rr := httptest.NewRecorder()

	handler.AuthMethods(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	tests := []struct {
		header string
		want   string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Cache-Control", "no-store"},
	}
	for _, tc := range tests {
		got := rr.Header().Get(tc.header)
		if got != tc.want {
			t.Errorf("header %s: got %q, want %q", tc.header, got, tc.want)
		}
	}
}

// TestLogoutClearsCookieAttributes verifies that calling Logout (even without
// a valid cookie) sets a clear-cookie with the correct attributes.
// Validates: Requirement 4.3
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
// Validates: Requirement 12.7
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
// body limit (complements Property 17).
// Validates: Requirement 12.8
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

// TestAuthMethodsResponseBody verifies the JSON body of the AuthMethods endpoint.
func TestAuthMethodsResponseBody(t *testing.T) {
	svc := newTestAuthService()
	handler := NewAuthHandler(svc, "production")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/methods", nil)
	rr := httptest.NewRecorder()

	handler.AuthMethods(rr, req)

	var resp auth.AuthMethodsResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// With the test config (no Google creds, no admin email), both should be false.
	if resp.Google {
		t.Error("expected Google=false with empty client ID/secret")
	}
	if resp.Local {
		t.Error("expected Local=false with empty admin email")
	}
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
