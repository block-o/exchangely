package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/google/uuid"
)

// RateLimitRepository implements auth.RateLimitRepository using PostgreSQL.
type RateLimitRepository struct {
	db *sql.DB
}

// Compile-time check that RateLimitRepository satisfies the auth.RateLimitRepository interface.
var _ auth.RateLimitRepository = (*RateLimitRepository)(nil)

// NewRateLimitRepository returns a new RateLimitRepository backed by the given database.
func NewRateLimitRepository(db *sql.DB) *RateLimitRepository {
	return &RateLimitRepository{db: db}
}

// CheckAndIncrement atomically inserts a request record and returns the count
// of requests within the current window for the given token/user. Uses a CTE
// to combine INSERT and COUNT in a single round trip.
func (r *RateLimitRepository) CheckAndIncrement(ctx context.Context, tokenID *uuid.UUID, userID *uuid.UUID, ip string, window time.Duration) (int, error) {
	interval := formatInterval(window)
	var count int
	err := r.db.QueryRowContext(ctx, `
		WITH inserted AS (
			INSERT INTO api_rate_limit_log (token_id, user_id, ip_address, requested_at)
			VALUES ($1, $2, $3, now())
			RETURNING requested_at
		)
		SELECT COUNT(*) FROM api_rate_limit_log
		WHERE (token_id = $1 OR user_id = $2)
		  AND requested_at > now() - $4::interval
	`, tokenID, userID, ip, interval).Scan(&count)
	return count, err
}

// CheckIPAndIncrement atomically inserts a request record and returns the
// count of requests within the current window for the given IP address.
func (r *RateLimitRepository) CheckIPAndIncrement(ctx context.Context, ip string, window time.Duration) (int, error) {
	interval := formatInterval(window)
	var count int
	err := r.db.QueryRowContext(ctx, `
		WITH inserted AS (
			INSERT INTO api_rate_limit_log (ip_address, requested_at)
			VALUES ($1, now())
			RETURNING requested_at
		)
		SELECT COUNT(*) FROM api_rate_limit_log
		WHERE ip_address = $1
		  AND requested_at > now() - $2::interval
	`, ip, interval).Scan(&count)
	return count, err
}

// PruneExpired removes rows older than the given window and returns the
// number of rows deleted.
func (r *RateLimitRepository) PruneExpired(ctx context.Context, window time.Duration) (int64, error) {
	interval := formatInterval(window)
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM api_rate_limit_log
		WHERE requested_at < now() - $1::interval
	`, interval)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// formatInterval converts a time.Duration to a PostgreSQL interval string
// (e.g., "60 seconds").
func formatInterval(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int(d.Seconds()))
}
