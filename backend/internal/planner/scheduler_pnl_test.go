package planner

import (
	"fmt"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/google/uuid"
)

func TestBuildPnLRefreshTasks_GeneratesCorrectIDs(t *testing.T) {
	s := NewScheduler(5*time.Second, 5*time.Minute, 24*time.Hour, 24*time.Hour, 1*time.Hour)
	userID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	tasks := s.BuildPnLRefreshTasks([]uuid.UUID{userID}, now)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	expectedID := fmt.Sprintf("%s:%s:periodic", task.TypePnLRefresh, userID.String())
	if tasks[0].ID != expectedID {
		t.Fatalf("expected ID %q, got %q", expectedID, tasks[0].ID)
	}
}

func TestBuildPnLRefreshTasks_EmptyUsers(t *testing.T) {
	s := NewScheduler(5*time.Second, 5*time.Minute, 24*time.Hour, 24*time.Hour, 1*time.Hour)
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	tasks := s.BuildPnLRefreshTasks([]uuid.UUID{}, now)

	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks for empty user list, got %d", len(tasks))
	}
}

func TestBuildPnLRefreshTasks_MultipleUsers(t *testing.T) {
	s := NewScheduler(5*time.Second, 5*time.Minute, 24*time.Hour, 24*time.Hour, 1*time.Hour)
	users := []uuid.UUID{
		uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		uuid.MustParse("33333333-3333-3333-3333-333333333333"),
	}
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	tasks := s.BuildPnLRefreshTasks(users, now)

	if len(tasks) != len(users) {
		t.Fatalf("expected %d tasks, got %d", len(users), len(tasks))
	}

	seen := make(map[string]bool)
	for _, tsk := range tasks {
		if seen[tsk.ID] {
			t.Fatalf("duplicate task ID: %s", tsk.ID)
		}
		seen[tsk.ID] = true
	}

	for _, uid := range users {
		expectedID := fmt.Sprintf("%s:%s:periodic", task.TypePnLRefresh, uid.String())
		if !seen[expectedID] {
			t.Fatalf("missing task for user %s", uid.String())
		}
	}
}

func TestBuildPnLRefreshTasks_TaskFields(t *testing.T) {
	interval := 2 * time.Hour
	s := NewScheduler(5*time.Second, 5*time.Minute, 24*time.Hour, 24*time.Hour, interval)
	userID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	tasks := s.BuildPnLRefreshTasks([]uuid.UUID{userID}, now)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	tsk := tasks[0]

	if tsk.Type != task.TypePnLRefresh {
		t.Fatalf("expected type %q, got %q", task.TypePnLRefresh, tsk.Type)
	}

	if tsk.Interval != interval.String() {
		t.Fatalf("expected interval %q, got %q", interval.String(), tsk.Interval)
	}

	expectedStart := now.UTC().Truncate(interval)
	expectedEnd := expectedStart.Add(interval)

	if !tsk.WindowStart.Equal(expectedStart) {
		t.Fatalf("expected WindowStart %v, got %v", expectedStart, tsk.WindowStart)
	}
	if !tsk.WindowEnd.Equal(expectedEnd) {
		t.Fatalf("expected WindowEnd %v, got %v", expectedEnd, tsk.WindowEnd)
	}

	if tsk.Pair != userID.String() {
		t.Fatalf("expected Pair %q, got %q", userID.String(), tsk.Pair)
	}

	if tsk.Description == "" {
		t.Fatal("expected non-empty description")
	}
}
