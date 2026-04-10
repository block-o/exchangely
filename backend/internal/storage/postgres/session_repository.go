package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/google/uuid"
)

// SessionRepository implements auth.SessionRepository using PostgreSQL.
type SessionRepository struct {
	db *sql.DB
}

// Compile-time check that SessionRepository satisfies the auth.SessionRepository interface.
var _ auth.SessionRepository = (*SessionRepository)(nil)

// NewSessionRepository returns a new SessionRepository backed by the given database.
func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// Create inserts a new session row. The session's CreatedAt field is set
// server-side by the database default, and the returned value is scanned
// back into the struct.
func (r *SessionRepository) Create(ctx context.Context, session *auth.Session) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO sessions (id, user_id, refresh_token_hash, expires_at, user_agent)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at
	`,
		session.ID, session.UserID, session.RefreshTokenHash,
		session.ExpiresAt, session.UserAgent,
	).Scan(&session.CreatedAt)
}

// FindByTokenHash returns the session matching the given refresh token hash,
// or nil if not found.
func (r *SessionRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*auth.Session, error) {
	var s auth.Session
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, refresh_token_hash, expires_at, created_at, user_agent
		FROM sessions
		WHERE refresh_token_hash = $1
	`, tokenHash).Scan(
		&s.ID, &s.UserID, &s.RefreshTokenHash,
		&s.ExpiresAt, &s.CreatedAt, &s.UserAgent,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Delete removes a single session by its ID.
func (r *SessionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = $1`, id)
	return err
}

// DeleteAllForUser removes all sessions belonging to the given user.
func (r *SessionRepository) DeleteAllForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

// DeleteExpired removes all sessions whose expires_at is in the past and
// returns the number of rows deleted.
func (r *SessionRepository) DeleteExpired(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < $1`, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
