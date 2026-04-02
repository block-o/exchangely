package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunningComposeStack(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("EXCHANGELY_E2E_BASE_URL"), "/")
	if baseURL == "" {
		t.Skip("EXCHANGELY_E2E_BASE_URL is not set")
	}

	client := &http.Client{Timeout: 5 * time.Second}

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
		var payload map[string]any
		getJSON(t, client, baseURL+"/api/v1/system/sync-status", &payload)
		if len(payload) == 0 {
			t.Fatal("expected sync status payload")
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
