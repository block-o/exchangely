package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/task"
)

type TaskRepository struct {
	db       *sql.DB
	workerID string
}

func NewTaskRepository(db *sql.DB, workerID string) *TaskRepository {
	return &TaskRepository{
		db:       db,
		workerID: workerID,
	}
}

func (r *TaskRepository) Enqueue(ctx context.Context, tasks []task.Task) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range tasks {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tasks (id, task_type, pair_symbol, interval, window_start, window_end, status, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'pending', NOW())
			ON CONFLICT (id) DO NOTHING
		`, item.ID, item.Type, item.Pair, item.Interval, item.WindowStart.UTC(), item.WindowEnd.UTC()); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *TaskRepository) Claim(ctx context.Context, id string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'running',
		    claimed_by = $2,
		    claimed_at = NOW(),
		    updated_at = NOW(),
		    last_error = NULL
		WHERE id = $1
		  AND status IN ('pending', 'failed')
	`, id, r.workerID)
	if err != nil {
		return false, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rows == 1, nil
}

func (r *TaskRepository) Complete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'completed',
		    completed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

func (r *TaskRepository) Fail(ctx context.Context, id, reason string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'failed',
		    last_error = $2,
		    updated_at = NOW()
		WHERE id = $1
	`, id, reason)
	return err
}

func (r *TaskRepository) Pending(ctx context.Context, limit int) ([]task.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_type, pair_symbol, interval, window_start, window_end
		FROM tasks
		WHERE status IN ('pending', 'failed')
		ORDER BY window_start, created_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []task.Task
	for rows.Next() {
		var item task.Task
		if err := rows.Scan(&item.ID, &item.Type, &item.Pair, &item.Interval, &item.WindowStart, &item.WindowEnd); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *TaskRepository) MarkBackfillSeeded(ctx context.Context, pairSymbol string, lastSynced time.Time) error {
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
