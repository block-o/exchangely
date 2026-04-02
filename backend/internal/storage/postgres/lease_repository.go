package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/lease"
)

type LeaseRepository struct {
	db *sql.DB
}

func NewLeaseRepository(db *sql.DB) *LeaseRepository {
	return &LeaseRepository{db: db}
}

func (r *LeaseRepository) AcquireOrRenew(ctx context.Context, name, holder string, ttl time.Duration) (lease.Lease, bool, error) {
	var current lease.Lease
	current.Name = name

	row := r.db.QueryRowContext(ctx, `
		INSERT INTO service_leases (lease_name, holder_id, expires_at, updated_at)
		VALUES ($1, $2, NOW() + make_interval(secs => $3), NOW())
		ON CONFLICT (lease_name) DO UPDATE
		SET holder_id = EXCLUDED.holder_id,
		    expires_at = EXCLUDED.expires_at,
		    updated_at = NOW()
		WHERE service_leases.expires_at < NOW() OR service_leases.holder_id = EXCLUDED.holder_id
		RETURNING lease_name, holder_id, expires_at, updated_at
	`, name, holder, ttl.Seconds())

	if err := row.Scan(&current.Name, &current.HolderID, &current.ExpiresAt, &current.LastBeatAt); err == nil {
		return current, current.HolderID == holder, nil
	} else if err != sql.ErrNoRows {
		return lease.Lease{}, false, err
	}

	current, err := r.Current(ctx, name)
	if err != nil {
		return lease.Lease{}, false, err
	}

	return current, current.HolderID == holder, nil
}

func (r *LeaseRepository) Current(ctx context.Context, name string) (lease.Lease, error) {
	var current lease.Lease
	err := r.db.QueryRowContext(ctx, `
		SELECT lease_name, holder_id, expires_at, updated_at
		FROM service_leases
		WHERE lease_name = $1
	`, name).Scan(&current.Name, &current.HolderID, &current.ExpiresAt, &current.LastBeatAt)

	return current, err
}
