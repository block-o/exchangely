package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/task"
)

const failedRetryDelay = "5 minutes"
const staleRunningTimeout = "30 minutes"

type TaskNotifier interface {
	NotifyUpdate()
}

type TaskRepository struct {
	db       *sql.DB
	workerID string
	notifier TaskNotifier
}

func NewTaskRepository(db *sql.DB, workerID string) *TaskRepository {
	return &TaskRepository{
		db:       db,
		workerID: workerID,
	}
}

func (r *TaskRepository) SetNotifier(n TaskNotifier) {
	r.notifier = n
}

func (r *TaskRepository) notify() {
	if r.notifier != nil {
		r.notifier.NotifyUpdate()
	}
}

func (r *TaskRepository) Enqueue(ctx context.Context, tasks []task.Task) ([]task.Task, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	enqueued := make([]task.Task, 0, len(tasks))
	for _, item := range tasks {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO tasks (id, task_type, pair_symbol, interval, window_start, window_end, status, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'pending', NOW())
			ON CONFLICT (id) DO NOTHING
		`, item.ID, item.Type, item.Pair, item.Interval, item.WindowStart.UTC(), item.WindowEnd.UTC())
		if err != nil {
			return nil, err
		}

		rows, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if rows == 1 {
			enqueued = append(enqueued, item)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if len(enqueued) > 0 {
		r.notify()
	}
	return enqueued, nil
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
		  AND (
			status = 'pending'
			OR (status = 'failed' AND updated_at <= NOW() - $3::interval)
			OR (status = 'running' AND updated_at <= NOW() - $4::interval)
		  )
	`, id, r.workerID, failedRetryDelay, staleRunningTimeout)
	if err != nil {
		return false, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	if rows == 1 {
		r.notify()
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
	if err == nil {
		r.notify()
	}
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
	if err == nil {
		r.notify()
	}
	return err
}

func (r *TaskRepository) Pending(ctx context.Context, limit int) ([]task.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_type, pair_symbol, interval, window_start, window_end
		FROM tasks
		WHERE status = 'pending'
		   OR (status = 'failed' AND updated_at <= NOW() - $2::interval)
		   OR (status = 'running' AND updated_at <= NOW() - $3::interval)
		ORDER BY CASE status WHEN 'pending' THEN 0 ELSE 1 END,
		         window_start,
		         CASE interval WHEN '1h' THEN 0 ELSE 1 END,
		         created_at
		LIMIT $1
	`, limit, failedRetryDelay, staleRunningTimeout)
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

// UpcomingTasks fetches tasks that have not yet been claimed, ordered by their intended window start.
func (r *TaskRepository) UpcomingTasks(ctx context.Context, limit int) ([]task.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_type, pair_symbol, interval, window_start, window_end, status, created_at
		FROM tasks
		WHERE status = 'pending'
		ORDER BY window_start ASC, created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []task.Task
	for rows.Next() {
		var item task.Task
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.Type, &item.Pair, &item.Interval, &item.WindowStart, &item.WindowEnd, &item.Status, &createdAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

// RecentTasks fetches most recently completed or failed tasks.
func (r *TaskRepository) RecentTasks(ctx context.Context, limit int) ([]task.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_type, pair_symbol, interval, window_start, window_end, status, last_error, updated_at
		FROM tasks
		WHERE status IN ('completed', 'failed')
		ORDER BY updated_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []task.Task
	for rows.Next() {
		var item task.Task
		var lastError sql.NullString
		var updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.Type, &item.Pair, &item.Interval, &item.WindowStart, &item.WindowEnd, &item.Status, &lastError, &updatedAt); err != nil {
			return nil, err
		}
		if lastError.Valid {
			item.LastError = lastError.String
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
