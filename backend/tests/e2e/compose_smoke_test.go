package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestRunningComposeStack(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("EXCHANGELY_E2E_BASE_URL"), "/")
	if baseURL == "" {
		t.Skip("EXCHANGELY_E2E_BASE_URL is not set")
	}
	databaseURL := os.Getenv("EXCHANGELY_E2E_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("EXCHANGELY_E2E_DATABASE_URL is not set")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	db := openDB(t, databaseURL)
	defer db.Close()

	t.Run("health", func(t *testing.T) {
		var payload struct {
			Status string            `json:"status"`
			Checks map[string]string `json:"checks"`
		}
		getJSON(t, client, baseURL+"/api/v1/health", &payload)
		if payload.Status != "ok" {
			t.Fatalf("expected health status ok, got %q", payload.Status)
		}
		if payload.Checks["api"] != "ok" || payload.Checks["kafka"] != "ok" || payload.Checks["timescaledb"] != "ok" {
			t.Fatalf("unexpected health checks: %+v", payload.Checks)
		}
	})

	t.Run("catalog", func(t *testing.T) {
		var assets struct {
			Data []map[string]any `json:"data"`
		}
		getJSON(t, client, baseURL+"/api/v1/assets", &assets)
		if len(assets.Data) == 0 {
			t.Fatal("expected assets to be seeded")
		}

		var pairs struct {
			Data []map[string]any `json:"data"`
		}
		getJSON(t, client, baseURL+"/api/v1/pairs", &pairs)
		if len(pairs.Data) == 0 {
			t.Fatal("expected pairs to be seeded")
		}
	})

	t.Run("system sync status", func(t *testing.T) {
		var payload struct {
			PlannerLeader string `json:"planner_leader"`
			Pairs         []struct {
				Pair string `json:"pair"`
			} `json:"pairs"`
		}

		waitFor(t, 20*time.Second, func() bool {
			getJSON(t, client, baseURL+"/api/v1/system/sync-status", &payload)
			return payload.PlannerLeader != "" && payload.PlannerLeader != "unknown" && len(payload.Pairs) > 0
		})

		if payload.PlannerLeader == "" || payload.PlannerLeader == "unknown" {
			t.Fatalf("expected planner leader to be assigned, got %q", payload.PlannerLeader)
		}
		if len(payload.Pairs) == 0 {
			t.Fatal("expected sync status to include tracked pairs")
		}
	})

	t.Run("planner lease", func(t *testing.T) {
		waitFor(t, 20*time.Second, func() bool {
			holder, expiresAt, err := plannerLease(db, "planner_leader")
			if err != nil {
				t.Fatalf("planner lease query failed: %v", err)
			}
			return holder != "" && expiresAt.After(time.Now().UTC())
		})

		holder, expiresAt, err := plannerLease(db, "planner_leader")
		if err != nil {
			t.Fatalf("planner lease query failed: %v", err)
		}
		if holder == "" {
			t.Fatal("expected planner lease holder")
		}
		if !expiresAt.After(time.Now().UTC()) {
			t.Fatalf("expected active planner lease, expires_at=%s", expiresAt.Format(time.RFC3339))
		}
	})

	t.Run("task flow", func(t *testing.T) {
		waitFor(t, 20*time.Second, func() bool {
			total, claimed, completedOrFailed, err := taskCounts(db)
			if err != nil {
				t.Fatalf("task counts query failed: %v", err)
			}
			return total > 0 && claimed > 0 && completedOrFailed > 0
		})

		total, claimed, completedOrFailed, err := taskCounts(db)
		if err != nil {
			t.Fatalf("task counts query failed: %v", err)
		}
		if total == 0 {
			t.Fatal("expected planner to enqueue tasks")
		}
		if claimed == 0 {
			t.Fatal("expected worker to claim at least one task")
		}
		if completedOrFailed == 0 {
			t.Fatal("expected worker to advance at least one task out of pending/running")
		}
	})
}

func getJSON(t *testing.T, client *http.Client, url string, target any) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request for %s: %v", url, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request %s failed: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("request %s returned status %d", url, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decode %s failed: %v", url, err)
	}
}

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(1 * time.Second)
	}

	t.Fatalf("condition not met within %s", timeout)
}

func openDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Fatalf("ping database failed: %v", err)
	}

	return db
}

func taskCounts(db *sql.DB) (total int, claimed int, completedOrFailed int, err error) {
	row := db.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE claimed_by IS NOT NULL),
		       COUNT(*) FILTER (WHERE status IN ('completed', 'failed'))
		FROM tasks
	`)
	err = row.Scan(&total, &claimed, &completedOrFailed)
	return
}

func plannerLease(db *sql.DB, leaseName string) (holder string, expiresAt time.Time, err error) {
	row := db.QueryRow(`
		SELECT holder_id, expires_at
		FROM service_leases
		WHERE lease_name = $1
	`, leaseName)
	err = row.Scan(&holder, &expiresAt)
	return
}
