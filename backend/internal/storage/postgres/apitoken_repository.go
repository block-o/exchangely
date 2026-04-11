package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/google/uuid"
)

// APITokenRepository implements auth.APITokenRepository using PostgreSQL.
type APITokenRepository struct {
	db *sql.DB
}

// Compile-time check that APITokenRepository satisfies the auth.APITokenRepository interface.
var _ auth.APITokenRepository = (*APITokenRepository)(nil)

// NewAPITokenRepository returns a new APITokenRepository backed by the given database.
func NewAPITokenRepository(db *sql.DB) *APITokenRepository {
	return &APITokenRepository{db: db}
}

// Create inserts a new API token row. The token's CreatedAt field is set
// server-side by the database default, and the returned value is scanned
// back into the struct.
func (r *APITokenRepository) Create(ctx context.Context, token *auth.APIToken) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO api_tokens (id, user_id, token_hash, label, prefix, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at
	`,
		token.ID, token.UserID, token.TokenHash,
		token.Label, token.Prefix, token.ExpiresAt,
	).Scan(&token.CreatedAt)
}

// FindByTokenHash returns the API token matching the given token hash,
// or nil if not found.
func (r *APITokenRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*auth.APIToken, error) {
	var t auth.APIToken
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, token_hash, label, prefix, created_at, last_used_at, revoked_at, expires_at
		FROM api_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(
		&t.ID, &t.UserID, &t.TokenHash,
		&t.Label, &t.Prefix, &t.CreatedAt,
		&t.LastUsedAt, &t.RevokedAt, &t.ExpiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListByUserID returns all API tokens for the given user, ordered by
// created_at descending (newest first).
func (r *APITokenRepository) ListByUserID(ctx context.Context, userID uuid.UUID) ([]auth.APIToken, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, token_hash, label, prefix, created_at, last_used_at, revoked_at, expires_at
		FROM api_tokens
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // error checked via rows.Err()

	var tokens []auth.APIToken
	for rows.Next() {
		var t auth.APIToken
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.TokenHash,
			&t.Label, &t.Prefix, &t.CreatedAt,
			&t.LastUsedAt, &t.RevokedAt, &t.ExpiresAt,
		); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// CountActiveByUserID returns the number of active (non-revoked, non-expired)
// tokens for the given user.
func (r *APITokenRepository) CountActiveByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM api_tokens
		WHERE user_id = $1
		  AND revoked_at IS NULL
		  AND expires_at > now()
	`, userID).Scan(&count)
	return count, err
}

// Revoke sets revoked_at on the token identified by id and user_id. Returns
// auth.ErrTokenNotFound if the token does not exist or belongs to a different
// user. Idempotent: re-revoking an already-revoked token returns nil.
func (r *APITokenRepository) Revoke(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	// First check that the token exists and belongs to this user.
	var revokedAt *time.Time
	err := r.db.QueryRowContext(ctx, `
		SELECT revoked_at FROM api_tokens WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&revokedAt)
	if err == sql.ErrNoRows {
		return auth.ErrTokenNotFound
	}
	if err != nil {
		return err
	}
	// Already revoked — idempotent success.
	if revokedAt != nil {
		return nil
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE api_tokens SET revoked_at = now() WHERE id = $1 AND user_id = $2
	`, id, userID)
	return err
}

// UpdateLastUsedAt sets the last_used_at timestamp for the given token.
func (r *APITokenRepository) UpdateLastUsedAt(ctx context.Context, id uuid.UUID, t time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE api_tokens
		SET last_used_at = $2
		WHERE id = $1
	`, id, t)
	return err
}
