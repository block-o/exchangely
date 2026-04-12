package router

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/block-o/exchangely/backend/internal/domain/ticker"
	"github.com/block-o/exchangely/backend/internal/httpapi/dto"
	"github.com/block-o/exchangely/backend/internal/httpapi/handlers"
	"github.com/block-o/exchangely/backend/internal/httpapi/middleware"
	"github.com/block-o/exchangely/backend/internal/portfolio"
	"github.com/block-o/exchangely/backend/internal/service"
)

type Services struct {
	Catalog          *service.CatalogService
	Market           *service.MarketService
	System           *service.SystemService
	News             *service.NewsService
	Auth             *auth.Service               // nil when auth is disabled
	APITokenService  *auth.APITokenService       // nil when API tokens are not configured
	APIRateLimiter   *auth.APIRateLimiter        // nil when rate limiting is not configured
	AdminUserService *auth.AdminUserService      // nil when admin user management is not configured
	PortfolioService *portfolio.PortfolioService // nil when portfolio is disabled
	ValuationEngine  *portfolio.ValuationEngine  // nil when portfolio is disabled
}

type Options struct {
	AllowedOrigins []string
	Env            string   // "development" or "production"
	AuthMode       string   // "local", "sso", "local,sso", or "" (disabled)
	TrustedProxies []string // CIDR ranges or IPs whose X-Forwarded-For / X-Real-IP headers are trusted
	APIBaseURL     string   // public API base URL for OpenAPI spec (e.g. http://localhost:8080/api/v1)
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

	// GET /api/v1/tickers — returns the freshest persisted ticker point plus 24h stats for all pairs,
	// enriched with 24h hourly sparkline candle data so the frontend can render trend charts
	// without issuing per-pair historical requests.
	mux.HandleFunc("/api/v1/tickers", func(w http.ResponseWriter, r *http.Request) {
		items, err := svcs.Market.TickersWithSparklines(r.Context())
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

	// GET /api/v1/config — public endpoint returning frontend-relevant configuration.
	// Always registered regardless of auth mode so the frontend can bootstrap.
	mux.HandleFunc("/api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		authMethods := map[string]bool{"google": false, "local": false}
		authEnabled := svcs.Auth != nil
		if authEnabled {
			m := svcs.Auth.AuthMethods()
			authMethods["google"] = m.Google
			authMethods["local"] = m.Local
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"auth_enabled": authEnabled,
			"auth_methods": authMethods,
			"version":      "v1.0.0",
		})
	})

	mux.HandleFunc("/api/v1/system/sync-status", func(w http.ResponseWriter, r *http.Request) {
		item, err := svcs.System.SyncSnapshot(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
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

		// Wire API token service for token management endpoints.
		if svcs.APITokenService != nil {
			authHandler = authHandler.WithAPITokenService(svcs.APITokenService)
		}

		hasLocal := strings.Contains(opts.AuthMode, "local")
		hasSSO := strings.Contains(opts.AuthMode, "sso")

		if hasSSO {
			mux.HandleFunc("/api/v1/auth/google/login", authHandler.GoogleLogin)
			mux.HandleFunc("/api/v1/auth/google/callback", authHandler.GoogleCallback)
		}
		if hasLocal {
			mux.HandleFunc("/api/v1/auth/local/login", authHandler.LocalLogin)
			mux.HandleFunc("/api/v1/auth/local/change-password", authHandler.ChangePassword)
		}
		mux.HandleFunc("/api/v1/auth/refresh", authHandler.Refresh)
		mux.HandleFunc("/api/v1/auth/logout", authHandler.Logout)
		mux.HandleFunc("/api/v1/auth/me", authHandler.Me)

		// API token management routes (require JWT session auth).
		if svcs.APITokenService != nil {
			mux.HandleFunc("/api/v1/auth/api-tokens", func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPost:
					authHandler.CreateAPIToken(w, r)
				case http.MethodGet:
					authHandler.ListAPITokens(w, r)
				default:
					w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPost}, ", "))
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				}
			})
			mux.HandleFunc("/api/v1/auth/api-tokens/", authHandler.RevokeAPIToken)
		}

		// Admin user management routes (admin-only via /api/v1/system/ prefix).
		if svcs.AdminUserService != nil {
			adminUserHandler := handlers.NewAdminUserHandler(svcs.AdminUserService)

			// GET /api/v1/system/users — list users with pagination and filters.
			mux.HandleFunc("/api/v1/system/users", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					adminUserHandler.List(w, r)
				} else {
					w.Header().Set("Allow", "GET")
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				}
			})

			// /api/v1/system/users/{id}/* — route to specific user operations.
			mux.HandleFunc("/api/v1/system/users/", func(w http.ResponseWriter, r *http.Request) {
				path := r.URL.Path

				// PATCH /api/v1/system/users/{id}/role
				if strings.HasSuffix(path, "/role") && r.Method == http.MethodPatch {
					adminUserHandler.UpdateRole(w, r)
					return
				}

				// PATCH /api/v1/system/users/{id}/status
				if strings.HasSuffix(path, "/status") && r.Method == http.MethodPatch {
					adminUserHandler.UpdateStatus(w, r)
					return
				}

				// POST /api/v1/system/users/{id}/force-password-reset
				if strings.HasSuffix(path, "/force-password-reset") && r.Method == http.MethodPost {
					adminUserHandler.ForcePasswordReset(w, r)
					return
				}

				// GET /api/v1/system/users/{id} — must come after specific routes.
				if r.Method == http.MethodGet {
					// Check if this is a simple user ID path (no additional segments).
					pathAfterUsers := strings.TrimPrefix(path, "/api/v1/system/users/")
					if !strings.Contains(pathAfterUsers, "/") {
						adminUserHandler.Get(w, r)
						return
					}
				}

				http.Error(w, "not found", http.StatusNotFound)
			})
		}
	}

	// Portfolio endpoints — only registered when portfolio service is available.
	if svcs.PortfolioService != nil && svcs.ValuationEngine != nil {
		ph := handlers.NewPortfolioHandler(svcs.PortfolioService, svcs.ValuationEngine)
		if svcs.Market != nil {
			ph = ph.WithMarketService(svcs.Market)
		}

		// Holdings CRUD.
		mux.HandleFunc("/api/v1/portfolio/holdings", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				ph.ListHoldings(w, r)
			case http.MethodPost:
				ph.CreateHolding(w, r)
			default:
				w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPost}, ", "))
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})
		mux.HandleFunc("/api/v1/portfolio/holdings/", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				ph.UpdateHolding(w, r)
			case http.MethodDelete:
				ph.DeleteHolding(w, r)
			default:
				w.Header().Set("Allow", strings.Join([]string{http.MethodPut, http.MethodDelete}, ", "))
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})

		// Valuation and history.
		mux.HandleFunc("/api/v1/portfolio/valuation", ph.GetValuation)
		mux.HandleFunc("/api/v1/portfolio/history", ph.GetHistory)

		// Sync all sources.
		mux.HandleFunc("/api/v1/portfolio/sync-all", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			ph.SyncAll(w, r)
		})

		// Exchange credentials.
		mux.HandleFunc("/api/v1/portfolio/credentials", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				ph.ListCredentials(w, r)
			case http.MethodPost:
				ph.CreateCredential(w, r)
			default:
				w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPost}, ", "))
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})
		mux.HandleFunc("/api/v1/portfolio/credentials/", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if strings.HasSuffix(path, "/sync") && r.Method == http.MethodPost {
				ph.SyncCredential(w, r)
				return
			}
			if r.Method == http.MethodDelete {
				ph.DeleteCredential(w, r)
				return
			}
			http.Error(w, "not found", http.StatusNotFound)
		})

		// Wallets.
		mux.HandleFunc("/api/v1/portfolio/wallets", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				ph.ListWallets(w, r)
			case http.MethodPost:
				ph.CreateWallet(w, r)
			default:
				w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPost}, ", "))
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})
		mux.HandleFunc("/api/v1/portfolio/wallets/", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if strings.HasSuffix(path, "/sync") && r.Method == http.MethodPost {
				ph.SyncWallet(w, r)
				return
			}
			if r.Method == http.MethodDelete {
				ph.DeleteWallet(w, r)
				return
			}
			http.Error(w, "not found", http.StatusNotFound)
		})

		// Ledger.
		mux.HandleFunc("/api/v1/portfolio/ledger/connect", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			ph.ConnectLedger(w, r)
		})
		mux.HandleFunc("/api/v1/portfolio/ledger/sync", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			ph.SyncLedger(w, r)
		})
		mux.HandleFunc("/api/v1/portfolio/ledger", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				w.Header().Set("Allow", http.MethodDelete)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			ph.DisconnectLedger(w, r)
		})

		// Portfolio SSE stream.
		mux.HandleFunc("/api/v1/portfolio/stream", ph.StreamPortfolio)
	}

	mux.HandleFunc("/swagger/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write([]byte(defaultOpenAPIYAML(opts.APIBaseURL)))
	})

	// Wrap with middleware chain: RealIP → APITokenMW → AuthMW → RateLimitMW → mux.
	// When services are nil, each middleware passes requests through unchanged.
	apiTokenMW := middleware.NewAPITokenMiddleware(svcs.APITokenService)
	authMW := middleware.NewAuthMiddleware(svcs.Auth)
	rateLimitMW := middleware.NewRateLimitMiddleware(svcs.APIRateLimiter)
	realIPMW := middleware.NewRealIPMiddleware(opts.TrustedProxies)
	return withAccessLog(withCORS(realIPMW.Wrap(apiTokenMW.Wrap(authMW.Wrap(rateLimitMW.Wrap(mux)))), opts.AllowedOrigins))
}

func serveSwaggerPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerPageHTML()))
}

func defaultOpenAPIYAML(apiBaseURL string) string {
	serverURL := "http://localhost:8080"
	if apiBaseURL != "" {
		// Strip /api/v1 suffix to get the server root.
		serverURL = strings.TrimSuffix(apiBaseURL, "/api/v1")
		serverURL = strings.TrimSuffix(serverURL, "/")
	}
	return `openapi: 3.0.3
info:
  title: Exchangely API
  version: 0.1.0
  description: REST API for Exchangely market data and sync state. Github (https://github.com/block-o/exchangely)
servers:
  - url: ` + serverURL + `
security:
  - BearerAuth: []
paths:
  /api/v1/health:
    get:
      tags: [System]
      summary: Health status
      security: []
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
      security: []
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
      security: []
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
      security: []
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
      security: []
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
      summary: Latest realtime ticker views with sparkline data
      security: []
      description: >
        Returns the freshest persisted ticker point for every pair enriched with
        the last 24 hourly candle points (sparkline field) so the frontend can
        render trend charts without issuing per-pair historical requests.
        Price and last_update_unix prefer the newest realtime raw sample when it
        is newer than the current hourly candle; 1h, 24h, and 7d stats are
        derived from stored hourly candles. The sparkline array is cached
        server-side with a 60-second TTL.
      responses:
        "200":
          description: Latest tickers with embedded sparkline candles
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
      security: []
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
  /api/v1/portfolio/holdings:
    get:
      tags: [Portfolio]
      summary: List user's holdings
      description: Returns all holdings for the authenticated user. Requires a valid JWT session.
      responses:
        "200":
          description: User holdings
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: array
                    items: { $ref: "#/components/schemas/Holding" }
    post:
      tags: [Portfolio]
      summary: Create manual holding
      description: Creates a new manual holding for the authenticated user. Requires a valid JWT session.
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                asset_symbol: { type: string }
                quantity: { type: number, format: double }
                avg_buy_price: { type: number, format: double }
                quote_currency: { type: string }
                notes: { type: string }
      responses:
        "201":
          description: Created holding
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Holding" }
  /api/v1/portfolio/holdings/{id}:
    put:
      tags: [Portfolio]
      summary: Update holding
      description: Updates an existing holding owned by the authenticated user. Requires a valid JWT session.
      parameters:
        - { in: path, name: id, required: true, schema: { type: string, format: uuid } }
      responses:
        "200":
          description: Updated holding
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Holding" }
    delete:
      tags: [Portfolio]
      summary: Delete holding
      description: Deletes a holding owned by the authenticated user. Requires a valid JWT session.
      parameters:
        - { in: path, name: id, required: true, schema: { type: string, format: uuid } }
      responses:
        "204":
          description: Holding deleted
  /api/v1/portfolio/valuation:
    get:
      tags: [Portfolio]
      summary: Current portfolio valuation
      description: Returns a computed valuation snapshot with per-asset breakdown. Requires a valid JWT session.
      parameters:
        - { in: query, name: quote, schema: { type: string, default: USD } }
      responses:
        "200":
          description: Portfolio valuation snapshot
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Valuation" }
  /api/v1/portfolio/history:
    get:
      tags: [Portfolio]
      summary: Historical portfolio value
      description: Returns historical portfolio value data points for charting. Requires a valid JWT session.
      parameters:
        - { in: query, name: range, schema: { type: string, enum: [1d, 7d, 30d, 1y], default: 7d } }
        - { in: query, name: quote, schema: { type: string, default: USD } }
      responses:
        "200":
          description: Historical value data points
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: array
                    items: { $ref: "#/components/schemas/HistoricalPoint" }
  /api/v1/portfolio/stream:
    get:
      tags: [Portfolio]
      summary: SSE live portfolio updates
      description: Streams live portfolio valuation updates via Server-Sent Events. Requires a valid JWT session.
      responses:
        "200":
          description: Server-Sent Events stream
          content:
            text/event-stream:
              schema: { $ref: "#/components/schemas/Valuation" }
  /api/v1/portfolio/credentials:
    post:
      tags: [Portfolio]
      summary: Store exchange credential
      description: Stores an exchange API credential for automatic balance syncing. Secrets are encrypted at rest. Requires a valid JWT session.
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                exchange: { type: string, enum: [binance, kraken, coinbase] }
                api_key: { type: string }
                api_secret: { type: string }
      responses:
        "201":
          description: Credential stored (metadata only)
          content:
            application/json:
              schema: { $ref: "#/components/schemas/ExchangeCredentialMeta" }
    get:
      tags: [Portfolio]
      summary: List exchange credentials (metadata only)
      description: Returns metadata for all exchange credentials owned by the authenticated user. Secrets are never exposed. Requires a valid JWT session.
      responses:
        "200":
          description: Credential metadata list
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: array
                    items: { $ref: "#/components/schemas/ExchangeCredentialMeta" }
  /api/v1/portfolio/credentials/{id}:
    delete:
      tags: [Portfolio]
      summary: Delete exchange credential
      description: Deletes an exchange credential and its associated synced holdings. Requires a valid JWT session.
      parameters:
        - { in: path, name: id, required: true, schema: { type: string, format: uuid } }
      responses:
        "204":
          description: Credential deleted
  /api/v1/portfolio/credentials/{id}/sync:
    post:
      tags: [Portfolio]
      summary: Trigger exchange sync
      description: Triggers an immediate balance sync for the specified exchange credential. Requires a valid JWT session.
      parameters:
        - { in: path, name: id, required: true, schema: { type: string, format: uuid } }
      responses:
        "200":
          description: Sync completed
  /api/v1/portfolio/wallets:
    post:
      tags: [Portfolio]
      summary: Link wallet address
      description: Links an on-chain wallet address for automatic balance tracking. Requires a valid JWT session.
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                chain: { type: string, enum: [ethereum, solana, bitcoin] }
                address: { type: string }
                label: { type: string }
      responses:
        "201":
          description: Wallet linked (metadata only)
          content:
            application/json:
              schema: { $ref: "#/components/schemas/WalletMeta" }
    get:
      tags: [Portfolio]
      summary: List linked wallets (metadata only)
      description: Returns metadata for all linked wallets owned by the authenticated user. Full addresses are never exposed. Requires a valid JWT session.
      responses:
        "200":
          description: Wallet metadata list
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: array
                    items: { $ref: "#/components/schemas/WalletMeta" }
  /api/v1/portfolio/wallets/{id}:
    delete:
      tags: [Portfolio]
      summary: Delete linked wallet
      description: Deletes a linked wallet and its associated synced holdings. Requires a valid JWT session.
      parameters:
        - { in: path, name: id, required: true, schema: { type: string, format: uuid } }
      responses:
        "204":
          description: Wallet deleted
  /api/v1/portfolio/wallets/{id}/sync:
    post:
      tags: [Portfolio]
      summary: Trigger wallet sync
      description: Triggers an immediate balance sync for the specified wallet. Requires a valid JWT session.
      parameters:
        - { in: path, name: id, required: true, schema: { type: string, format: uuid } }
      responses:
        "200":
          description: Sync completed
  /api/v1/portfolio/ledger/connect:
    post:
      tags: [Portfolio]
      summary: Connect Ledger Live
      description: Connects a Ledger Live account using an API token. One Ledger connection per user. Requires a valid JWT session.
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                token: { type: string }
      responses:
        "201":
          description: Ledger connected
          content:
            application/json:
              schema: { $ref: "#/components/schemas/LedgerCredentialMeta" }
  /api/v1/portfolio/ledger/sync:
    post:
      tags: [Portfolio]
      summary: Re-sync Ledger balances
      description: Triggers an immediate balance sync from Ledger Live. Requires a valid JWT session and an existing Ledger connection.
      responses:
        "200":
          description: Sync completed
  /api/v1/portfolio/ledger:
    delete:
      tags: [Portfolio]
      summary: Disconnect Ledger Live
      description: Disconnects the Ledger Live connection and removes associated synced holdings. Requires a valid JWT session.
      responses:
        "204":
          description: Ledger disconnected
  /api/v1/news:
    get:
      tags: [News]
      summary: List latest news
      security: []
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
      tags: [News]
      summary: Realtime news SSE stream
      security: []
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
      tags: [System]
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
      tags: [System]
      summary: Dismiss a warning
      responses:
        "204":
          description: Warning dismissed
  /api/v1/system/users:
    get:
      tags: [Admin]
      summary: List users
      description: Paginated user list with search, role, and status filters. Admin only.
      parameters:
        - { in: query, name: search, schema: { type: string }, description: "Filter by email or name substring" }
        - { in: query, name: role, schema: { type: string, enum: [user, premium, admin] }, description: "Filter by role" }
        - { in: query, name: status, schema: { type: string, enum: [active, disabled] }, description: "Filter by account status" }
        - { in: query, name: page, schema: { type: integer, default: 1 }, description: "Page number" }
        - { in: query, name: limit, schema: { type: integer, default: 50 }, description: "Items per page" }
      responses:
        "200":
          description: Paginated user list
        "403":
          description: Forbidden (non-admin)
  /api/v1/system/users/{id}:
    get:
      tags: [Admin]
      summary: Get user by ID
      parameters:
        - { in: path, name: id, required: true, schema: { type: string, format: uuid } }
      responses:
        "200":
          description: User details
        "404":
          description: User not found
  /api/v1/system/users/{id}/role:
    patch:
      tags: [Admin]
      summary: Update user role
      parameters:
        - { in: path, name: id, required: true, schema: { type: string, format: uuid } }
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                role: { type: string, enum: [user, premium, admin] }
      responses:
        "200":
          description: Updated user
        "400":
          description: Invalid role or self-change attempt
  /api/v1/system/users/{id}/status:
    patch:
      tags: [Admin]
      summary: Enable or disable user
      parameters:
        - { in: path, name: id, required: true, schema: { type: string, format: uuid } }
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                disabled: { type: boolean }
      responses:
        "200":
          description: Updated user
        "400":
          description: Self-disable attempt
  /api/v1/system/users/{id}/force-password-reset:
    post:
      tags: [Admin]
      summary: Force password reset on next login
      parameters:
        - { in: path, name: id, required: true, schema: { type: string, format: uuid } }
      responses:
        "200":
          description: Updated user with must_change_password set
        "400":
          description: User has no password authentication

components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
      description: JWT access token obtained via /api/v1/auth/local/login or /api/v1/auth/google/callback
  schemas:
    Asset:
      type: object
      description: "A supported asset from the catalog."
      properties:
        symbol: { type: string, description: "Ticker symbol (e.g. BTC, EUR)." }
        name: { type: string, description: "Human-readable name (e.g. Bitcoin, Euro)." }
        type: { type: string, description: "Asset class — crypto or fiat." }
        circulating_supply: { type: number, format: double, description: "Current circulating supply used for market cap calculation." }
    Pair:
      type: object
      description: "A supported trading pair from the catalog."
      properties:
        base: { type: string, description: "Base asset symbol (e.g. BTC)." }
        quote: { type: string, description: "Quote asset symbol (e.g. EUR)." }
        symbol: { type: string, description: "Concatenated pair symbol (e.g. BTCEUR)." }
    Candle:
      type: object
      description: "A single OHLCV candle for a trading pair at a given resolution."
      properties:
        pair: { type: string, description: "Trading pair symbol." }
        interval: { type: string, description: "Candle resolution — 1h or 1d." }
        timestamp: { type: integer, format: int64, description: "Bucket start as unix epoch seconds." }
        open: { type: number, format: double, description: "Opening price." }
        high: { type: number, format: double, description: "Highest price." }
        low: { type: number, format: double, description: "Lowest price." }
        close: { type: number, format: double, description: "Closing price." }
        volume: { type: number, format: double, description: "Traded volume." }
        source: { type: string, description: "Data provider that produced this candle." }
        finalized: { type: boolean, description: "Whether the candle covers a completed time window." }
    Ticker:
      type: object
      description: >
        Point-in-time market snapshot for a trading pair. Returned by /tickers
        (with sparkline) and /tickers/stream (without sparkline). Price and
        last_update_unix prefer the newest realtime raw sample over the current
        hourly candle; variation and volume stats are derived from stored hourly
        candles.
      properties:
        pair: { type: string, description: "Trading pair symbol (e.g. BTCEUR)." }
        price: { type: number, format: double, description: "Latest market price from the freshest source." }
        market_cap: { type: number, format: double, description: "Estimated market cap (price × circulating supply)." }
        variation_1h: { type: number, format: double, description: "Percentage price change over the last hour." }
        variation_24h: { type: number, format: double, description: "Percentage price change over the last 24 hours." }
        variation_7d: { type: number, format: double, description: "Percentage price change over the last 7 days." }
        volume_24h: { type: number, format: double, description: "Trailing 24h quote-currency turnover. Prefers provider-native 24h snapshots; falls back to hourly candle aggregation." }
        high_24h: { type: number, format: double, description: "Highest price in the last 24 hours." }
        low_24h: { type: number, format: double, description: "Lowest price in the last 24 hours." }
        last_update_unix: { type: integer, format: int64, description: "Unix epoch of the latest data sample." }
        source: { type: string, description: "Provider that produced the latest sample (e.g. binance, kraken)." }
        sparkline:
          type: array
          description: "Last 24 hourly OHLCV points for sparkline rendering. Present in /tickers responses; absent in /tickers/stream deltas."
          items: { $ref: "#/components/schemas/SparklinePoint" }
    SparklinePoint:
      type: object
      description: "Lightweight hourly OHLCV point used for sparkline charts."
      properties:
        timestamp: { type: integer, format: int64, description: "Bucket start as unix epoch seconds." }
        open: { type: number, format: double, description: "Opening price." }
        high: { type: number, format: double, description: "Highest price." }
        low: { type: number, format: double, description: "Lowest price." }
        close: { type: number, format: double, description: "Closing price." }
        volume: { type: number, format: double, description: "Traded volume." }
    Task:
      type: object
      description: "A scheduled or completed task in the planner/worker pipeline."
      properties:
        id: { type: string, description: "Unique task identifier." }
        type: { type: string, description: "Task type (e.g. historical_backfill, live_ticker, consolidation)." }
        pair: { type: string, description: "Target trading pair symbol, or * for system-wide tasks." }
        interval: { type: string, description: "Data resolution (e.g. 1h, 1d, realtime)." }
        window_start: { type: string, format: date-time, description: "Start of the time window this task covers." }
        window_end: { type: string, format: date-time, description: "End of the time window this task covers." }
        status: { type: string, description: "Current lifecycle status (pending, running, completed, failed)." }
        last_error: { type: string, description: "Error message from the most recent failed attempt, if any." }
        completed_at: { type: string, format: date-time, description: "Timestamp when the task completed or failed." }
    SyncStatus:
      type: object
      description: "Per-pair backfill and consolidation progress."
      properties:
        pair: { type: string, description: "Trading pair symbol." }
        backfill_completed: { type: boolean, description: "Whether hourly backfill has reached the earliest available data." }
        last_synced_unix: { type: integer, format: int64, description: "Unix epoch of the most recently synced candle." }
        next_target_unix: { type: integer, format: int64, description: "Unix epoch of the next backfill target." }
        hourly_backfill_completed: { type: boolean, description: "Whether hourly-resolution backfill is fully caught up." }
        daily_backfill_completed: { type: boolean, description: "Whether daily-resolution consolidation is fully caught up." }
        hourly_synced_unix: { type: integer, format: int64, description: "Unix epoch of the most recently synced hourly candle." }
        daily_synced_unix: { type: integer, format: int64, description: "Unix epoch of the most recently synced daily candle." }
        next_hourly_target_unix: { type: integer, format: int64, description: "Unix epoch of the next hourly backfill target." }
        next_daily_target_unix: { type: integer, format: int64, description: "Unix epoch of the next daily consolidation target." }
    HealthStatus:
      type: object
      description: "Service health check result."
      properties:
        status: { type: string, description: "Overall status (e.g. ok, degraded)." }
        checks: { type: object, additionalProperties: { type: string }, description: "Per-dependency health results." }
        timestamp: { type: integer, format: int64, description: "Unix epoch when the check was performed." }
    ActiveWarning:
      type: object
      description: "An active system warning surfaced in the Operations panel."
      properties:
        id: { type: string, description: "Unique warning identifier." }
        level: { type: string, description: "Severity — warning or error." }
        title: { type: string, description: "Short human-readable title." }
        detail: { type: string, description: "Extended description with context." }
        fingerprint: { type: string, description: "Stable fingerprint for dismissal deduplication." }
        timestamp: { type: integer, format: int64, description: "Unix epoch when the warning was raised." }
    NewsItem:
      type: object
      description: "A news article from an RSS feed."
      properties:
        id: { type: string, description: "Unique article identifier." }
        title: { type: string, description: "Article headline." }
        link: { type: string, description: "URL to the full article." }
        source: { type: string, description: "RSS feed source (e.g. CoinDesk, Cointelegraph)." }
        published_at: { type: string, format: date-time, description: "Publication timestamp." }
    Holding:
      type: object
      description: "A single position in a user's portfolio."
      properties:
        id: { type: string, format: uuid }
        user_id: { type: string, format: uuid }
        asset_symbol: { type: string }
        quantity: { type: number, format: double }
        avg_buy_price: { type: number, format: double, nullable: true }
        quote_currency: { type: string }
        source: { type: string, enum: [manual, binance, kraken, coinbase, ethereum, solana, bitcoin, ledger] }
        source_ref: { type: string, format: uuid, nullable: true }
        notes: { type: string }
        created_at: { type: string, format: date-time }
        updated_at: { type: string, format: date-time }
    Valuation:
      type: object
      description: "Computed portfolio valuation snapshot."
      properties:
        total_value: { type: number, format: double }
        quote_currency: { type: string }
        assets:
          type: array
          items: { $ref: "#/components/schemas/AssetValuation" }
        updated_at: { type: string, format: date-time }
    AssetValuation:
      type: object
      description: "Per-asset breakdown within a portfolio valuation."
      properties:
        asset_symbol: { type: string }
        quantity: { type: number, format: double }
        current_price: { type: number, format: double }
        current_value: { type: number, format: double }
        allocation_pct: { type: number, format: double }
        avg_buy_price: { type: number, format: double, nullable: true }
        unrealized_pnl: { type: number, format: double, nullable: true }
        unrealized_pnl_pct: { type: number, format: double, nullable: true }
        priced: { type: boolean }
        source: { type: string }
    HistoricalPoint:
      type: object
      description: "A single data point in the historical portfolio value series."
      properties:
        timestamp: { type: integer, format: int64 }
        value: { type: number, format: double }
    ExchangeCredentialMeta:
      type: object
      description: "Exchange credential metadata (secrets are never exposed)."
      properties:
        id: { type: string, format: uuid }
        user_id: { type: string, format: uuid }
        exchange: { type: string, enum: [binance, kraken, coinbase] }
        api_key_prefix: { type: string }
        status: { type: string, enum: [active, failed] }
        error_reason: { type: string, nullable: true }
        last_sync_at: { type: string, format: date-time, nullable: true }
        created_at: { type: string, format: date-time }
        updated_at: { type: string, format: date-time }
    WalletMeta:
      type: object
      description: "Wallet address metadata (full address is never exposed)."
      properties:
        id: { type: string, format: uuid }
        user_id: { type: string, format: uuid }
        chain: { type: string, enum: [ethereum, solana, bitcoin] }
        address_prefix: { type: string }
        label: { type: string }
        last_sync_at: { type: string, format: date-time, nullable: true }
        created_at: { type: string, format: date-time }
        updated_at: { type: string, format: date-time }
    LedgerCredentialMeta:
      type: object
      description: "Ledger Live credential metadata."
      properties:
        id: { type: string, format: uuid }
        user_id: { type: string, format: uuid }
        last_sync_at: { type: string, format: date-time, nullable: true }
        created_at: { type: string, format: date-time }
        updated_at: { type: string, format: date-time }
`
}

func swaggerPageHTML() string {
	return `<!doctype html>
<html lang="en" data-theme="dark">
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
      --color-interactive-bg: rgba(255, 255, 255, 0.04);
      --color-interactive-border: rgba(255, 255, 255, 0.18);
      --color-input-bg: rgba(6, 9, 14, 0.78);
      --color-input-border: rgba(255, 255, 255, 0.12);
      --color-opblock-bg: rgba(255, 255, 255, 0.02);
      --color-opblock-border: rgba(255, 255, 255, 0.08);
      --color-scheme-bg: rgba(255, 255, 255, 0.03);
      --shadow-panel: 0 8px 32px 0 rgba(0, 0, 0, 0.37);
      --bg-gradient-1: rgba(8, 51, 68, 0.8);
      --bg-gradient-2: rgba(15, 23, 42, 1);
      --pill-primary-text: #02171a;
      --pill-primary-hover: #6af7ff;
    }
    [data-theme="light"] {
      --color-bg-base: #f8fafc;
      --color-bg-panel: rgba(255, 255, 255, 0.82);
      --color-text-primary: #0f172a;
      --color-text-secondary: #475569;
      --color-text-accent: #0284c7;
      --color-border: rgba(15, 23, 42, 0.1);
      --color-interactive-bg: rgba(15, 23, 42, 0.04);
      --color-interactive-border: rgba(15, 23, 42, 0.15);
      --color-input-bg: rgba(241, 245, 249, 0.8);
      --color-input-border: rgba(15, 23, 42, 0.12);
      --color-opblock-bg: rgba(15, 23, 42, 0.02);
      --color-opblock-border: rgba(15, 23, 42, 0.08);
      --color-scheme-bg: rgba(15, 23, 42, 0.03);
      --shadow-panel: 0 8px 32px 0 rgba(15, 23, 42, 0.08);
      --bg-gradient-1: rgba(186, 230, 253, 0.5);
      --bg-gradient-2: rgba(241, 245, 249, 1);
      --pill-primary-text: #fff;
      --pill-primary-hover: #0369a1;
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
        radial-gradient(circle at top left, var(--bg-gradient-1), transparent 40%),
        radial-gradient(circle at bottom right, var(--bg-gradient-2), transparent 60%);
      background-attachment: fixed;
      -webkit-font-smoothing: antialiased;
      transition: background-color 0.3s ease, color 0.3s ease;
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
      transition: background 0.3s ease, border-color 0.3s ease;
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
      color: var(--pill-primary-text);
      background: var(--color-text-accent);
      border-color: transparent;
      font-weight: 700;
    }
    .pill:hover {
      color: var(--color-text-primary);
      border-color: var(--color-interactive-border);
      background: var(--color-interactive-bg);
    }
    .pill.primary:hover {
      color: var(--pill-primary-text);
      background: var(--pill-primary-hover);
    }
    .theme-toggle {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 42px;
      height: 42px;
      border-radius: 999px;
      border: 1px solid var(--color-border);
      background: none;
      color: var(--color-text-secondary);
      cursor: pointer;
      font-size: 1.1rem;
      transition: color 0.2s ease, border-color 0.2s ease, background 0.2s ease;
    }
    .theme-toggle:hover {
      color: var(--color-text-accent);
      border-color: var(--color-interactive-border);
      background: var(--color-interactive-bg);
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
      background: var(--color-scheme-bg);
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
      border-color: var(--color-opblock-border);
    }
    .swagger-ui .opblock {
      background: var(--color-opblock-bg);
      border-radius: 18px;
    }
    .swagger-ui .opblock.is-open .opblock-summary {
      border-bottom-color: var(--color-opblock-border);
    }
    .swagger-ui input,
    .swagger-ui select,
    .swagger-ui textarea {
      color: var(--color-text-primary);
      background: var(--color-input-bg);
      border: 1px solid var(--color-input-border);
    }
    .swagger-ui .btn.execute {
      background: #00b8c5;
      border-color: #00b8c5;
    }
    [data-theme="light"] .swagger-ui .btn.execute {
      background: #0284c7;
      border-color: #0284c7;
      color: #fff;
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
    <section class="panel" style="position:relative">
      <button id="theme-toggle" class="theme-toggle" onclick="toggleTheme()" title="Toggle theme" aria-label="Toggle dark/light theme" style="position:absolute;top:16px;right:16px;z-index:10">🌙</button>
      <div id="swagger-ui"></div>
    </section>
  </main>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    // Theme resolution: ?theme= query param > cookie > default (dark)
    (function() {
      function getCookie(name) {
        var match = document.cookie.match(new RegExp("(?:^|; )" + name + "=([^;]*)"));
        return match ? decodeURIComponent(match[1]) : null;
      }
      function setCookie(name, value) {
        document.cookie = name + "=" + encodeURIComponent(value) + "; path=/; max-age=31536000; SameSite=Lax";
      }

      var params = new URLSearchParams(window.location.search);
      var qp = params.get("theme");
      var cookie = getCookie("exchangely_theme");
      var theme = "dark";
      if (qp === "light" || qp === "dark") {
        theme = qp;
      } else if (cookie === "light" || cookie === "dark") {
        theme = cookie;
      }

      document.documentElement.setAttribute("data-theme", theme);
      setCookie("exchangely_theme", theme);

      function updateToggleIcon(t) {
        var btn = document.getElementById("theme-toggle");
        if (btn) btn.textContent = t === "dark" ? "\u2600\uFE0F" : "\uD83C\uDF19";
      }

      window.toggleTheme = function() {
        var current = document.documentElement.getAttribute("data-theme") || "dark";
        var next = current === "dark" ? "light" : "dark";
        document.documentElement.setAttribute("data-theme", next);
        setCookie("exchangely_theme", next);
        // Update URL without reload so bookmarks/shares keep the theme
        var url = new URL(window.location);
        url.searchParams.set("theme", next);
        window.history.replaceState({}, "", url);
        updateToggleIcon(next);
      };

      window.addEventListener("DOMContentLoaded", function() {
        updateToggleIcon(theme);
      });
    })();

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
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
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
