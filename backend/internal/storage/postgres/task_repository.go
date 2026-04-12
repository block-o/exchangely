package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/task"
)

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
	defer func() {
		_ = tx.Rollback()
	}()

	enqueued := make([]task.Task, 0, len(tasks))
	for _, item := range tasks {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO tasks (id, task_type, pair_symbol, interval, window_start, window_end, description, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', NOW(), NOW())
			ON CONFLICT (id) DO UPDATE
			SET status = 'pending',
			    window_start = EXCLUDED.window_start,
			    window_end = EXCLUDED.window_end,
			    description = EXCLUDED.description,
			    retry_count = 0,
			    retry_at = NULL,
			    last_error = NULL,
			    claimed_by = NULL,
			    claimed_at = NULL,
			    completed_at = NULL,
			    updated_at = NOW()
			WHERE tasks.status IN ('completed', 'failed')
			   OR (tasks.status = 'running' AND tasks.task_type = 'live_ticker' AND tasks.claimed_at < NOW() - INTERVAL '30 seconds')
			   OR (tasks.status = 'pending' AND tasks.retry_at IS NOT NULL AND tasks.retry_at > NOW())
		`, item.ID, item.Type, item.Pair, item.Interval, item.WindowStart.UTC(), item.WindowEnd.UTC(), item.Description)
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
	// Calculate next retry with jitter (1h +/- 5m)
	jitterSeconds := rand.Intn(601) - 300 // -300 to +300 seconds
	nextRetryAt := time.Now().Add(time.Hour + time.Duration(jitterSeconds)*time.Second).UTC()

	_, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = CASE 
		             WHEN (interval = '1h' AND retry_count < 24) OR (interval = '1d' AND retry_count < 7) THEN 'pending'
		             ELSE 'failed'
		         END,
		    retry_at = CASE 
		             WHEN (interval = '1h' AND retry_count < 24) OR (interval = '1d' AND retry_count < 7) THEN $1
		             ELSE retry_at
		         END,
		    retry_count = retry_count + 1,
		    last_error = $2,
		    updated_at = NOW()
		WHERE id = $3
	`, nextRetryAt, errStr, taskID)
	if err == nil {
		r.notify()
	}
	return err
}

// ActiveBackfillPairs returns the set of pair symbols that currently have at
// least one pending or running backfill task that is actually actionable.
// Pending tasks waiting for a retry cooldown (retry_at in the future) are
// excluded so the planner can generate fresh work for those pairs instead
// of leaving them idle until the retry window opens.
func (r *TaskRepository) ActiveBackfillPairs(ctx context.Context) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT pair_symbol
		FROM tasks
		WHERE task_type = $1
		  AND (
		      status = 'running'
		      OR (status = 'pending' AND (retry_at IS NULL OR retry_at <= NOW()))
		  )
	`, task.TypeBackfill)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	active := make(map[string]bool)
	for rows.Next() {
		var sym string
		if err := rows.Scan(&sym); err != nil {
			return nil, err
		}
		active[sym] = true
	}
	return active, rows.Err()
}

func (r *TaskRepository) Pending(ctx context.Context, limit, backfillLimit int) ([]task.Task, error) {
	if limit <= 0 {
		return nil, nil
	}
	if backfillLimit < 0 {
		backfillLimit = 0
	}
	if backfillLimit > limit {
		backfillLimit = limit
	}

	// Phase 1: Always fetch live_ticker tasks first — they get priority slots.
	realtimeItems, err := r.pendingByTaskType(ctx, limit, task.TypeRealtime)
	if err != nil {
		return nil, err
	}

	remaining := limit - len(realtimeItems)
	if remaining <= 0 {
		return realtimeItems, nil
	}

	// Phase 2: Fetch non-backfill, non-realtime tasks.
	otherItems, err := r.pendingNonBackfillNonRealtime(ctx, remaining)
	if err != nil {
		return nil, err
	}

	items := append(realtimeItems, otherItems...)
	remaining = limit - len(items)
	if remaining <= 0 || backfillLimit == 0 {
		return items, nil
	}
	if backfillLimit > remaining {
		backfillLimit = remaining
	}

	// Phase 3: Fill remaining slots with backfill tasks.
	backfillItems, err := r.pendingByType(ctx, backfillLimit, true)
	if err != nil {
		return nil, err
	}

	return append(items, backfillItems...), nil
}

func (r *TaskRepository) pendingByTaskType(ctx context.Context, limit int, taskType string) ([]task.Task, error) {
	if limit <= 0 {
		return nil, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_type, pair_symbol, interval, window_start, window_end, description
		FROM tasks
		WHERE status = 'pending'
		  AND (retry_at IS NULL OR retry_at <= NOW())
		  AND task_type = $2
		ORDER BY created_at ASC
		LIMIT $1
	`, limit, taskType)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []task.Task
	for rows.Next() {
		var item task.Task
		if err := rows.Scan(&item.ID, &item.Type, &item.Pair, &item.Interval, &item.WindowStart, &item.WindowEnd, &item.Description); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *TaskRepository) pendingNonBackfillNonRealtime(ctx context.Context, limit int) ([]task.Task, error) {
	if limit <= 0 {
		return nil, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_type, pair_symbol, interval, window_start, window_end, description
		FROM tasks
		WHERE status = 'pending'
		  AND (retry_at IS NULL OR retry_at <= NOW())
		  AND task_type <> $1
		  AND task_type <> $2
		ORDER BY CASE task_type
		             WHEN 'integrity_check' THEN 0
		             WHEN 'consolidation' THEN 1
		             ELSE 2
		         END,
		         window_start ASC,
		         created_at ASC
		LIMIT $3
	`, task.TypeBackfill, task.TypeRealtime, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []task.Task
	for rows.Next() {
		var item task.Task
		if err := rows.Scan(&item.ID, &item.Type, &item.Pair, &item.Interval, &item.WindowStart, &item.WindowEnd, &item.Description); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *TaskRepository) pendingByType(ctx context.Context, limit int, backfillOnly bool) ([]task.Task, error) {
	if limit <= 0 {
		return nil, nil
	}

	operator := "<>"
	if backfillOnly {
		operator = "="
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_type, pair_symbol, interval, window_start, window_end, description
		FROM tasks
		WHERE status = 'pending'
		  AND (retry_at IS NULL OR retry_at <= NOW())
		  AND task_type `+operator+` $2
		ORDER BY CASE task_type
		             WHEN 'live_ticker' THEN 0
		             WHEN 'integrity_check' THEN 1
		             WHEN 'consolidation' THEN 2
		             ELSE 3
		         END,
		         window_start ASC,
		         created_at ASC
		LIMIT $1
	`, limit, task.TypeBackfill)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []task.Task
	for rows.Next() {
		var item task.Task
		if err := rows.Scan(&item.ID, &item.Type, &item.Pair, &item.Interval, &item.WindowStart, &item.WindowEnd, &item.Description); err != nil {
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
		SELECT id, task_type, pair_symbol, interval, window_start, window_end, status, description, created_at
		FROM tasks
		WHERE status IN ('pending', 'running')
		ORDER BY CASE WHEN status = 'running' THEN 0 ELSE 1 END,
		         CASE task_type
		             WHEN 'live_ticker' THEN 0
		             WHEN 'integrity_check' THEN 1
		             WHEN 'consolidation' THEN 2
		             ELSE 3
		         END,
		         window_start ASC,
		         created_at ASC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []task.Task
	for rows.Next() {
		var item task.Task
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.Type, &item.Pair, &item.Interval, &item.WindowStart, &item.WindowEnd, &item.Status, &item.Description, &createdAt); err != nil {
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
	defer func() {
		_ = rows.Close()
	}()

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

// PruneOldTasks deletes completed and failed tasks. It first removes tasks older than olderThan,
// and then further prunes the log to ensure no more than keepMax completed/failed tasks remain.
// This keeps the task log bounded by both time (retention period) and volume (max count).
// Returns the total number of rows deleted.
func (r *TaskRepository) PruneOldTasks(ctx context.Context, olderThan time.Duration, keepMax int) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Duration-based prune
	cutoff := time.Now().UTC().Add(-olderThan)
	res1, err := tx.ExecContext(ctx, `
		DELETE FROM tasks
		WHERE status IN ('completed', 'failed')
		  AND updated_at < $1
	`, cutoff)
	if err != nil {
		return 0, err
	}
	deleted1, _ := res1.RowsAffected()

	// 2. Count-based prune
	var deleted2 int64
	if keepMax > 0 {
		res2, err := tx.ExecContext(ctx, `
			DELETE FROM tasks
			WHERE id IN (
				SELECT id
				FROM tasks
				WHERE status IN ('completed', 'failed')
				ORDER BY updated_at DESC
				OFFSET $1
			)
		`, keepMax)
		if err != nil {
			return deleted1, err
		}
		deleted2, _ = res2.RowsAffected()
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return deleted1 + deleted2, nil
}
