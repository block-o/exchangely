package router

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/block-o/exchangely/backend/internal/domain/ticker"
	"github.com/block-o/exchangely/backend/internal/httpapi/dto"
	"github.com/block-o/exchangely/backend/internal/httpapi/handlers"
	"github.com/block-o/exchangely/backend/internal/httpapi/middleware"
	"github.com/block-o/exchangely/backend/internal/service"
)

type Services struct {
	Catalog *service.CatalogService
	Market  *service.MarketService
	System  *service.SystemService
	News    *service.NewsService
	Auth    *auth.Service // nil when auth is disabled
}

type Options struct {
	AllowedOrigins []string
	Env            string   // "development" or "production"
	TrustedProxies []string // CIDR ranges or IPs whose X-Forwarded-For / X-Real-IP headers are trusted
}

func New(svcs Services, opts Options) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, svcs.System.Health(r.Context()))
	})

	mux.HandleFunc("/api/v1/assets", func(w http.ResponseWriter, r *http.Request) {
		items, err := svcs.Catalog.Assets(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dto.ListResponse[any]{Data: toAnySlice(items)})
	})

	mux.HandleFunc("/api/v1/pairs", func(w http.ResponseWriter, r *http.Request) {
		items, err := svcs.Catalog.Pairs(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dto.ListResponse[any]{Data: toAnySlice(items)})
	})

	// GET /api/v1/historical/{pair} — returns canonical stored OHLCV data for the requested interval.
	// This endpoint is resolution-driven (`1h`, `1d`) and is intentionally separate from live ticker reads.
	mux.HandleFunc("/api/v1/historical/", func(w http.ResponseWriter, r *http.Request) {
		pairSymbol := strings.TrimPrefix(r.URL.Path, "/api/v1/historical/")
		interval := r.URL.Query().Get("interval")
		if pairSymbol == "" || interval == "" {
			http.Error(w, "pair and interval are required", http.StatusBadRequest)
			return
		}

		start := parseUnix(r.URL.Query().Get("start_time"))
		end := parseUnix(r.URL.Query().Get("end_time"))
		items, err := svcs.Market.Historical(r.Context(), pairSymbol, interval, start, end)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dto.ListResponse[any]{Data: toAnySlice(items)})
	})

	// GET /api/v1/ticker/{pair} — returns the freshest persisted ticker point for one pair.
	// Price/last_update prefer the newest realtime raw sample over the current hourly aggregate.
	mux.HandleFunc("/api/v1/ticker/", func(w http.ResponseWriter, r *http.Request) {
		pairSymbol := strings.TrimPrefix(r.URL.Path, "/api/v1/ticker/")
		if pairSymbol == "" {
			http.Error(w, "pair is required", http.StatusBadRequest)
			return
		}

		item, err := svcs.Market.Ticker(r.Context(), pairSymbol)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	// GET /api/v1/tickers — returns the freshest persisted ticker point plus 24h stats for all pairs.
	// Price/last_update prefer the newest realtime raw sample over the current hourly aggregate.
	mux.HandleFunc("/api/v1/tickers", func(w http.ResponseWriter, r *http.Request) {
		items, err := svcs.Market.Tickers(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dto.ListResponse[any]{Data: toAnySlice(items)})
	})

	// GET /api/v1/tickers/stream — Server-Sent Events (SSE) endpoint.
	// Accepts an optional ?quote= query parameter to filter tickers by quote currency.
	mux.HandleFunc("/api/v1/tickers/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		quoteFilter := strings.ToUpper(r.URL.Query().Get("quote"))

		ctx := r.Context()
		updates := svcs.Market.Subscribe()
		defer svcs.Market.Unsubscribe(updates)
		flusher.Flush()

		// Track last emitted state per pair so we only send actual changes.
		lastSent := make(map[string]ticker.Ticker)

		for {
			select {
			case <-ctx.Done():
				return
			case <-updates.Updates():
				pairs := updates.DrainPendingPairs()
				if len(pairs) == 0 {
					continue
				}

				items := make([]any, 0, len(pairs))
				for _, pairSymbol := range pairs {
					if quoteFilter != "" && !strings.HasSuffix(pairSymbol, quoteFilter) {
						continue
					}
					item, err := svcs.Market.Ticker(ctx, pairSymbol)
					if err != nil {
						slog.Warn("ticker delta load failed", "pair", pairSymbol, "error", err)
						continue
					}
					if prev, ok := lastSent[pairSymbol]; ok && prev == item {
						continue
					}
					lastSent[pairSymbol] = item
					items = append(items, item)
				}
				if len(items) == 0 {
					continue
				}

				data, _ := json.Marshal(map[string]any{"tickers": items})
				if err := writeSSEData(w, data); err != nil {
					slog.Warn("ticker stream write failed", "error", err)
					return
				}
				flusher.Flush()
			}
		}
	})

	mux.HandleFunc("/api/v1/system/sync-status", func(w http.ResponseWriter, r *http.Request) {
		item, err := svcs.System.SyncSnapshot(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	mux.HandleFunc("/api/v1/system/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"api_version": "v1.0.0",
		})
	})

	// GET /api/v1/news — returns the latest news articles.
	mux.HandleFunc("/api/v1/news", func(w http.ResponseWriter, r *http.Request) {
		limit := getIntParam(r, "limit", 50)
		items, err := svcs.News.List(r.Context(), limit)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dto.ListResponse[any]{Data: toAnySlice(items)})
	})

	// GET /api/v1/news/stream — Server-Sent Events (SSE) endpoint for news updates.
	mux.HandleFunc("/api/v1/news/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		ctx := r.Context()
		updates := svcs.News.Subscribe()
		defer svcs.News.Unsubscribe(updates)

		// Initial push
		items, _ := svcs.News.List(ctx, 50)
		data, _ := json.Marshal(map[string]any{"news": items})
		if err := writeSSEData(w, data); err != nil {
			slog.Warn("initial news stream write failed", "error", err)
			return
		}
		flusher.Flush()

		for {
			select {
			case <-ctx.Done():
				return
			case <-updates:
				items, _ := svcs.News.List(ctx, 50)
				data, _ := json.Marshal(map[string]any{"news": items})
				if err := writeSSEData(w, data); err != nil {
					slog.Warn("news stream write failed", "error", err)
					return
				}
				flusher.Flush()
			}
		}
	})

	mux.HandleFunc("/api/v1/system/warnings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			items, err := svcs.System.ActiveWarnings(r.Context())
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, items)
		case http.MethodPost:
			var payload struct {
				ID          string `json:"id"`
				Fingerprint string `json:"fingerprint"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "invalid warning dismiss payload", http.StatusBadRequest)
				return
			}
			if payload.ID == "" || payload.Fingerprint == "" {
				http.Error(w, "id and fingerprint are required", http.StatusBadRequest)
				return
			}
			if err := svcs.System.DismissWarning(r.Context(), payload.ID, payload.Fingerprint); err != nil {
				writeError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPost}, ", "))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/system/tasks", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		defaultLimit := positiveInt(getIntParam(r, "limit", 50), 50)
		upcomingLimit := positiveInt(getIntParam(r, "upcoming_limit", defaultLimit), defaultLimit)
		recentLimit := positiveInt(getIntParam(r, "recent_limit", defaultLimit), defaultLimit)
		defaultPage := positiveInt(getIntParam(r, "page", 1), 1)
		upcomingPage := positiveInt(getIntParam(r, "upcoming_page", defaultPage), defaultPage)
		recentPage := positiveInt(getIntParam(r, "recent_page", defaultPage), defaultPage)
		upcomingOffset := (upcomingPage - 1) * upcomingLimit
		recentOffset := (recentPage - 1) * recentLimit

		typesStr := r.URL.Query().Get("type")
		var types []string
		if typesStr != "" {
			types = strings.Split(typesStr, ",")
		}
		statusesStr := r.URL.Query().Get("status")
		var statuses []string
		if statusesStr != "" {
			statuses = strings.Split(statusesStr, ",")
		}

		upcoming, upTotal, err := svcs.System.UpcomingTasks(ctx, upcomingLimit, upcomingOffset)
		if err != nil {
			writeError(w, err)
			return
		}
		recent, recTotal, err := svcs.System.RecentTasks(ctx, recentLimit, recentOffset, types, statuses)
		if err != nil {
			writeError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"upcoming":      upcoming,
			"upcomingTotal": upTotal,
			"upcomingLimit": upcomingLimit,
			"upcomingPage":  upcomingPage,
			"recent":        recent,
			"recentTotal":   recTotal,
			"recentLimit":   recentLimit,
			"recentPage":    recentPage,
		})
	})

	mux.HandleFunc("/api/v1/system/tasks/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		ctx := r.Context()
		defaultLimit := positiveInt(getIntParam(r, "limit", 50), 50)
		upcomingLimit := positiveInt(getIntParam(r, "upcoming_limit", defaultLimit), defaultLimit)
		recentLimit := positiveInt(getIntParam(r, "recent_limit", defaultLimit), defaultLimit)

		typesStr := r.URL.Query().Get("type")
		var types []string
		if typesStr != "" {
			types = strings.Split(typesStr, ",")
		}
		statusesStr := r.URL.Query().Get("status")
		var statuses []string
		if statusesStr != "" {
			statuses = strings.Split(statusesStr, ",")
		}

		ch := make(chan service.TaskStreamSnapshot)

		go func() {
			if err := svcs.System.StreamTasks(ctx, ch, upcomingLimit, recentLimit, types, statuses); err != nil {
				slog.Warn("task stream ended", "error", err)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case snapshot := <-ch:
				data, _ := json.Marshal(snapshot)
				if err := writeSSEData(w, data); err != nil {
					slog.Warn("task stream write failed", "error", err)
					return
				}
				flusher.Flush()
			}
		}
	})

	mux.HandleFunc("/swagger", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/swagger" {
			http.NotFound(w, r)
			return
		}
		serveSwaggerPage(w)
	})

	// Auth endpoints — only registered when auth is enabled.
	if svcs.Auth != nil {
		env := "production"
		if opts.Env != "" {
			env = opts.Env
		}
		authHandler := handlers.NewAuthHandler(svcs.Auth, env)

		mux.HandleFunc("/api/v1/auth/google/login", authHandler.GoogleLogin)
		mux.HandleFunc("/api/v1/auth/google/callback", authHandler.GoogleCallback)
		mux.HandleFunc("/api/v1/auth/local/login", authHandler.LocalLogin)
		mux.HandleFunc("/api/v1/auth/refresh", authHandler.Refresh)
		mux.HandleFunc("/api/v1/auth/logout", authHandler.Logout)
		mux.HandleFunc("/api/v1/auth/me", authHandler.Me)
		mux.HandleFunc("/api/v1/auth/local/change-password", authHandler.ChangePassword)
		mux.HandleFunc("/api/v1/auth/methods", authHandler.AuthMethods)
	}

	mux.HandleFunc("/swagger/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join("..", "docs", "openapi", "openapi.yaml")
		if _, err := os.Stat(path); err == nil {
			http.ServeFile(w, r, path)
			return
		}
		path = filepath.Join("docs", "openapi", "openapi.yaml")
		if _, err := os.Stat(path); err == nil {
			http.ServeFile(w, r, path)
			return
		}
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write([]byte(defaultOpenAPIYAML()))
	})

	// Wrap with auth middleware. When svcs.Auth is nil (auth disabled),
	// the middleware passes all requests through unchanged.
	authMW := middleware.NewAuthMiddleware(svcs.Auth)
	realIPMW := middleware.NewRealIPMiddleware(opts.TrustedProxies)
	return withAccessLog(withCORS(realIPMW.Wrap(authMW.Wrap(mux)), opts.AllowedOrigins))
}

func serveSwaggerPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerPageHTML()))
}

func defaultOpenAPIYAML() string {
	return `openapi: 3.0.3
info:
  title: Exchangely API
  version: 0.1.0
  description: REST API for Exchangely market data and sync state. Github (https://github.com/block-o/exchangely)
servers:
  - url: http://localhost:8080
paths:
  /api/v1/health:
    get:
      tags: [System]
      summary: Health status
      responses:
        "200":
          description: Service health
          content:
            application/json:
              schema: { $ref: "#/components/schemas/HealthStatus" }
  /api/v1/assets:
    get:
      tags: [Catalog]
      summary: List supported assets
      responses:
        "200":
          description: Asset catalog
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    {
                      type: array,
                      items: { $ref: "#/components/schemas/Asset" },
                    }
  /api/v1/pairs:
    get:
      tags: [Catalog]
      summary: List supported pairs
      responses:
        "200":
          description: Pair catalog
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    {
                      type: array,
                      items: { $ref: "#/components/schemas/Pair" },
                    }
  /api/v1/historical/{pair}:
    get:
      tags: [Market]
      summary: Historical OHLCV data
      description: Returns canonical stored candles for the requested interval.
      parameters:
        - in: path
          name: pair
          required: true
          schema: { type: string }
        - in: query
          name: interval
          required: true
          schema: { type: string, enum: [1h, 1d] }
      responses:
        "200":
          description: Historical candle data
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    {
                      type: array,
                      items: { $ref: "#/components/schemas/Candle" },
                    }
  /api/v1/ticker/{pair}:
    get:
      tags: [Market]
      summary: Latest realtime ticker view
      description: Returns the freshest persisted ticker point for a pair. Price and last_update_unix prefer the newest realtime raw sample when it is newer than the current hourly candle; 1h, 24h, and 7d stats are derived from stored hourly candles.
      parameters:
        - in: path
          name: pair
          required: true
          schema: { type: string }
      responses:
        "200":
          description: Latest ticker
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Ticker" }
  /api/v1/tickers:
    get:
      tags: [Market]
      summary: Latest realtime ticker views
      description: Returns the freshest persisted ticker point for every pair. Price and last_update_unix prefer the newest realtime raw sample when it is newer than the current hourly candle; 1h, 24h, and 7d stats are derived from stored hourly candles.
      responses:
        "200":
          description: Latest tickers
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    {
                      type: array,
                      items: { $ref: "#/components/schemas/Ticker" },
                    }
  /api/v1/tickers/stream:
    get:
      tags: [Market]
      summary: Realtime ticker SSE stream
      description: Streams delta ticker updates only. Each SSE event contains the freshest persisted ticker rows for the pairs that changed since the previous event; clients should bootstrap with /api/v1/tickers and then merge incoming deltas. Use the quote parameter to receive only pairs denominated in a specific currency.
      parameters:
        - name: quote
          in: query
          required: false
          description: Filter tickers by quote currency (e.g. EUR, USD). When omitted all pairs are streamed.
          schema:
            type: string
            enum: [EUR, USD]
      responses:
        "200":
          description: Server-Sent Events stream
          content:
            text/event-stream:
              schema:
                type: object
                properties:
                  tickers:
                    type: array
                    items:
                      $ref: "#/components/schemas/Ticker"
  /api/v1/news:
    get:
      summary: List latest news
      responses:
        "200":
          description: List of news items
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    {
                      type: array,
                      items: { $ref: "#/components/schemas/NewsItem" },
                    }
  /api/v1/news/stream:
    get:
      summary: Realtime news SSE stream
      responses:
        "200":
          description: Server-Sent Events stream
  /api/v1/system/sync-status:
    get:
      tags: [System]
      summary: Backfill progress
      responses:
        "200":
          description: Sync status snapshot
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    {
                      type: array,
                      items: { $ref: "#/components/schemas/SyncStatus" },
                    }
  /api/v1/system/version:
    get:
      tags: [System]
      summary: Runtime version metadata
      responses:
        "200":
          description: Version snapshot
          content:
            application/json:
              schema:
                type: object
                properties:
                  api_version: { type: string }
  /api/v1/system/tasks:
    get:
      tags: [System]
      summary: Task queue snapshot
      responses:
        "200":
          description: Upcoming and recent tasks
          content:
            application/json:
              schema:
                type: object
                properties:
                  upcoming:
                    {
                      type: array,
                      items: { $ref: "#/components/schemas/Task" },
                    }
                  upcomingTotal: { type: integer }
                  recent:
                    {
                      type: array,
                      items: { $ref: "#/components/schemas/Task" },
                    }
                  recentTotal: { type: integer }
  /api/v1/system/tasks/stream:
    get:
      tags: [System]
      summary: Realtime task and sync-status SSE stream
      description: >
        Streams task queue snapshots enriched with per-pair sync status.
        Each SSE event contains upcoming/recent tasks plus the current
        backfill progress for every pair, allowing the Historical Coverage
        tab to update in real time as backfill tasks complete.
      responses:
        "200":
          description: Server-Sent Events stream
          content:
            text/event-stream:
              schema:
                type: object
                properties:
                  upcoming:
                    type: array
                    items: { $ref: "#/components/schemas/Task" }
                  upcomingTotal: { type: integer }
                  recent:
                    type: array
                    items: { $ref: "#/components/schemas/Task" }
                  recentTotal: { type: integer }
                  syncStatus:
                    type: array
                    items: { $ref: "#/components/schemas/SyncStatus" }
  /api/v1/system/warnings:
    get:
      summary: Active system warnings
      responses:
        "200":
          description: List of active warnings
          content:
            application/json:
              schema:
                type: array
                items: { $ref: "#/components/schemas/ActiveWarning" }
    post:
      summary: Dismiss a warning
      responses:
        "204":
          description: Warning dismissed
components:
  schemas:
    Asset:
      type: object
      properties:
        symbol: { type: string }
        name: { type: string }
        type: { type: string }
        circulating_supply: { type: number, format: double }
    Pair:
      type: object
      properties:
        base: { type: string }
        quote: { type: string }
        symbol: { type: string }
    Candle:
      type: object
      properties:
        pair: { type: string }
        interval: { type: string }
        timestamp: { type: integer, format: int64 }
        open: { type: number, format: double }
        high: { type: number, format: double }
        low: { type: number, format: double }
        close: { type: number, format: double }
        volume: { type: number, format: double }
        source: { type: string }
        finalized: { type: boolean }
    Ticker:
      type: object
      properties:
        pair: { type: string }
        price: { type: number, format: double }
        market_cap: { type: number, format: double }
        variation_24h: { type: number, format: double }
        volume_24h: { type: number, format: double, description: "Trailing 24h quote-currency turnover for this pair. Prefers provider-native 24h snapshots and otherwise estimates from stored hourly candles." }
        high_24h: { type: number, format: double }
        low_24h: { type: number, format: double }
        last_update_unix: { type: integer, format: int64 }
        source: { type: string }
    Task:
      type: object
      properties:
        id: { type: string }
        type: { type: string }
        pair: { type: string }
        interval: { type: string }
        window_start: { type: string, format: date-time }
        window_end: { type: string, format: date-time }
        status: { type: string }
        last_error: { type: string }
        completed_at: { type: string, format: date-time }
    SyncStatus:
      type: object
      properties:
        pair: { type: string }
        backfill_completed: { type: boolean }
        last_synced_unix: { type: integer, format: int64 }
        next_target_unix: { type: integer, format: int64 }
        hourly_backfill_completed: { type: boolean }
        daily_backfill_completed: { type: boolean }
        hourly_synced_unix: { type: integer, format: int64 }
        daily_synced_unix: { type: integer, format: int64 }
        next_hourly_target_unix: { type: integer, format: int64 }
        next_daily_target_unix: { type: integer, format: int64 }
    HealthStatus:
      type: object
      properties:
        status: { type: string }
        checks: { type: object, additionalProperties: { type: string } }
        timestamp: { type: integer, format: int64 }
    ActiveWarning:
      type: object
      properties:
        id: { type: string }
        level: { type: string }
        title: { type: string }
        detail: { type: string }
        fingerprint: { type: string }
        timestamp: { type: integer, format: int64 }
    NewsItem:
      type: object
      properties:
        id: { type: string }
        title: { type: string }
        link: { type: string }
        source: { type: string }
        published_at: { type: string, format: date-time }
`
}

func swaggerPageHTML() string {
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Exchangely API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    :root {
      --color-bg-base: #06090e;
      --color-bg-panel: rgba(14, 21, 30, 0.82);
      --color-text-primary: #f0f4f8;
      --color-text-secondary: #94a3b8;
      --color-text-accent: #00f0ff;
      --color-border: rgba(255, 255, 255, 0.08);
      --shadow-panel: 0 8px 32px 0 rgba(0, 0, 0, 0.37);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-width: 320px;
      min-height: 100vh;
      color: var(--color-text-primary);
      font-family: Inter, system-ui, sans-serif;
      background-color: var(--color-bg-base);
      background-image:
        radial-gradient(circle at top left, rgba(8, 51, 68, 0.8), transparent 40%),
        radial-gradient(circle at bottom right, rgba(15, 23, 42, 1), transparent 60%);
      background-attachment: fixed;
      -webkit-font-smoothing: antialiased;
    }
    a { color: inherit; text-decoration: none; }
    .shell {
      max-width: 1180px;
      margin: 0 auto;
      padding: 32px 24px 64px;
    }
    .hero,
    .panel {
      border: 1px solid var(--color-border);
      border-radius: 24px;
      background: var(--color-bg-panel);
      backdrop-filter: blur(16px);
      box-shadow: var(--shadow-panel);
    }
    .hero {
      display: flex;
      justify-content: space-between;
      align-items: flex-start;
      flex-wrap: wrap;
      gap: 24px;
      margin-bottom: 24px;
      padding: 32px;
    }
    .eyebrow {
      margin: 0 0 8px;
      color: var(--color-text-accent);
      font-size: 0.85rem;
      font-weight: 700;
      letter-spacing: 0.14em;
      text-transform: uppercase;
    }
    h1 {
      margin: 0;
      font-size: clamp(2rem, 4vw, 3rem);
      letter-spacing: -0.03em;
    }
    .hero p,
    .muted {
      color: var(--color-text-secondary);
    }
    .hero p {
      max-width: 60ch;
      margin: 12px 0 0;
      line-height: 1.6;
    }
    .cta-row {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
    }
    .pill {
      display: inline-flex;
      align-items: center;
      min-height: 42px;
      padding: 0 16px;
      border-radius: 999px;
      border: 1px solid var(--color-border);
      color: var(--color-text-secondary);
      transition: color 0.2s ease, border-color 0.2s ease, background 0.2s ease;
    }
    .pill.primary {
      color: #02171a;
      background: var(--color-text-accent);
      border-color: transparent;
      font-weight: 700;
    }
    .pill:hover {
      color: var(--color-text-primary);
      border-color: rgba(255, 255, 255, 0.18);
      background: rgba(255, 255, 255, 0.04);
    }
    .pill.primary:hover {
      color: #02171a;
      background: #6af7ff;
    }
    .panel {
      padding: 0;
      overflow: hidden;
    }
    #swagger-ui {
      padding: 24px;
      background: transparent;
    }
    .swagger-ui {
      color: var(--color-text-primary);
      font-family: Inter, system-ui, sans-serif;
    }
    .swagger-ui .topbar {
      display: none;
    }
    .swagger-ui .scheme-container {
      background: rgba(255, 255, 255, 0.03);
      box-shadow: none;
      border-bottom: 1px solid var(--color-border);
    }
    .swagger-ui .info .title,
    .swagger-ui .info hgroup.main a,
    .swagger-ui .opblock-tag,
    .swagger-ui .opblock .opblock-summary-path,
    .swagger-ui .opblock .opblock-summary-description,
    .swagger-ui .parameter__name,
    .swagger-ui .response-col_status,
    .swagger-ui .responses-inner h4,
    .swagger-ui .responses-inner h5,
    .swagger-ui section.models h4,
    .swagger-ui label,
    .swagger-ui .tab li,
    .swagger-ui .model-title,
    .swagger-ui .model,
    .swagger-ui .prop-name {
      color: var(--color-text-primary);
    }
    .swagger-ui,
    .swagger-ui .info p,
    .swagger-ui .info li,
    .swagger-ui .markdown p,
    .swagger-ui .markdown li,
    .swagger-ui .parameter__type,
    .swagger-ui .parameter__deprecated,
    .swagger-ui .response-col_description,
    .swagger-ui .model-toggle:after,
    .swagger-ui .opblock-description-wrapper p,
    .swagger-ui .opblock-external-docs-wrapper p,
    .swagger-ui .opblock-title_normal p {
      color: var(--color-text-secondary);
    }
    .swagger-ui .opblock,
    .swagger-ui .scheme-container,
    .swagger-ui section.models,
    .swagger-ui .model-box,
    .swagger-ui .responses-table tbody tr td,
    .swagger-ui table thead tr td,
    .swagger-ui table thead tr th {
      border-color: rgba(255, 255, 255, 0.08);
    }
    .swagger-ui .opblock {
      background: rgba(255, 255, 255, 0.02);
      border-radius: 18px;
    }
    .swagger-ui .opblock.is-open .opblock-summary {
      border-bottom-color: rgba(255, 255, 255, 0.08);
    }
    .swagger-ui input,
    .swagger-ui select,
    .swagger-ui textarea {
      color: var(--color-text-primary);
      background: rgba(6, 9, 14, 0.78);
      border: 1px solid rgba(255, 255, 255, 0.12);
    }
    .swagger-ui .btn.execute {
      background: #00b8c5;
      border-color: #00b8c5;
    }
    .swagger-ui .btn.authorize,
    .swagger-ui .btn.try-out__btn,
    .swagger-ui .btn.cancel {
      border-radius: 999px;
    }
    .swagger-ui .responses-wrapper,
    .swagger-ui .opblock-body pre,
    .swagger-ui .highlight-code {
      background: transparent;
    }
    @media (max-width: 768px) {
      .shell { padding: 16px 12px 32px; }
      .hero, .panel { padding: 22px; }
      #swagger-ui { padding: 16px; }
    }
  </style>
</head>
<body>
  <main class="shell">
    <section class="panel">
      <div id="swagger-ui"></div>
    </section>
  </main>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        url: "/swagger/openapi.yaml",
        dom_id: "#swagger-ui",
        deepLinking: true,
        displayRequestDuration: true,
        docExpansion: "list",
        persistAuthorization: true,
        tryItOutEnabled: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        layout: "BaseLayout"
      });
    };
  </script>
</body>
</html>`
}

func getIntParam(r *http.Request, name string, defaultVal int) int {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

func positiveInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func parseUnix(val string) time.Time {
	if val == "" {
		return time.Time{}
	}
	sec, _ := strconv.ParseInt(val, 10, 64)
	return time.Unix(sec, 0).UTC()
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Warn("response encode failed", "status", status, "error", err)
	}
}

func writeSSEData(w http.ResponseWriter, data []byte) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", string(data))
	return err
}

func writeError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "error", err)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "resource not found", http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func toAnySlice[T any](items []T) []any {
	result := make([]any, len(items))
	for i, item := range items {
		result[i] = item
	}
	return result
}

func withCORS(next http.Handler, allowedOrigins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := false
		for _, o := range allowedOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if r.Method == http.MethodOptions {
			if !allowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Debug("request",
			"method", r.Method,
			"url", r.URL.String(),
			"duration", time.Since(start),
		)
	})
}
