package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestLoad(t *testing.T) {
	if os.Getenv("EXCHANGELY_RUN_LOAD_TEST") != "true" {
		t.Skip("EXCHANGELY_RUN_LOAD_TEST is not set")
	}

	baseURL := strings.TrimRight(os.Getenv("EXCHANGELY_E2E_BASE_URL"), "/")
	databaseURL := os.Getenv("EXCHANGELY_E2E_DATABASE_URL")

	client := &http.Client{Timeout: 10 * time.Second}
	db := openDB(t, databaseURL)
	defer func() {
		_ = db.Close()
	}()

	seededPair := seedMarketFixture(t, db)

	// Concurrency settings
	workers := 20
	iterations := 50
	var wg sync.WaitGroup

	errCh := make(chan error, workers*iterations*2)

	start := time.Now()
	t.Logf("Starting load test with %d workers and %d iterations per worker", workers, iterations)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// 1. Test individual ticker endpoint (should hit cache after first call)
				tickerURL := fmt.Sprintf(
					"%s/api/v1/ticker/%s",
					baseURL,
					seededPair,
				)
				if err := fetchAndValidate(client, tickerURL); err != nil {
					errCh <- fmt.Errorf("worker %d iteration %d ticker: %w", workerID, i, err)
				}

				// 2. Test global tickers endpoint (should hit cache for 30s)
				tickersURL := fmt.Sprintf("%s/api/v1/tickers", baseURL)
				if err := fetchAndValidate(client, tickersURL); err != nil {
					errCh <- fmt.Errorf("worker %d iteration %d global: %w", workerID, i, err)
				}
			}
		}(w)
	}

	wg.Wait()
	close(errCh)

	duration := time.Since(start)
	t.Logf("Load test completed in %v", duration)

	errorCount := 0
	for err := range errCh {
		t.Errorf("Request failed: %v", err)
		errorCount++
		if errorCount > 10 {
			t.Fatal("Too many errors, aborting")
		}
	}

	if errorCount == 0 {
		t.Log("All requests succeeded")
	}
}

func fetchAndValidate(client *http.Client, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	return nil
}
