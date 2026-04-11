package postgres

import (
	"context"
	"database/sql"
	"time"
)

type IntegrityRepository struct {
	db *sql.DB
}

func NewIntegrityRepository(db *sql.DB) *IntegrityRepository {
	return &IntegrityRepository{db: db}
}

func (r *IntegrityRepository) MarkDayVerified(ctx context.Context, pairSymbol string, day time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO integrity_coverage (pair_symbol, day, verified, updated_at)
		VALUES ($1, $2, TRUE, NOW())
		ON CONFLICT (pair_symbol, day) DO UPDATE
		SET verified = TRUE,
		    updated_at = NOW()
	`, pairSymbol, day.UTC().Truncate(24*time.Hour))
	return err
}

func (r *IntegrityRepository) GetAllVerifiedDays(ctx context.Context) (map[string]map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT pair_symbol, day
		FROM integrity_coverage
		WHERE verified = TRUE
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	result := make(map[string]map[string]bool)
	for rows.Next() {
		var pairSymbol string
		var day time.Time
		if err := rows.Scan(&pairSymbol, &day); err != nil {
			return nil, err
		}
		if result[pairSymbol] == nil {
			result[pairSymbol] = make(map[string]bool)
		}
		result[pairSymbol][day.UTC().Format("2006-01-02")] = true
	}

	return result, rows.Err()
}
