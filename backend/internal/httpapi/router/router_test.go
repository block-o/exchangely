package router

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithCORSAllowsConfiguredOriginOnPreflight(t *testing.T) {
	handler := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), []string{"http://localhost:5173"})

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("unexpected allow origin header: %q", response.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestWithCORSRejectsUnknownOriginOnPreflight(t *testing.T) {
	handler := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), []string{"http://localhost:5173"})

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	request.Header.Set("Origin", "https://example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

func TestWriteErrorMapsNoRowsToNotFound(t *testing.T) {
	response := httptest.NewRecorder()

	writeError(response, sql.ErrNoRows)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.Code)
	}
}

// TestWithCORSAllowsAllRequiredMethods verifies that the CORS middleware
// includes every HTTP method the app uses: GET (reads), POST (create/auth),
// PATCH (admin user updates), DELETE (token revocation), and OPTIONS (preflight).
func TestWithCORSAllowsAllRequiredMethods(t *testing.T) {
	handler := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), []string{"http://localhost:5173"})

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/system/users/some-id/status", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodPatch)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}

	allowMethods := response.Header().Get("Access-Control-Allow-Methods")
	if allowMethods == "" {
		t.Fatal("Access-Control-Allow-Methods header missing")
	}

	required := []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"}
	for _, method := range required {
		if !strings.Contains(allowMethods, method) {
			t.Errorf("%s missing from Access-Control-Allow-Methods: %q", method, allowMethods)
		}
	}
}

func TestWithCORSAllowsRequiredHeaders(t *testing.T) {
	handler := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), []string{"http://localhost:5173"})

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/local/login", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	allowHeaders := response.Header().Get("Access-Control-Allow-Headers")
	for _, header := range []string{"Content-Type", "Authorization"} {
		if !strings.Contains(allowHeaders, header) {
			t.Errorf("%s missing from Access-Control-Allow-Headers: %q", header, allowHeaders)
		}
	}

	if response.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("Access-Control-Allow-Credentials should be true")
	}
}
