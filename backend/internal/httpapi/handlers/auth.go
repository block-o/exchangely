package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

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
	service         *auth.Service
	apiTokenService *auth.APITokenService
	env             string // "development" or "production"
}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(service *auth.Service, env string) *AuthHandler {
	return &AuthHandler{service: service, env: env}
}

// WithAPITokenService returns a copy of the handler with the given
// APITokenService attached. The token management endpoints (Create, List,
// Revoke) require this service to be set.
func (h *AuthHandler) WithAPITokenService(svc *auth.APITokenService) *AuthHandler {
	h.apiTokenService = svc
	return h
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
		if errors.Is(err, auth.ErrAccountDisabled) {
			http.Redirect(w, r, "/#login?error=account_disabled", http.StatusFound)
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

// requireJWTSession checks that the request was authenticated via JWT (not an
// API token). Returns true if the request should proceed, false if a 401 was
// written. API tokens cannot manage other API tokens.
func (h *AuthHandler) requireJWTSession(w http.ResponseWriter, r *http.Request) (*auth.Claims, bool) {
	method, _ := middleware.AuthMethodFromContext(r.Context())
	if method == "api_token" {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	return claims, true
}

// CreateAPIToken handles POST /api/v1/auth/api-tokens.
// It creates a new API token for the authenticated user and returns the raw
// token exactly once in the response body with a 201 status.
func (h *AuthHandler) CreateAPIToken(w http.ResponseWriter, r *http.Request) {
	h.setSecurityHeaders(w)

	claims, ok := h.requireJWTSession(w, r)
	if !ok {
		return
	}

	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	label := strings.TrimSpace(req.Label)
	if label == "" {
		h.writeJSONError(w, http.StatusBadRequest, "label is required")
		return
	}

	rawToken, token, err := h.apiTokenService.CreateToken(r.Context(), userID, label)
	if err != nil {
		if errors.Is(err, auth.ErrTokenLimitReached) {
			h.writeJSONError(w, http.StatusConflict, "token limit reached")
			return
		}
		slog.Error("create api token failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]any{
		"token":      rawToken,
		"id":         token.ID.String(),
		"label":      token.Label,
		"prefix":     token.Prefix,
		"created_at": token.CreatedAt,
		"expires_at": token.ExpiresAt,
	})
}

// ListAPITokens handles GET /api/v1/auth/api-tokens.
// It returns all tokens for the authenticated user.
func (h *AuthHandler) ListAPITokens(w http.ResponseWriter, r *http.Request) {
	h.setSecurityHeaders(w)

	claims, ok := h.requireJWTSession(w, r)
	if !ok {
		return
	}

	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tokens, err := h.apiTokenService.ListTokens(r.Context(), userID)
	if err != nil {
		slog.Error("list api tokens failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Build response items with status field.
	type tokenResponse struct {
		ID         uuid.UUID  `json:"id"`
		Label      string     `json:"label"`
		Prefix     string     `json:"prefix"`
		Status     string     `json:"status"`
		CreatedAt  time.Time  `json:"created_at"`
		LastUsedAt *time.Time `json:"last_used_at"`
		RevokedAt  *time.Time `json:"revoked_at"`
		ExpiresAt  time.Time  `json:"expires_at"`
	}

	items := make([]tokenResponse, len(tokens))
	for i, t := range tokens {
		items[i] = tokenResponse{
			ID:         t.ID,
			Label:      t.Label,
			Prefix:     t.Prefix,
			Status:     t.TokenStatus(),
			CreatedAt:  t.CreatedAt,
			LastUsedAt: t.LastUsedAt,
			RevokedAt:  t.RevokedAt,
			ExpiresAt:  t.ExpiresAt,
		}
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// RevokeAPIToken handles DELETE /api/v1/auth/api-tokens/{id}.
// It revokes the specified token for the authenticated user.
func (h *AuthHandler) RevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	h.setSecurityHeaders(w)

	claims, ok := h.requireJWTSession(w, r)
	if !ok {
		return
	}

	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Extract token ID from the URL path.
	tokenIDStr := strings.TrimPrefix(r.URL.Path, "/api/v1/auth/api-tokens/")
	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid token id")
		return
	}

	if err := h.apiTokenService.RevokeToken(r.Context(), tokenID, userID); err != nil {
		if errors.Is(err, auth.ErrTokenNotFound) {
			h.writeJSONError(w, http.StatusNotFound, "not found")
			return
		}
		slog.Error("revoke api token failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

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
	case errors.Is(err, auth.ErrAccountDisabled):
		h.writeJSONError(w, http.StatusUnauthorized, "account disabled")
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
