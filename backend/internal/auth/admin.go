package auth

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
)

var (
	// ErrInvalidRole is returned when an invalid role value is provided.
	ErrInvalidRole = errors.New("invalid role")
	// ErrCannotChangeSelfRole is returned when an admin attempts to change their own role.
	ErrCannotChangeSelfRole = errors.New("cannot change own role")
	// ErrCannotDisableSelf is returned when an admin attempts to disable their own account.
	ErrCannotDisableSelf = errors.New("cannot disable own account")
	// ErrNoPasswordAuth is returned when attempting to force password reset on a user without password authentication.
	ErrNoPasswordAuth = errors.New("user has no password authentication")
)

// AdminUserService provides admin user management operations.
type AdminUserService struct {
	users    UserRepository
	sessions SessionRepository
}

// NewAdminUserService creates a new AdminUserService.
func NewAdminUserService(users UserRepository, sessions SessionRepository) *AdminUserService {
	return &AdminUserService{
		users:    users,
		sessions: sessions,
	}
}

// ListUsers returns a paginated list of users matching the given filters.
func (s *AdminUserService) ListUsers(ctx context.Context, search, role, status string, page, limit int) ([]User, int, error) {
	offset := (page - 1) * limit
	return s.users.ListWithFilters(ctx, search, role, status, limit, offset)
}

// GetUser returns a single user by ID.
func (s *AdminUserService) GetUser(ctx context.Context, userID uuid.UUID) (*User, error) {
	return s.users.FindByID(ctx, userID)
}

// UpdateRole updates a user's role. Returns an error if the role is invalid,
// if the acting admin attempts to change their own role, or if the user is not found.
func (s *AdminUserService) UpdateRole(ctx context.Context, actingAdminID, targetUserID uuid.UUID, newRole string) (*User, error) {
	// Validate role.
	if newRole != "user" && newRole != "premium" && newRole != "admin" {
		return nil, ErrInvalidRole
	}

	// Prevent self-role-change.
	if actingAdminID == targetUserID {
		return nil, ErrCannotChangeSelfRole
	}

	// Fetch old user record for logging.
	oldUser, err := s.users.FindByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if oldUser == nil {
		return nil, ErrUserNotFound
	}

	oldRole := oldUser.Role

	// Update role.
	if err := s.users.UpdateRole(ctx, targetUserID, newRole); err != nil {
		return nil, err
	}

	// Fetch updated user record.
	updatedUser, err := s.users.FindByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}

	// Log structured event.
	slog.InfoContext(ctx, "auth event",
		"event", "admin_role_change",
		"acting_admin_id", actingAdminID,
		"target_user_id", targetUserID,
		"target_email", maskEmail(oldUser.Email),
		"old_role", oldRole,
		"new_role", newRole,
	)

	return updatedUser, nil
}

// UpdateDisabled updates a user's disabled status. Returns an error if the acting
// admin attempts to disable their own account or if the user is not found.
func (s *AdminUserService) UpdateDisabled(ctx context.Context, actingAdminID, targetUserID uuid.UUID, disabled bool) (*User, error) {
	// Prevent self-disable.
	if actingAdminID == targetUserID {
		return nil, ErrCannotDisableSelf
	}

	// Update disabled status.
	if err := s.users.UpdateDisabled(ctx, targetUserID, disabled); err != nil {
		return nil, err
	}

	// If disabling, invalidate all sessions.
	if disabled {
		if err := s.sessions.DeleteAllForUser(ctx, targetUserID); err != nil {
			// Log but don't fail the operation.
			slog.ErrorContext(ctx, "failed to delete sessions for disabled user",
				"target_user_id", targetUserID,
				"error", err,
			)
		}
	}

	// Fetch updated user record.
	updatedUser, err := s.users.FindByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}

	// Log structured event.
	slog.InfoContext(ctx, "auth event",
		"event", "admin_disable_status_change",
		"acting_admin_id", actingAdminID,
		"target_user_id", targetUserID,
		"disabled", disabled,
	)

	return updatedUser, nil
}

// ForcePasswordReset sets the must_change_password flag for a user. Returns an error
// if the user does not have password authentication or if the user is not found.
func (s *AdminUserService) ForcePasswordReset(ctx context.Context, actingAdminID, targetUserID uuid.UUID) (*User, error) {
	// Fetch user to check has_password.
	user, err := s.users.FindByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	// Check user has password authentication.
	if !user.HasPassword {
		return nil, ErrNoPasswordAuth
	}

	// Set must_change_password flag.
	if err := s.users.SetMustChangePassword(ctx, targetUserID, true); err != nil {
		return nil, err
	}

	// Fetch updated user record.
	updatedUser, err := s.users.FindByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}

	// Log structured event.
	slog.InfoContext(ctx, "auth event",
		"event", "admin_force_password_reset",
		"acting_admin_id", actingAdminID,
		"target_user_id", targetUserID,
		"target_email", maskEmail(user.Email),
	)

	return updatedUser, nil
}
