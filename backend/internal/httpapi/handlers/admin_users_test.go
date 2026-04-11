package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/block-o/exchangely/backend/internal/httpapi/middleware"
	"github.com/google/uuid"
)

// mockAdminUserService is a mock implementation of AdminUserService for testing.
type mockAdminUserService struct {
	listUsersFunc          func(ctx context.Context, search, role, status string, page, limit int) ([]auth.User, int, error)
	getUserFunc            func(ctx context.Context, userID uuid.UUID) (*auth.User, error)
	updateRoleFunc         func(ctx context.Context, actingAdminID, targetUserID uuid.UUID, newRole string) (*auth.User, error)
	updateDisabledFunc     func(ctx context.Context, actingAdminID, targetUserID uuid.UUID, disabled bool) (*auth.User, error)
	forcePasswordResetFunc func(ctx context.Context, actingAdminID, targetUserID uuid.UUID) (*auth.User, error)
}

func (m *mockAdminUserService) ListUsers(ctx context.Context, search, role, status string, page, limit int) ([]auth.User, int, error) {
	if m.listUsersFunc != nil {
		return m.listUsersFunc(ctx, search, role, status, page, limit)
	}
	return []auth.User{}, 0, nil
}

func (m *mockAdminUserService) GetUser(ctx context.Context, userID uuid.UUID) (*auth.User, error) {
	if m.getUserFunc != nil {
		return m.getUserFunc(ctx, userID)
	}
	return nil, auth.ErrUserNotFound
}

func (m *mockAdminUserService) UpdateRole(ctx context.Context, actingAdminID, targetUserID uuid.UUID, newRole string) (*auth.User, error) {
	if m.updateRoleFunc != nil {
		return m.updateRoleFunc(ctx, actingAdminID, targetUserID, newRole)
	}
	return nil, nil
}

func (m *mockAdminUserService) UpdateDisabled(ctx context.Context, actingAdminID, targetUserID uuid.UUID, disabled bool) (*auth.User, error) {
	if m.updateDisabledFunc != nil {
		return m.updateDisabledFunc(ctx, actingAdminID, targetUserID, disabled)
	}
	return nil, nil
}

func (m *mockAdminUserService) ForcePasswordReset(ctx context.Context, actingAdminID, targetUserID uuid.UUID) (*auth.User, error) {
	if m.forcePasswordResetFunc != nil {
		return m.forcePasswordResetFunc(ctx, actingAdminID, targetUserID)
	}
	return nil, nil
}

// TestListUsers_Success tests the List handler with valid query parameters.
func TestListUsers_Success(t *testing.T) {
	mockSvc := &mockAdminUserService{
		listUsersFunc: func(ctx context.Context, search, role, status string, page, limit int) ([]auth.User, int, error) {
			return []auth.User{
				{ID: uuid.New(), Email: "user1@example.com", Role: "user"},
				{ID: uuid.New(), Email: "user2@example.com", Role: "premium"},
			}, 2, nil
		},
	}

	handler := NewAdminUserHandler(mockSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/users?page=1&limit=50", nil)
	w := httptest.NewRecorder()

	handler.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["total"] != float64(2) {
		t.Errorf("expected total 2, got %v", resp["total"])
	}
}

// TestListUsers_InvalidPagination tests the List handler with invalid pagination parameters.
func TestListUsers_InvalidPagination(t *testing.T) {
	mockSvc := &mockAdminUserService{}
	handler := NewAdminUserHandler(mockSvc)

	tests := []struct {
		name  string
		query string
	}{
		{"negative page", "?page=-1&limit=50"},
		{"zero page", "?page=0&limit=50"},
		{"negative limit", "?page=1&limit=-1"},
		{"zero limit", "?page=1&limit=0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/system/users"+tt.query, nil)
			w := httptest.NewRecorder()

			handler.List(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", w.Code)
			}
		})
	}
}

// TestGetUser_Success tests the Get handler with a valid user ID.
func TestGetUser_Success(t *testing.T) {
	userID := uuid.New()
	mockSvc := &mockAdminUserService{
		getUserFunc: func(ctx context.Context, id uuid.UUID) (*auth.User, error) {
			if id == userID {
				return &auth.User{ID: userID, Email: "user@example.com", Role: "user"}, nil
			}
			return nil, auth.ErrUserNotFound
		},
	}

	handler := NewAdminUserHandler(mockSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/users/"+userID.String(), nil)
	w := httptest.NewRecorder()

	handler.Get(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestGetUser_NotFound tests the Get handler when user doesn't exist.
func TestGetUser_NotFound(t *testing.T) {
	mockSvc := &mockAdminUserService{
		getUserFunc: func(ctx context.Context, id uuid.UUID) (*auth.User, error) {
			return nil, auth.ErrUserNotFound
		},
	}

	handler := NewAdminUserHandler(mockSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/users/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()

	handler.Get(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

// TestGetUser_NilUserReturns404 tests that the Get handler returns 404 when
// the service returns (nil, nil) — the FindByID pattern for not-found.
func TestGetUser_NilUserReturns404(t *testing.T) {
	mockSvc := &mockAdminUserService{
		getUserFunc: func(ctx context.Context, id uuid.UUID) (*auth.User, error) {
			return nil, nil
		},
	}

	handler := NewAdminUserHandler(mockSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/users/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()

	handler.Get(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "user not found" {
		t.Errorf("expected error 'user not found', got %q", resp["error"])
	}
}

// TestGetUser_InvalidUUID tests the Get handler with an invalid UUID.
func TestGetUser_InvalidUUID(t *testing.T) {
	mockSvc := &mockAdminUserService{}
	handler := NewAdminUserHandler(mockSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/users/invalid-uuid", nil)
	w := httptest.NewRecorder()

	handler.Get(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestUpdateRole_Success tests the UpdateRole handler with valid input.
func TestUpdateRole_Success(t *testing.T) {
	adminID := uuid.New()
	targetID := uuid.New()

	mockSvc := &mockAdminUserService{
		updateRoleFunc: func(ctx context.Context, actingAdminID, targetUserID uuid.UUID, newRole string) (*auth.User, error) {
			return &auth.User{ID: targetID, Email: "user@example.com", Role: newRole}, nil
		},
	}

	handler := NewAdminUserHandler(mockSvc)
	body := bytes.NewBufferString(`{"role":"premium"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/system/users/"+targetID.String()+"/role", body)
	req.Header.Set("Content-Type", "application/json")

	// Add claims to context.
	claims := &auth.Claims{Sub: adminID.String(), Role: "admin"}
	ctx := middleware.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.UpdateRole(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestUpdateRole_InvalidRole tests the UpdateRole handler with an invalid role.
func TestUpdateRole_InvalidRole(t *testing.T) {
	adminID := uuid.New()
	targetID := uuid.New()

	mockSvc := &mockAdminUserService{
		updateRoleFunc: func(ctx context.Context, actingAdminID, targetUserID uuid.UUID, newRole string) (*auth.User, error) {
			return nil, auth.ErrInvalidRole
		},
	}

	handler := NewAdminUserHandler(mockSvc)
	body := bytes.NewBufferString(`{"role":"superadmin"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/system/users/"+targetID.String()+"/role", body)
	req.Header.Set("Content-Type", "application/json")

	claims := &auth.Claims{Sub: adminID.String(), Role: "admin"}
	ctx := middleware.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.UpdateRole(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestUpdateRole_SelfChange tests that an admin cannot change their own role.
func TestUpdateRole_SelfChange(t *testing.T) {
	adminID := uuid.New()

	mockSvc := &mockAdminUserService{
		updateRoleFunc: func(ctx context.Context, actingAdminID, targetUserID uuid.UUID, newRole string) (*auth.User, error) {
			return nil, auth.ErrCannotChangeSelfRole
		},
	}

	handler := NewAdminUserHandler(mockSvc)
	body := bytes.NewBufferString(`{"role":"user"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/system/users/"+adminID.String()+"/role", body)
	req.Header.Set("Content-Type", "application/json")

	claims := &auth.Claims{Sub: adminID.String(), Role: "admin"}
	ctx := middleware.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.UpdateRole(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["error"] != "cannot change own role" {
		t.Errorf("expected error 'cannot change own role', got %s", resp["error"])
	}
}

// TestUpdateStatus_Success tests the UpdateStatus handler with valid input.
func TestUpdateStatus_Success(t *testing.T) {
	adminID := uuid.New()
	targetID := uuid.New()

	mockSvc := &mockAdminUserService{
		updateDisabledFunc: func(ctx context.Context, actingAdminID, targetUserID uuid.UUID, disabled bool) (*auth.User, error) {
			return &auth.User{ID: targetID, Email: "user@example.com", Disabled: disabled}, nil
		},
	}

	handler := NewAdminUserHandler(mockSvc)
	body := bytes.NewBufferString(`{"disabled":true}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/system/users/"+targetID.String()+"/status", body)
	req.Header.Set("Content-Type", "application/json")

	claims := &auth.Claims{Sub: adminID.String(), Role: "admin"}
	ctx := middleware.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.UpdateStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestUpdateStatus_SelfDisable tests that an admin cannot disable their own account.
func TestUpdateStatus_SelfDisable(t *testing.T) {
	adminID := uuid.New()

	mockSvc := &mockAdminUserService{
		updateDisabledFunc: func(ctx context.Context, actingAdminID, targetUserID uuid.UUID, disabled bool) (*auth.User, error) {
			return nil, auth.ErrCannotDisableSelf
		},
	}

	handler := NewAdminUserHandler(mockSvc)
	body := bytes.NewBufferString(`{"disabled":true}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/system/users/"+adminID.String()+"/status", body)
	req.Header.Set("Content-Type", "application/json")

	claims := &auth.Claims{Sub: adminID.String(), Role: "admin"}
	ctx := middleware.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.UpdateStatus(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["error"] != "cannot disable own account" {
		t.Errorf("expected error 'cannot disable own account', got %s", resp["error"])
	}
}

// TestForcePasswordReset_Success tests the ForcePasswordReset handler with valid input.
func TestForcePasswordReset_Success(t *testing.T) {
	adminID := uuid.New()
	targetID := uuid.New()

	mockSvc := &mockAdminUserService{
		forcePasswordResetFunc: func(ctx context.Context, actingAdminID, targetUserID uuid.UUID) (*auth.User, error) {
			return &auth.User{ID: targetID, Email: "user@example.com", MustChangePassword: true}, nil
		},
	}

	handler := NewAdminUserHandler(mockSvc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/users/"+targetID.String()+"/force-password-reset", nil)

	claims := &auth.Claims{Sub: adminID.String(), Role: "admin"}
	ctx := middleware.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ForcePasswordReset(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestForcePasswordReset_NoPasswordAuth tests that force reset fails for OAuth-only users.
func TestForcePasswordReset_NoPasswordAuth(t *testing.T) {
	adminID := uuid.New()
	targetID := uuid.New()

	mockSvc := &mockAdminUserService{
		forcePasswordResetFunc: func(ctx context.Context, actingAdminID, targetUserID uuid.UUID) (*auth.User, error) {
			return nil, auth.ErrNoPasswordAuth
		},
	}

	handler := NewAdminUserHandler(mockSvc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/users/"+targetID.String()+"/force-password-reset", nil)

	claims := &auth.Claims{Sub: adminID.String(), Role: "admin"}
	ctx := middleware.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ForcePasswordReset(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["error"] != "user has no password authentication" {
		t.Errorf("expected error 'user has no password authentication', got %s", resp["error"])
	}
}

// TestUnauthorized_NoClaims tests that handlers return 401 when no claims are in context.
func TestUnauthorized_NoClaims(t *testing.T) {
	mockSvc := &mockAdminUserService{}
	handler := NewAdminUserHandler(mockSvc)

	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		handler func(w http.ResponseWriter, r *http.Request)
	}{
		{"UpdateRole", http.MethodPatch, "/api/v1/system/users/" + uuid.New().String() + "/role", `{"role":"premium"}`, handler.UpdateRole},
		{"UpdateStatus", http.MethodPatch, "/api/v1/system/users/" + uuid.New().String() + "/status", `{"disabled":true}`, handler.UpdateStatus},
		{"ForcePasswordReset", http.MethodPost, "/api/v1/system/users/" + uuid.New().String() + "/force-password-reset", "", handler.ForcePasswordReset},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *bytes.Buffer
			if tt.body != "" {
				body = bytes.NewBufferString(tt.body)
			} else {
				body = bytes.NewBuffer(nil)
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()

			tt.handler(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected status 401, got %d", w.Code)
			}
		})
	}
}
