package postgres

import (
	"context"
	"database/sql"
	"time"
)

type SyncState struct {
	HourlyLastSynced        time.Time
	HourlyRealtimeStartedAt time.Time
	DailyLastSynced         time.Time
	HourlyBackfillCompleted bool
	DailyBackfillCompleted  bool
}

type SyncRow struct {
	Pair                      string
	BackfillCompleted         bool
	LastSyncedUnix            int64
	NextTargetUnix            int64
	HourlyBackfillCompleted   bool
	DailyBackfillCompleted    bool
	HourlySyncedUnix          int64
	HourlyRealtimeStartedUnix int64
	DailySyncedUnix           int64
	NextHourlyTargetUnix      int64
	NextDailyTargetUnix       int64
	UpdatedAt                 time.Time
}

type SyncRepository struct {
	db *sql.DB
}

func NewSyncRepository(db *sql.DB) *SyncRepository {
	return &SyncRepository{db: db}
}

func (r *SyncRepository) States(ctx context.Context) (map[string]SyncState, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT pair_symbol,
		       hourly_last_synced_at,
		       hourly_realtime_started_at,
		       daily_last_synced_at,
		       hourly_backfill_completed,
		       daily_backfill_completed
		FROM sync_status
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	result := map[string]SyncState{}
	for rows.Next() {
		var pairSymbol string
		var state SyncState
		var hourlySynced sql.NullTime
		var hourlyRealtimeStarted sql.NullTime
		var dailySynced sql.NullTime
		if err := rows.Scan(&pairSymbol, &hourlySynced, &hourlyRealtimeStarted, &dailySynced, &state.HourlyBackfillCompleted, &state.DailyBackfillCompleted); err != nil {
			return nil, err
		}
		if hourlySynced.Valid {
			state.HourlyLastSynced = hourlySynced.Time.UTC()
		}
		if hourlyRealtimeStarted.Valid {
			state.HourlyRealtimeStartedAt = hourlyRealtimeStarted.Time.UTC()
		}
		if dailySynced.Valid {
			state.DailyLastSynced = dailySynced.Time.UTC()
		}
		if !state.HourlyBackfillCompleted {
			state.DailyBackfillCompleted = false
			state.DailyLastSynced = time.Time{}
		}
		result[pairSymbol] = state
	}

	return result, rows.Err()
}

func (r *SyncRepository) UpsertProgress(ctx context.Context, pairSymbol, interval string, lastSynced time.Time, backfillCompleted bool) error {
	switch interval {
	case "1d":
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO sync_status (
				pair_symbol,
				daily_last_synced_at,
				daily_backfill_completed,
				last_synced_at,
				backfill_completed,
				updated_at
			)
			VALUES ($1, $2, $3, $2, FALSE, NOW())
			ON CONFLICT (pair_symbol) DO UPDATE
			SET daily_last_synced_at = COALESCE(GREATEST(sync_status.daily_last_synced_at, EXCLUDED.daily_last_synced_at), EXCLUDED.daily_last_synced_at),
			    daily_backfill_completed = EXCLUDED.daily_backfill_completed,
			    backfill_completed = sync_status.hourly_backfill_completed AND EXCLUDED.daily_backfill_completed,
			    updated_at = NOW()
		`, pairSymbol, lastSynced.UTC(), backfillCompleted)
		return err
	default:
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO sync_status (
				pair_symbol,
				hourly_last_synced_at,
				hourly_backfill_completed,
				last_synced_at,
				backfill_completed,
				updated_at
			)
			VALUES ($1, $2, $3, $2, FALSE, NOW())
			ON CONFLICT (pair_symbol) DO UPDATE
			SET hourly_last_synced_at = COALESCE(GREATEST(sync_status.hourly_last_synced_at, EXCLUDED.hourly_last_synced_at), EXCLUDED.hourly_last_synced_at),
			    hourly_backfill_completed = CASE
			        WHEN EXCLUDED.hourly_backfill_completed THEN TRUE
			        WHEN sync_status.hourly_realtime_started_at IS NOT NULL
			             AND EXCLUDED.hourly_last_synced_at >= sync_status.hourly_realtime_started_at THEN TRUE
			        ELSE sync_status.hourly_backfill_completed
			    END,
			    last_synced_at = COALESCE(GREATEST(sync_status.last_synced_at, EXCLUDED.last_synced_at), EXCLUDED.last_synced_at),
			    backfill_completed = (
			        CASE
			            WHEN EXCLUDED.hourly_backfill_completed THEN TRUE
			            WHEN sync_status.hourly_realtime_started_at IS NOT NULL
			                 AND EXCLUDED.hourly_last_synced_at >= sync_status.hourly_realtime_started_at THEN TRUE
			            ELSE sync_status.hourly_backfill_completed
			        END
			    ) AND sync_status.daily_backfill_completed,
			    updated_at = NOW()
		`, pairSymbol, lastSynced.UTC(), backfillCompleted)
		return err
	}
}

func (r *SyncRepository) MarkRealtimeStarted(ctx context.Context, pairSymbol string, startedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sync_status (
			pair_symbol,
			hourly_realtime_started_at,
			updated_at
		)
		VALUES ($1, $2, NOW())
		ON CONFLICT (pair_symbol) DO UPDATE
		SET hourly_realtime_started_at = COALESCE(
		        LEAST(sync_status.hourly_realtime_started_at, EXCLUDED.hourly_realtime_started_at),
		        sync_status.hourly_realtime_started_at,
		        EXCLUDED.hourly_realtime_started_at
		    ),
		    updated_at = NOW()
	`, pairSymbol, startedAt.UTC())
	return err
}

func (r *SyncRepository) MarkBackfillSeeded(ctx context.Context, pairSymbol string, lastSynced time.Time) error {
	var lastSyncedArg any
	if !lastSynced.IsZero() {
		lastSyncedArg = lastSynced.UTC()
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sync_status (
			pair_symbol,
			last_synced_at,
			backfill_completed,
			hourly_last_synced_at,
			daily_last_synced_at,
			hourly_backfill_completed,
			daily_backfill_completed,
			updated_at
		)
		VALUES ($1, $2, FALSE, $2, NULL, FALSE, FALSE, NOW())
		ON CONFLICT (pair_symbol) DO NOTHING
	`, pairSymbol, lastSyncedArg)
	return err
}

func (r *SyncRepository) SnapshotRows(ctx context.Context) ([]SyncRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.symbol,
		       COALESCE(s.hourly_backfill_completed, FALSE) AND COALESCE(s.daily_backfill_completed, FALSE),
		       COALESCE(EXTRACT(EPOCH FROM s.hourly_last_synced_at)::BIGINT, 0),
		       EXTRACT(EPOCH FROM NOW() + INTERVAL '1 hour')::BIGINT,
		       COALESCE(s.hourly_backfill_completed, FALSE),
		       COALESCE(s.daily_backfill_completed, FALSE),
		       COALESCE(EXTRACT(EPOCH FROM s.hourly_last_synced_at)::BIGINT, 0),
		       COALESCE(EXTRACT(EPOCH FROM s.hourly_realtime_started_at)::BIGINT, 0),
		       COALESCE(EXTRACT(EPOCH FROM s.daily_last_synced_at)::BIGINT, 0),
		       EXTRACT(EPOCH FROM NOW() + INTERVAL '1 hour')::BIGINT,
		       EXTRACT(EPOCH FROM date_trunc('day', NOW() AT TIME ZONE 'UTC') + INTERVAL '1 day')::BIGINT,
		       COALESCE(s.updated_at, NOW())
		FROM pairs p
		LEFT JOIN sync_status s ON s.pair_symbol = p.symbol
		ORDER BY p.symbol
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []SyncRow
	for rows.Next() {
		var item SyncRow
		if err := rows.Scan(
			&item.Pair,
			&item.BackfillCompleted,
			&item.LastSyncedUnix,
			&item.NextTargetUnix,
			&item.HourlyBackfillCompleted,
			&item.DailyBackfillCompleted,
			&item.HourlySyncedUnix,
			&item.HourlyRealtimeStartedUnix,
			&item.DailySyncedUnix,
			&item.NextHourlyTargetUnix,
			&item.NextDailyTargetUnix,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if !item.HourlyBackfillCompleted {
			item.DailyBackfillCompleted = false
			item.DailySyncedUnix = 0
		}
		item.BackfillCompleted = item.HourlyBackfillCompleted && item.DailyBackfillCompleted
		items = append(items, item)
	}

	return items, rows.Err()
}
