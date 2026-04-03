package router

import (
	"database/sql"
	"encoding/json"
	"errors"
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

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
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
