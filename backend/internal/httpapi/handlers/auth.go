package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/block-o/exchangely/backend/internal/httpapi/middleware"
	"github.com/google/uuid"
)

const (
	refreshCookieName = "refresh_token"
	csrfCookieName    = "oauth_state"
	cookiePath        = "/api/v1/auth"
	refreshMaxAge     = 7 * 24 * 3600 // 7 days in seconds
	maxLoginBodyBytes = 1024          // 1KB
)

// AuthHandler exposes HTTP handler methods for the auth endpoints.
type AuthHandler struct {
	service *auth.Service
	env     string // "development" or "production"
}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(service *auth.Service, env string) *AuthHandler {
	return &AuthHandler{service: service, env: env}
}

// GoogleLogin redirects the user to the Google OAuth authorization URL.
// It stores a CSRF state value in a short-lived cookie for validation on callback.
func (h *AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	h.setSecurityHeaders(w)

	authURL, state, err := h.service.GoogleLogin()
	if err != nil {
		slog.Error("google login failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    state,
		Path:     cookiePath,
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		Secure:   h.env != "development",
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, authURL, http.StatusFound)
}

// GoogleCallback handles the OAuth redirect from Google. It validates the CSRF
// state, exchanges the authorization code for tokens, sets the refresh token
// cookie, and redirects to the frontend.
func (h *AuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	h.setSecurityHeaders(w)

	// Read CSRF state from cookie.
	stateCookie, err := r.Cookie(csrfCookieName)
	if err != nil {
		http.Redirect(w, r, "/#login?error=csrf_failed", http.StatusFound)
		return
	}

	// Clear the CSRF cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     cookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.env != "development",
		SameSite: http.SameSiteLaxMode,
	})

	queryState := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	if queryState == "" || code == "" {
		errMsg := r.URL.Query().Get("error")
		if errMsg == "" {
			errMsg = "oauth_failed"
		}
		http.Redirect(w, r, "/#login?error="+errMsg, http.StatusFound)
		return
	}

	accessToken, refreshToken, _, err := h.service.GoogleCallback(r.Context(), code, queryState, stateCookie.Value, r.RemoteAddr)
	if err != nil {
		slog.Error("google callback failed", "error", err)
		if errors.Is(err, auth.ErrCSRFStateMismatch) {
			http.Redirect(w, r, "/#login?error=csrf_failed", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/#login?error=oauth_failed", http.StatusFound)
		return
	}

	_ = accessToken // Access token is not used in the redirect; frontend fetches it via refresh.
	h.setRefreshCookie(w, refreshToken)
	http.Redirect(w, r, "/#", http.StatusFound)
}

// LocalLogin validates email/password credentials, enforces a 1KB body limit,
// and returns an access token in the response body.
func (h *AuthHandler) LocalLogin(w http.ResponseWriter, r *http.Request) {
	h.setSecurityHeaders(w)

	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBodyBytes)

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err.Error() == "http: request body too large" || strings.Contains(err.Error(), "http: request body too large") {
			h.writeJSONError(w, http.StatusRequestEntityTooLarge, "request too large")
			return
		}
		h.writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	userAgent := r.UserAgent()
	ip := r.RemoteAddr

	accessToken, refreshToken, _, err := h.service.LocalLogin(r.Context(), req.Email, req.Password, userAgent, ip)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	h.setRefreshCookie(w, refreshToken)
	h.writeJSON(w, http.StatusOK, map[string]string{
		"access_token": accessToken,
	})
}

// Refresh reads the refresh token cookie, rotates the session, and returns
// a new access token.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	h.setSecurityHeaders(w)

	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	userAgent := r.UserAgent()
	accessToken, newRefreshToken, err := h.service.RefreshToken(r.Context(), cookie.Value, userAgent)
	if err != nil {
		// Clear the cookie on any refresh failure.
		h.clearRefreshCookie(w)
		h.writeAuthError(w, err)
		return
	}

	h.setRefreshCookie(w, newRefreshToken)
	h.writeJSON(w, http.StatusOK, map[string]string{
		"access_token": accessToken,
	})
}

// Logout invalidates the session and clears the refresh token cookie.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.setSecurityHeaders(w)

	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		// No cookie — already logged out.
		h.clearRefreshCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := h.service.Logout(r.Context(), cookie.Value); err != nil {
		slog.Error("logout failed", "error", err)
	}

	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// Me returns the authenticated user's profile from the request context.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	h.setSecurityHeaders(w)

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.service.Me(r.Context(), userID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			h.writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		slog.Error("me endpoint failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.writeJSON(w, http.StatusOK, user)
}

// ChangePassword validates the current password and updates to a new one.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	h.setSecurityHeaders(w)

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.ChangePassword(r.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			h.writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		if strings.Contains(err.Error(), "password does not meet complexity requirements") {
			h.writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		slog.Error("change password failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Internal helpers ---

// setSecurityHeaders sets the required security headers on auth responses.
func (h *AuthHandler) setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cache-Control", "no-store")
}

// setRefreshCookie sets the refresh token as an HTTP-only cookie.
func (h *AuthHandler) setRefreshCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     cookiePath,
		MaxAge:   refreshMaxAge,
		HttpOnly: true,
		Secure:   h.env != "development",
		SameSite: http.SameSiteLaxMode,
	})
}

// clearRefreshCookie removes the refresh token cookie.
func (h *AuthHandler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     cookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.env != "development",
		SameSite: http.SameSiteLaxMode,
	})
}

// writeJSON writes a JSON response with the given status code.
func (h *AuthHandler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// writeJSONError writes a JSON error response.
func (h *AuthHandler) writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// writeAuthError maps auth service errors to appropriate HTTP responses.
func (h *AuthHandler) writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		h.writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
	case errors.Is(err, auth.ErrIPBlocked):
		w.Header().Set("Retry-After", "900")
		h.writeJSONError(w, http.StatusTooManyRequests, "too many requests")
	case errors.Is(err, auth.ErrRateLimited):
		w.Header().Set("Retry-After", "900") // 15 minutes
		h.writeJSONError(w, http.StatusTooManyRequests, "too many requests")
	case errors.Is(err, auth.ErrSessionNotFound), errors.Is(err, auth.ErrSessionExpired):
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, auth.ErrUserNotFound):
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
	default:
		slog.Error("auth error", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
	}
}
