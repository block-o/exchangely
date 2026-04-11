package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/block-o/exchangely/backend/internal/httpapi/middleware"
	"github.com/google/uuid"
)

// AdminUserServiceInterface defines the interface for admin user management operations.
type AdminUserServiceInterface interface {
	ListUsers(ctx context.Context, search, role, status string, page, limit int) ([]auth.User, int, error)
	GetUser(ctx context.Context, userID uuid.UUID) (*auth.User, error)
	UpdateRole(ctx context.Context, actingAdminID, targetUserID uuid.UUID, newRole string) (*auth.User, error)
	UpdateDisabled(ctx context.Context, actingAdminID, targetUserID uuid.UUID, disabled bool) (*auth.User, error)
	ForcePasswordReset(ctx context.Context, actingAdminID, targetUserID uuid.UUID) (*auth.User, error)
}

// AdminUserHandler exposes HTTP handler methods for admin user management endpoints.
type AdminUserHandler struct {
	service AdminUserServiceInterface
}

// NewAdminUserHandler creates an AdminUserHandler.
func NewAdminUserHandler(service AdminUserServiceInterface) *AdminUserHandler {
	return &AdminUserHandler{service: service}
}

// List handles GET /api/v1/system/users with pagination, search, and filters.
func (h *AdminUserHandler) List(w http.ResponseWriter, r *http.Request) {
	// Extract query parameters.
	search := r.URL.Query().Get("search")
	role := r.URL.Query().Get("role")
	status := r.URL.Query().Get("status")

	page := getIntParam(r, "page", 1)
	limit := getIntParam(r, "limit", 50)

	// Validate pagination parameters.
	if page <= 0 || limit <= 0 {
		h.writeJSONError(w, http.StatusBadRequest, "invalid pagination parameters")
		return
	}

	// Call service.
	users, total, err := h.service.ListUsers(r.Context(), search, role, status, page, limit)
	if err != nil {
		slog.Error("list users failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Ensure data is never null in JSON response.
	if users == nil {
		users = []auth.User{}
	}

	// Write response.
	h.writeJSON(w, http.StatusOK, map[string]any{
		"data":  users,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// Get handles GET /api/v1/system/users/{id}.
func (h *AdminUserHandler) Get(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from path.
	userIDStr := strings.TrimPrefix(r.URL.Path, "/api/v1/system/users/")
	userIDStr = strings.Split(userIDStr, "/")[0] // Handle trailing path segments.

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	// Call service.
	user, err := h.service.GetUser(r.Context(), userID)
	if err != nil {
		if err == auth.ErrUserNotFound {
			h.writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		slog.Error("get user failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		h.writeJSONError(w, http.StatusNotFound, "user not found")
		return
	}

	// Write response.
	h.writeJSON(w, http.StatusOK, user)
}

// UpdateRole handles PATCH /api/v1/system/users/{id}/role.
func (h *AdminUserHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	// Extract acting admin ID from context.
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	actingAdminID, err := uuid.Parse(claims.Sub)
	if err != nil {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Extract user ID from path.
	userIDStr := strings.TrimPrefix(r.URL.Path, "/api/v1/system/users/")
	userIDStr = strings.TrimSuffix(userIDStr, "/role")

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	// Parse request body.
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Call service.
	user, err := h.service.UpdateRole(r.Context(), actingAdminID, userID, req.Role)
	if err != nil {
		switch err {
		case auth.ErrInvalidRole:
			h.writeJSONError(w, http.StatusBadRequest, "invalid role: must be user, premium, or admin")
		case auth.ErrCannotChangeSelfRole:
			h.writeJSONError(w, http.StatusBadRequest, "cannot change own role")
		case auth.ErrUserNotFound:
			h.writeJSONError(w, http.StatusNotFound, "user not found")
		default:
			slog.Error("update role failed", "error", err)
			h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	// Write response.
	h.writeJSON(w, http.StatusOK, user)
}

// UpdateStatus handles PATCH /api/v1/system/users/{id}/status.
func (h *AdminUserHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	// Extract acting admin ID from context.
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	actingAdminID, err := uuid.Parse(claims.Sub)
	if err != nil {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Extract user ID from path.
	userIDStr := strings.TrimPrefix(r.URL.Path, "/api/v1/system/users/")
	userIDStr = strings.TrimSuffix(userIDStr, "/status")

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	// Parse request body.
	var req struct {
		Disabled bool `json:"disabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Call service.
	user, err := h.service.UpdateDisabled(r.Context(), actingAdminID, userID, req.Disabled)
	if err != nil {
		switch err {
		case auth.ErrCannotDisableSelf:
			h.writeJSONError(w, http.StatusBadRequest, "cannot disable own account")
		case auth.ErrUserNotFound:
			h.writeJSONError(w, http.StatusNotFound, "user not found")
		default:
			slog.Error("update status failed", "error", err)
			h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	// Write response.
	h.writeJSON(w, http.StatusOK, user)
}

// ForcePasswordReset handles POST /api/v1/system/users/{id}/force-password-reset.
func (h *AdminUserHandler) ForcePasswordReset(w http.ResponseWriter, r *http.Request) {
	// Extract acting admin ID from context.
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	actingAdminID, err := uuid.Parse(claims.Sub)
	if err != nil {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Extract user ID from path.
	userIDStr := strings.TrimPrefix(r.URL.Path, "/api/v1/system/users/")
	userIDStr = strings.TrimSuffix(userIDStr, "/force-password-reset")

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	// Call service.
	user, err := h.service.ForcePasswordReset(r.Context(), actingAdminID, userID)
	if err != nil {
		switch err {
		case auth.ErrNoPasswordAuth:
			h.writeJSONError(w, http.StatusBadRequest, "user has no password authentication")
		case auth.ErrUserNotFound:
			h.writeJSONError(w, http.StatusNotFound, "user not found")
		default:
			slog.Error("force password reset failed", "error", err)
			h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	// Write response.
	h.writeJSON(w, http.StatusOK, user)
}

// getIntParam extracts an integer query parameter with a default value.
func getIntParam(r *http.Request, name string, defaultVal int) int {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

// writeJSON writes a JSON response with the given status code.
func (h *AdminUserHandler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// writeJSONError writes a JSON error response.
func (h *AdminUserHandler) writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
