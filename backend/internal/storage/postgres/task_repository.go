package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
			INSERT INTO tasks (id, task_type, pair_symbol, interval, window_start, window_end, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'pending', NOW(), NOW())
			ON CONFLICT (id) DO NOTHING
		`, item.ID, item.Type, item.Pair, item.Interval, item.WindowStart.UTC(), item.WindowEnd.UTC())
		if err != nil {
			return nil, err
		}

		rows, _ := result.RowsAffected()
		if rows > 0 {
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

func (r *TaskRepository) Claim(ctx context.Context, taskID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'running',
		    claimed_by = $1,
		    claimed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $2
		  AND status IN ('pending', 'failed')
	`, r.workerID, taskID)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		r.notify()
		return true, nil
	}
	return false, nil
}

func (r *TaskRepository) Complete(ctx context.Context, taskID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'completed',
		    completed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, taskID)
	if err == nil {
		r.notify()
	}
	return err
}

func (r *TaskRepository) Fail(ctx context.Context, taskID string, errStr string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'failed',
		    last_error = $1,
		    updated_at = NOW()
		WHERE id = $2
	`, errStr, taskID)
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
		if err := rows.Scan(&item.ID, &item.Type, &item.Pair, &item.Interval, &item.WindowStart, &item.WindowEnd); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

// UpcomingTasks fetches tasks that have not yet been claimed, ordered by their intended window start.
func (r *TaskRepository) UpcomingTasks(ctx context.Context, limit, offset int) ([]task.Task, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE status IN ('pending', 'running')").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_type, pair_symbol, interval, window_start, window_end, status, created_at
		FROM tasks
		WHERE status IN ('pending', 'running')
		ORDER BY CASE WHEN status = 'running' THEN 0 ELSE 1 END, window_start ASC, created_at ASC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []task.Task
	for rows.Next() {
		var item task.Task
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.Type, &item.Pair, &item.Interval, &item.WindowStart, &item.WindowEnd, &item.Status, &createdAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}

	return items, total, rows.Err()
}

// RecentTasks fetches most recently completed or failed tasks.
func (r *TaskRepository) RecentTasks(ctx context.Context, limit, offset int, types, statuses []string) ([]task.Task, int, error) {
	where := "status IN ('completed', 'failed')"
	args := []any{}

	if len(types) > 0 {
		placeholders := make([]string, len(types))
		for i := range types {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args = append(args, types[i])
		}
		where += fmt.Sprintf(" AND task_type IN (%s)", strings.Join(placeholders, ","))
	}

	if len(statuses) > 0 {
		placeholders := make([]string, len(statuses))
		for i := range statuses {
			placeholders[i] = fmt.Sprintf("$%d", len(args)+1)
			args = append(args, statuses[i])
		}
		where += fmt.Sprintf(" AND status IN (%s)", strings.Join(placeholders, ","))
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM tasks WHERE %s", where)
	var total int
	err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	limitIdx := len(args) + 1
	offsetIdx := len(args) + 2
	query := fmt.Sprintf(`
		SELECT id, task_type, pair_symbol, interval, window_start, window_end, status, last_error, updated_at
		FROM tasks
		WHERE %s
		ORDER BY updated_at DESC
		LIMIT $%d OFFSET $%d
	`, where, limitIdx, offsetIdx)

	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []task.Task
	for rows.Next() {
		var item task.Task
		var lastError sql.NullString
		var updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.Type, &item.Pair, &item.Interval, &item.WindowStart, &item.WindowEnd, &item.Status, &lastError, &updatedAt); err != nil {
			return nil, 0, err
		}
		if lastError.Valid {
			item.LastError = lastError.String
		}
		if !updatedAt.IsZero() {
			t := updatedAt.UTC()
			item.CompletedAt = &t
		}
		items = append(items, item)
	}

	return items, total, rows.Err()
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

// PruneOldTasks deletes completed and failed tasks whose updated_at is older than olderThan.
// This keeps the task log bounded. Returns the number of rows deleted.
func (r *TaskRepository) PruneOldTasks(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM tasks
		WHERE status IN ('completed', 'failed')
		  AND updated_at < $1
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
