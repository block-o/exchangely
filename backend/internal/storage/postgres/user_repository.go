package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/google/uuid"
)

// UserRepository implements auth.UserRepository using PostgreSQL.
type UserRepository struct {
	db *sql.DB
}

// Compile-time check that UserRepository satisfies the auth.UserRepository interface.
var _ auth.UserRepository = (*UserRepository)(nil)

// NewUserRepository returns a new UserRepository backed by the given database.
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// FindByID returns the user with the given ID, or nil if not found.
func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*auth.User, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, email, name, avatar_url, role, google_id, password_hash,
		       must_change_password, created_at, updated_at
		FROM users
		WHERE id = $1
	`, id)
	return scanUser(row)
}

// FindByEmail returns the user with the given email, or nil if not found.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*auth.User, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, email, name, avatar_url, role, google_id, password_hash,
		       must_change_password, created_at, updated_at
		FROM users
		WHERE email = $1
	`, email)
	return scanUser(row)
}

// FindByGoogleID returns the user with the given Google ID, or nil if not found.
func (r *UserRepository) FindByGoogleID(ctx context.Context, googleID string) (*auth.User, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, email, name, avatar_url, role, google_id, password_hash,
		       must_change_password, created_at, updated_at
		FROM users
		WHERE google_id = $1
	`, googleID)
	return scanUser(row)
}

// Create inserts a new user row. The user's CreatedAt and UpdatedAt fields are
// set server-side by the database defaults, and the returned values are scanned
// back into the struct.
func (r *UserRepository) Create(ctx context.Context, user *auth.User) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO users (id, email, name, avatar_url, role, google_id, password_hash, must_change_password)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at
	`,
		user.ID, user.Email, user.Name, user.AvatarURL,
		user.Role, user.GoogleID, user.PasswordHash, user.MustChangePassword,
	).Scan(&user.CreatedAt, &user.UpdatedAt)
}

// Update persists changes to an existing user's profile fields and bumps updated_at.
func (r *UserRepository) Update(ctx context.Context, user *auth.User) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET email = $2, name = $3, avatar_url = $4, role = $5,
		    google_id = $6, password_hash = $7, must_change_password = $8,
		    updated_at = $9
		WHERE id = $1
	`,
		user.ID, user.Email, user.Name, user.AvatarURL,
		user.Role, user.GoogleID, user.PasswordHash, user.MustChangePassword,
		now,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	user.UpdatedAt = now
	return nil
}

// UpdatePasswordHash sets the password hash and must_change_password flag for a user.
func (r *UserRepository) UpdatePasswordHash(ctx context.Context, userID uuid.UUID, hash string, mustChange bool) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET password_hash = $2, must_change_password = $3, updated_at = now()
		WHERE id = $1
	`, userID, hash, mustChange)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// scanUser scans a single user row. Returns nil, nil when no row is found.
func scanUser(row *sql.Row) (*auth.User, error) {
	var u auth.User
	err := row.Scan(
		&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.Role,
		&u.GoogleID, &u.PasswordHash,
		&u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
