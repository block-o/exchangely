package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/block-o/exchangely/backend/internal/httpapi/middleware"
	portfolioSvc "github.com/block-o/exchangely/backend/internal/portfolio"
	"github.com/google/uuid"
)

// TransactionHandler exposes HTTP handler methods for transaction and P&L endpoints.
type TransactionHandler struct {
	txService *portfolioSvc.TransactionService
	pnlEngine *portfolioSvc.PnLEngine
}

// NewTransactionHandler creates a TransactionHandler.
func NewTransactionHandler(txService *portfolioSvc.TransactionService, pnlEngine *portfolioSvc.PnLEngine) *TransactionHandler {
	return &TransactionHandler{txService: txService, pnlEngine: pnlEngine}
}

// requireJWTUser extracts the authenticated user ID from JWT claims and rejects
// API token access. Returns the user ID and true if the request should proceed.
func (h *TransactionHandler) requireJWTUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	method, _ := middleware.AuthMethodFromContext(r.Context())
	if method == "api_token" {
		h.writeJSONError(w, http.StatusForbidden, "transaction endpoints require JWT session auth")
		return uuid.Nil, false
	}

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return uuid.Nil, false
	}

	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		h.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return uuid.Nil, false
	}

	return userID, true
}

// ListTransactions handles GET /api/v1/portfolio/transactions.
func (h *TransactionHandler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	opts := portfolio.ListOptions{
		Asset: r.URL.Query().Get("asset"),
		Type:  r.URL.Query().Get("type"),
	}

	if s := r.URL.Query().Get("start"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			h.writeJSONError(w, http.StatusBadRequest, "invalid start parameter, expected RFC3339 format")
			return
		}
		opts.StartDate = &t
	}

	if s := r.URL.Query().Get("end"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			h.writeJSONError(w, http.StatusBadRequest, "invalid end parameter, expected RFC3339 format")
			return
		}
		opts.EndDate = &t
	}

	opts.Page = getIntQueryParam(r, "page", 1)
	if opts.Page < 1 {
		opts.Page = 1
	}
	opts.PageSize = getIntQueryParam(r, "page_size", 50)
	if opts.PageSize < 1 || opts.PageSize > 200 {
		opts.PageSize = 50
	}

	txs, total, err := h.txService.ListTransactions(r.Context(), userID, opts)
	if err != nil {
		slog.Error("list transactions failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if txs == nil {
		txs = []portfolio.Transaction{}
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data":      txs,
		"total":     total,
		"page":      opts.Page,
		"page_size": opts.PageSize,
	})
}

// UpdateTransaction handles PUT /api/v1/portfolio/transactions/{id}.
func (h *TransactionHandler) UpdateTransaction(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	txID, err := h.extractPathID(r.URL.Path, "/api/v1/portfolio/transactions/")
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid transaction id")
		return
	}

	var req struct {
		ReferenceValue *float64 `json:"reference_value"`
		Notes          *string  `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ReferenceValue == nil && req.Notes == nil {
		h.writeJSONError(w, http.StatusBadRequest, "at least one of reference_value or notes is required")
		return
	}

	err = h.txService.UpdateTransaction(r.Context(), userID, txID, req.ReferenceValue, req.Notes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.writeJSONError(w, http.StatusNotFound, "transaction not found")
			return
		}
		slog.Error("update transaction failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// GetPnL handles GET /api/v1/portfolio/pnl.
func (h *TransactionHandler) GetPnL(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	quoteCurrency := r.URL.Query().Get("quote")
	if quoteCurrency == "" {
		quoteCurrency = "USD"
	}

	snapshot, err := h.pnlEngine.GetSnapshot(r.Context(), userID, quoteCurrency)
	if err != nil {
		slog.Error("get pnl failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if snapshot == nil {
		h.writeJSON(w, http.StatusOK, map[string]any{
			"total_realized":   0,
			"total_unrealized": 0,
			"total_pnl":        0,
			"has_approximate":  false,
			"excluded_count":   0,
			"assets":           []any{},
			"computed_at":      nil,
		})
		return
	}

	h.writeJSON(w, http.StatusOK, snapshot)
}

// --- Helpers ---

func (h *TransactionHandler) extractPathID(path, prefix string) (uuid.UUID, error) {
	idStr := strings.TrimPrefix(path, prefix)
	idStr = strings.Split(idStr, "/")[0]
	return uuid.Parse(idStr)
}

func (h *TransactionHandler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *TransactionHandler) writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func getIntQueryParam(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
