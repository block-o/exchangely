package postgres

import (
	"context"
	"database/sql"
)

type WarningDismissalRepository struct {
	db *sql.DB
}

func NewWarningDismissalRepository(db *sql.DB) *WarningDismissalRepository {
	return &WarningDismissalRepository{db: db}
}

func (r *WarningDismissalRepository) DismissWarning(ctx context.Context, warningID, fingerprint string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO dismissed_warnings (warning_id, fingerprint, dismissed_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (warning_id) DO UPDATE
		SET fingerprint = EXCLUDED.fingerprint,
		    dismissed_at = EXCLUDED.dismissed_at
	`, warningID, fingerprint)
	return err
}

func (r *WarningDismissalRepository) DismissedWarnings(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT warning_id, fingerprint
		FROM dismissed_warnings
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	items := make(map[string]string)
	for rows.Next() {
		var warningID string
		var fingerprint string
		if err := rows.Scan(&warningID, &fingerprint); err != nil {
			return nil, err
		}
		items[warningID] = fingerprint
	}

	return items, rows.Err()
}
