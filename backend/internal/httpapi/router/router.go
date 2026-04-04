package router

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/httpapi/dto"
	"github.com/block-o/exchangely/backend/internal/service"
)

type Services struct {
	Catalog *service.CatalogService
	Market  *service.MarketService
	System  *service.SystemService
}

type Options struct {
	AllowedOrigins []string
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

	// GET /api/v1/tickers — returns the latest price, 24h change, 24h high/low for all pairs.
	mux.HandleFunc("/api/v1/tickers", func(w http.ResponseWriter, r *http.Request) {
		items, err := svcs.Market.Tickers(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dto.ListResponse[any]{Data: toAnySlice(items)})
	})

	// GET /api/v1/tickers/stream — Server-Sent Events (SSE) endpoint.
	mux.HandleFunc("/api/v1/tickers/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		ctx := r.Context()
		updates := svcs.Market.Subscribe()
		defer svcs.Market.Unsubscribe(updates)

		// Initial push
		items, _ := svcs.Market.Tickers(ctx)
		data, _ := json.Marshal(map[string]any{"tickers": items})
		if err := writeSSEData(w, data); err != nil {
			slog.Warn("initial ticker stream write failed", "error", err)
			return
		}
		flusher.Flush()

		for {
			select {
			case <-ctx.Done():
				return
			case <-updates:
				items, _ := svcs.Market.Tickers(ctx)
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

	mux.HandleFunc("/swagger/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join("..", "docs", "openapi", "openapi.yaml")
		if _, err := os.Stat(path); err != nil {
			path = filepath.Join("docs", "openapi", "openapi.yaml")
		}
		http.ServeFile(w, r, path)
	})

	return withAccessLog(withCORS(mux, opts.AllowedOrigins))
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
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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
