package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRealIPMiddleware_NoTrustedProxies(t *testing.T) {
	m := NewRealIPMiddleware(nil)
	if m.Enabled() {
		t.Fatal("expected Enabled() == false with no trusted proxies")
	}

	var captured string
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.RemoteAddr
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if captured != "1.2.3.4:5678" {
		t.Fatalf("expected RemoteAddr unchanged, got %q", captured)
	}
}

func TestRealIPMiddleware_TrustedProxy_XForwardedFor(t *testing.T) {
	m := NewRealIPMiddleware([]string{"10.0.0.0/8"})
	if !m.Enabled() {
		t.Fatal("expected Enabled() == true")
	}

	var captured string
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.RemoteAddr
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:5678"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.1")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if captured != "203.0.113.50" {
		t.Fatalf("expected RemoteAddr=203.0.113.50, got %q", captured)
	}
}

func TestRealIPMiddleware_TrustedProxy_XRealIP(t *testing.T) {
	m := NewRealIPMiddleware([]string{"172.16.0.1"})

	var captured string
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.RemoteAddr
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "172.16.0.1:9999"
	req.Header.Set("X-Real-IP", "198.51.100.42")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if captured != "198.51.100.42" {
		t.Fatalf("expected RemoteAddr=198.51.100.42, got %q", captured)
	}
}

func TestRealIPMiddleware_UntrustedProxy_IgnoresHeaders(t *testing.T) {
	m := NewRealIPMiddleware([]string{"10.0.0.0/8"})

	var captured string
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.RemoteAddr
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:1234" // not in 10.0.0.0/8
	req.Header.Set("X-Forwarded-For", "1.1.1.1")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if captured != "192.168.1.1:1234" {
		t.Fatalf("expected RemoteAddr unchanged for untrusted peer, got %q", captured)
	}
}

func TestRealIPMiddleware_InvalidXFFValue(t *testing.T) {
	m := NewRealIPMiddleware([]string{"10.0.0.0/8"})

	var captured string
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.RemoteAddr
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:5678"
	req.Header.Set("X-Forwarded-For", "not-an-ip, 10.0.0.1")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// Invalid XFF, no X-Real-IP → RemoteAddr stays unchanged.
	if captured != "10.0.0.1:5678" {
		t.Fatalf("expected RemoteAddr unchanged for invalid XFF, got %q", captured)
	}
}

func TestRealIPMiddleware_XForwardedForPreferredOverXRealIP(t *testing.T) {
	m := NewRealIPMiddleware([]string{"10.0.0.0/8"})

	var captured string
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.RemoteAddr
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:5678"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	req.Header.Set("X-Real-IP", "198.51.100.42")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if captured != "203.0.113.50" {
		t.Fatalf("expected X-Forwarded-For to take precedence, got %q", captured)
	}
}

func TestRealIPMiddleware_IPv6(t *testing.T) {
	m := NewRealIPMiddleware([]string{"::1"})

	var captured string
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.RemoteAddr
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[::1]:5678"
	req.Header.Set("X-Forwarded-For", "2001:db8::1")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if captured != "2001:db8::1" {
		t.Fatalf("expected RemoteAddr=2001:db8::1, got %q", captured)
	}
}

func TestRealIPMiddleware_InvalidEntries(t *testing.T) {
	m := NewRealIPMiddleware([]string{"not-valid", "", "10.0.0.1"})
	if !m.Enabled() {
		t.Fatal("expected Enabled() == true with at least one valid entry")
	}
	if len(m.trustedIPs) != 1 {
		t.Fatalf("expected 1 trusted IP, got %d", len(m.trustedIPs))
	}
}
