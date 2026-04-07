package postgres

import (
	"context"
	"database/sql"
	"time"
)

type CoverageRepository struct {
	db *sql.DB
}

func NewCoverageRepository(db *sql.DB) *CoverageRepository {
	return &CoverageRepository{db: db}
}

func (r *CoverageRepository) MarkDayComplete(ctx context.Context, pairSymbol string, day time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO data_coverage (pair_symbol, day, is_complete, updated_at)
		VALUES ($1, $2, TRUE, NOW())
		ON CONFLICT (pair_symbol, day) DO UPDATE
		SET is_complete = TRUE,
		    updated_at = NOW()
	`, pairSymbol, day.UTC().Truncate(24*time.Hour))
	return err
}

func (r *CoverageRepository) GetCompletedDays(ctx context.Context, pairSymbol string, start, end time.Time) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT day
		FROM data_coverage
		WHERE pair_symbol = $1
		  AND day >= $2
		  AND day <= $3
		  AND is_complete = TRUE
	`, pairSymbol, start.UTC().Truncate(24*time.Hour), end.UTC().Truncate(24*time.Hour))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	result := make(map[string]bool)
	for rows.Next() {
		var day time.Time
		if err := rows.Scan(&day); err != nil {
			return nil, err
		}
		result[day.UTC().Format("2006-01-02")] = true
	}

	return result, rows.Err()
}

func (r *CoverageRepository) GetAllCompletedDays(ctx context.Context) (map[string]map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT pair_symbol, day
		FROM data_coverage
		WHERE is_complete = TRUE
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
