package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
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
	marketTopic := strings.TrimSpace(os.Getenv("EXCHANGELY_E2E_KAFKA_MARKET_TOPIC"))
	if marketTopic == "" {
		t.Skip("EXCHANGELY_E2E_KAFKA_MARKET_TOPIC is not set")
	}
	kafkaContainer := strings.TrimSpace(os.Getenv("EXCHANGELY_E2E_KAFKA_CONTAINER"))
	if kafkaContainer == "" {
		t.Skip("EXCHANGELY_E2E_KAFKA_CONTAINER is not set")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	db := openDB(t, databaseURL)
	defer func() {
		_ = db.Close()
	}()
	seededPair := seedMarketFixture(t, db)

	t.Run("health", func(t *testing.T) {
		var payload struct {
			Status string            `json:"status"`
			Checks map[string]string `json:"checks"`
		}
		getJSON(t, client, baseURL+"/api/v1/health", &payload)
		if payload.Status != "ok" {
			t.Fatalf("expected health status ok, got %q", payload.Status)
		}
		if payload.Checks["api"] != "ok" || payload.Checks["kafka"] != "ok" || payload.Checks["db"] != "ok" {
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
		var payload []struct {
			Pair string `json:"pair"`
		}

		waitFor(t, 20*time.Second, func() bool {
			getJSON(t, client, baseURL+"/api/v1/system/sync-status", &payload)
			return len(payload) > 0
		})
		if len(payload) == 0 {
			t.Fatal("expected sync status to include tracked pairs")
		}
	})

	t.Run("planner lease", func(t *testing.T) {
		waitFor(t, 40*time.Second, func() bool {
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

	t.Run("market api", func(t *testing.T) {
		start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		end := start.Add(24 * time.Hour)

		var historical struct {
			Data []struct {
				Pair      string  `json:"pair"`
				Interval  string  `json:"interval"`
				Timestamp int64   `json:"timestamp"`
				Close     float64 `json:"close"`
				Source    string  `json:"source"`
			} `json:"data"`
		}
		historicalURL := fmt.Sprintf(
			"%s/api/v1/historical/%s?interval=1h&start_time=%d&end_time=%d",
			baseURL,
			seededPair,
			start.Unix(),
			end.Unix(),
		)
		getJSON(t, client, historicalURL, &historical)
		if len(historical.Data) != 2 {
			t.Fatalf("expected 2 historical candles, got %d", len(historical.Data))
		}
		if historical.Data[0].Pair != seededPair || historical.Data[0].Source != "e2e-fixture" {
			t.Fatalf("unexpected historical payload: %+v", historical.Data[0])
		}

		var latest struct {
			Pair           string  `json:"pair"`
			Price          float64 `json:"price"`
			Variation24H   float64 `json:"variation_24h"`
			LastUpdateUnix int64   `json:"last_update_unix"`
			Source         string  `json:"source"`
		}
		getJSON(t, client, baseURL+"/api/v1/ticker/"+seededPair, &latest)
		if latest.Pair != seededPair || latest.Source != "e2e-fixture" {
			t.Fatalf("unexpected ticker payload: %+v", latest)
		}
		if latest.Price != 110 {
			t.Fatalf("expected ticker price 110, got %v", latest.Price)
		}
		if latest.Variation24H != 10 {
			t.Fatalf("expected 24h variation 10, got %v", latest.Variation24H)
		}
		if latest.LastUpdateUnix != end.Unix() {
			t.Fatalf("expected ticker timestamp %d, got %d", end.Unix(), latest.LastUpdateUnix)
		}
	})

	t.Run("realtime market events", func(t *testing.T) {
		realtimePair := "E2EREALTIMEUSDT"
		realtimeTime := time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC)
		realtimeCandle := candle.Candle{
			Pair:      realtimePair,
			Interval:  "1h",
			Timestamp: realtimeTime.Unix(),
			Open:      201,
			High:      208,
			Low:       199,
			Close:     205,
			Volume:    12.5,
			Source:    "e2e-market-event",
			Finalized: false,
		}

		t.Cleanup(func() {
			_, _ = db.Exec(`DELETE FROM raw_candles WHERE pair_symbol = $1`, realtimePair)
			_, _ = db.Exec(`DELETE FROM candles_1h WHERE pair_symbol = $1`, realtimePair)
		})

		publishCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := publishMarketEvent(publishCtx, kafkaContainer, marketTopic, realtimeCandle); err != nil {
			t.Fatalf("publish realtime market event failed: %v", err)
		}

		waitFor(t, 20*time.Second, func() bool {
			rawCount, hourlyCount, err := realtimeCounts(db, realtimePair, realtimeTime)
			if err != nil {
				t.Fatalf("realtime counts query failed: %v", err)
			}
			return rawCount > 0 && hourlyCount > 0
		})

		rawCount, hourlyCount, err := realtimeCounts(db, realtimePair, realtimeTime)
		if err != nil {
			t.Fatalf("realtime counts query failed: %v", err)
		}
		if rawCount != 1 {
			t.Fatalf("expected 1 raw realtime candle, got %d", rawCount)
		}
		if hourlyCount != 1 {
			t.Fatalf("expected 1 consolidated realtime candle, got %d", hourlyCount)
		}

		var historical struct {
			Data []struct {
				Pair      string  `json:"pair"`
				Interval  string  `json:"interval"`
				Timestamp int64   `json:"timestamp"`
				Close     float64 `json:"close"`
				Source    string  `json:"source"`
			} `json:"data"`
		}
		historicalURL := fmt.Sprintf(
			"%s/api/v1/historical/%s?interval=1h&start_time=%d&end_time=%d",
			baseURL,
			realtimePair,
			realtimeTime.Add(-time.Hour).Unix(),
			realtimeTime.Add(time.Hour).Unix(),
		)
		getJSON(t, client, historicalURL, &historical)
		if len(historical.Data) != 1 {
			t.Fatalf("expected 1 realtime historical candle, got %d", len(historical.Data))
		}
		if historical.Data[0].Source != "consolidated" || historical.Data[0].Close != realtimeCandle.Close {
			t.Fatalf("unexpected realtime historical payload: %+v", historical.Data[0])
		}

		var latest struct {
			Pair           string  `json:"pair"`
			Price          float64 `json:"price"`
			LastUpdateUnix int64   `json:"last_update_unix"`
			Source         string  `json:"source"`
		}
		getJSON(t, client, baseURL+"/api/v1/ticker/"+realtimePair, &latest)
		if latest.Pair != realtimePair || latest.Source != "consolidated" {
			t.Fatalf("unexpected realtime ticker payload: %+v", latest)
		}
		if latest.Price != realtimeCandle.Close {
			t.Fatalf("expected realtime ticker price %v, got %v", realtimeCandle.Close, latest.Price)
		}
		if latest.LastUpdateUnix != realtimeTime.Unix() {
			t.Fatalf("expected realtime ticker timestamp %d, got %d", realtimeTime.Unix(), latest.LastUpdateUnix)
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
	defer func() {
		_ = resp.Body.Close()
	}()

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
		_ = db.Close()
		t.Fatalf("ping database failed: %v", err)
	}

	return db
}

func seedMarketFixture(t *testing.T, db *sql.DB) string {
	t.Helper()

	pairSymbol := "E2ETESTUSDT"
	first := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	second := first.Add(24 * time.Hour)

	for _, item := range []struct {
		timestamp time.Time
		open      float64
		high      float64
		low       float64
		close     float64
	}{
		{timestamp: first, open: 100, high: 102, low: 99, close: 100},
		{timestamp: second, open: 105, high: 111, low: 104, close: 110},
	} {
		if _, err := db.Exec(`
			INSERT INTO candles_1h (pair_symbol, bucket_start, open, high, low, close, volume, source, finalized)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE)
			ON CONFLICT (pair_symbol, bucket_start) DO UPDATE
			SET open = EXCLUDED.open,
			    high = EXCLUDED.high,
			    low = EXCLUDED.low,
			    close = EXCLUDED.close,
			    volume = EXCLUDED.volume,
			    source = EXCLUDED.source,
			    finalized = EXCLUDED.finalized
		`,
			pairSymbol,
			item.timestamp,
			item.open,
			item.high,
			item.low,
			item.close,
			100.0,
			"e2e-fixture",
		); err != nil {
			t.Fatalf("seed market fixture failed: %v", err)
		}
	}

	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM candles_1h WHERE pair_symbol = $1`, pairSymbol)
	})

	return pairSymbol
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

func realtimeCounts(db *sql.DB, pair string, bucketStart time.Time) (rawCount int, hourlyCount int, err error) {
	row := db.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE source = 'e2e-market-event'),
			(
				SELECT COUNT(*)
				FROM candles_1h
				WHERE pair_symbol = $1
				  AND bucket_start = $2
			)
		FROM raw_candles
		WHERE pair_symbol = $1
		  AND interval = '1h'
		  AND bucket_start = $2
	`, pair, bucketStart.UTC())
	err = row.Scan(&rawCount, &hourlyCount)
	return
}

func publishMarketEvent(ctx context.Context, kafkaContainer, topic string, item candle.Candle) error {
	body, err := json.Marshal(item)
	if err != nil {
		return err
	}

	command := exec.CommandContext(
		ctx,
		"docker",
		"exec",
		kafkaContainer,
		"/bin/sh",
		"-lc",
		fmt.Sprintf(
			"printf '%%s\\n' '%s|%s' | /opt/kafka/bin/kafka-console-producer.sh --bootstrap-server localhost:9092 --topic %s --property parse.key=true --property key.separator='|' >/dev/null",
			item.Pair,
			string(body),
			topic,
		),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
