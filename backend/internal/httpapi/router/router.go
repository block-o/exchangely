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
	// The connection stays open and pushes a full ticker snapshot each time the
	// backend's EventBus fires (triggered by BackfillExecutor or RealtimeIngestService
	// after a successful Postgres write). This eliminates frontend polling entirely.
	//
	// Lifecycle:
	//   1. Subscribe to MarketService's EventBus channel.
	//   2. Immediately send the current ticker state as the first SSE event.
	//   3. Block on the channel; on each signal, re-query and push fresh state.
	//   4. On client disconnect (ctx.Done), unsubscribe and close.
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

		// Send initial snapshot so the client has data immediately on connect.
		items, err := svcs.Market.Tickers(ctx)
		if err == nil {
			data, _ := json.Marshal(items)
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
		}

		// Block until either the client disconnects or new data arrives.
		for {
			select {
			case <-ctx.Done():
				return
			case <-updates:
				items, err := svcs.Market.Tickers(ctx)
				if err == nil {
					data, _ := json.Marshal(items)
					fmt.Fprintf(w, "data: %s\n\n", string(data))
					flusher.Flush()
				}
			}
		}
	})

	mux.HandleFunc("/api/v1/system/sync-status", func(w http.ResponseWriter, r *http.Request) {
		item, err := svcs.System.SyncStatus(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
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

func parseUnix(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	unix, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(unix, 0).UTC()
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, os.ErrNotExist) {
		status = http.StatusNotFound
	}
	if errors.Is(err, sql.ErrNoRows) {
		status = http.StatusNotFound
	}
	http.Error(w, err.Error(), status)
}

func toAnySlice[T any](items []T) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func withCORS(next http.Handler, allowedOrigins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		allowedOrigin, ok := matchAllowedOrigin(origin, allowedOrigins)
		if ok {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if r.Method == http.MethodOptions {
			if ok {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		slog.Info("http request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", recorder.status,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// statusRecorder wraps ResponseWriter to capture the HTTP status code for access logging.
// It also implements http.Flusher so that SSE streaming works through the logging middleware.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Flush delegates to the underlying ResponseWriter's Flush if supported.
// This is required for Server-Sent Events — without it, the SSE handler's
// w.(http.Flusher) type assertion fails and returns 500.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func matchAllowedOrigin(origin string, allowedOrigins []string) (string, bool) {
	if origin == "" {
		return "", false
	}
	for _, candidate := range allowedOrigins {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if candidate == "*" || candidate == origin {
			return origin, true
		}
	}
	return "", false
}
