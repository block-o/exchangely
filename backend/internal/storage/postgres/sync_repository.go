package postgres

import (
	"context"
	"database/sql"
	"time"
)

type SyncState struct {
	LastSynced        time.Time
	BackfillCompleted bool
}

type SyncRow struct {
	Pair              string
	BackfillCompleted bool
	LastSyncedUnix    int64
	NextTargetUnix    int64
}

type SyncRepository struct {
	db *sql.DB
}

func NewSyncRepository(db *sql.DB) *SyncRepository {
	return &SyncRepository{db: db}
}

func (r *SyncRepository) States(ctx context.Context) (map[string]SyncState, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT pair_symbol, last_synced_at, backfill_completed
		FROM sync_status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string]SyncState{}
	for rows.Next() {
		var pairSymbol string
		var state SyncState
		var lastSynced sql.NullTime
		if err := rows.Scan(&pairSymbol, &lastSynced, &state.BackfillCompleted); err != nil {
			return nil, err
		}
		if lastSynced.Valid {
			state.LastSynced = lastSynced.Time.UTC()
		}
		result[pairSymbol] = state
	}

	return result, rows.Err()
}

func (r *SyncRepository) UpsertProgress(ctx context.Context, pairSymbol string, lastSynced time.Time, backfillCompleted bool) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sync_status (pair_symbol, last_synced_at, backfill_completed, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (pair_symbol) DO UPDATE
		SET last_synced_at = COALESCE(GREATEST(sync_status.last_synced_at, EXCLUDED.last_synced_at), EXCLUDED.last_synced_at),
		    backfill_completed = EXCLUDED.backfill_completed,
		    updated_at = NOW()
	`, pairSymbol, lastSynced.UTC(), backfillCompleted)
	return err
}

func (r *SyncRepository) MarkBackfillSeeded(ctx context.Context, pairSymbol string, lastSynced time.Time) error {
	var lastSyncedArg any
	if !lastSynced.IsZero() {
		lastSyncedArg = lastSynced.UTC()
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sync_status (pair_symbol, last_synced_at, backfill_completed, updated_at)
		VALUES ($1, $2, FALSE, NOW())
		ON CONFLICT (pair_symbol) DO NOTHING
	`, pairSymbol, lastSyncedArg)
	return err
}

func (r *SyncRepository) SnapshotRows(ctx context.Context) ([]SyncRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.symbol,
		       COALESCE(s.backfill_completed, FALSE),
		       COALESCE(EXTRACT(EPOCH FROM s.last_synced_at)::BIGINT, 0),
		       EXTRACT(EPOCH FROM NOW() + INTERVAL '1 hour')::BIGINT
		FROM pairs p
		LEFT JOIN sync_status s ON s.pair_symbol = p.symbol
		ORDER BY p.symbol
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SyncRow
	for rows.Next() {
		var item SyncRow
		if err := rows.Scan(&item.Pair, &item.BackfillCompleted, &item.LastSyncedUnix, &item.NextTargetUnix); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}
