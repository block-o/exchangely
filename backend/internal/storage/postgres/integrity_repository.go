package postgres

import (
	"context"
	"database/sql"
	"time"
)

// IntegrityResult captures the outcome of a single day's integrity check.
type IntegrityResult struct {
	PairSymbol      string
	Day             time.Time
	Verified        bool
	GapCount        int
	DivergenceCount int
	SourcesChecked  int
	ErrorMessage    string
}

type IntegrityRepository struct {
	db *sql.DB
}

func NewIntegrityRepository(db *sql.DB) *IntegrityRepository {
	return &IntegrityRepository{db: db}
}

func (r *IntegrityRepository) MarkDayVerified(ctx context.Context, pairSymbol string, day time.Time) error {
	return r.RecordResult(ctx, IntegrityResult{
		PairSymbol: pairSymbol,
		Day:        day,
		Verified:   true,
	})
}

// RecordResult persists the full outcome of an integrity check for a single day,
// including failure details when the check did not pass.
func (r *IntegrityRepository) RecordResult(ctx context.Context, result IntegrityResult) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO integrity_coverage (pair_symbol, day, verified, gap_count, divergence_count, sources_checked, error_message, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (pair_symbol, day) DO UPDATE
		SET verified         = $3,
		    gap_count        = $4,
		    divergence_count = $5,
		    sources_checked  = $6,
		    error_message    = $7,
		    updated_at       = NOW()
	`, result.PairSymbol, result.Day.UTC().Truncate(24*time.Hour), result.Verified,
		result.GapCount, result.DivergenceCount, result.SourcesChecked, result.ErrorMessage)
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

// GetFailedDays returns all days that were checked but failed verification.
func (r *IntegrityRepository) GetFailedDays(ctx context.Context, pairSymbol string) ([]IntegrityResult, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT pair_symbol, day, verified, gap_count, divergence_count, sources_checked, error_message
		FROM integrity_coverage
		WHERE pair_symbol = $1
		  AND verified = FALSE
		ORDER BY day
	`, pairSymbol)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var results []IntegrityResult
	for rows.Next() {
		var r IntegrityResult
		if err := rows.Scan(&r.PairSymbol, &r.Day, &r.Verified, &r.GapCount, &r.DivergenceCount, &r.SourcesChecked, &r.ErrorMessage); err != nil {
			return nil, err
		}
		results = append(results, r)
	}

	return results, rows.Err()
}
