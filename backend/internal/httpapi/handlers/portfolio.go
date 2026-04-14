package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/httpapi/middleware"
	"github.com/block-o/exchangely/backend/internal/portfolio"
	"github.com/block-o/exchangely/backend/internal/service"
	"github.com/google/uuid"

	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
)

// PortfolioHandler exposes HTTP handler methods for portfolio endpoints.
type PortfolioHandler struct {
	svc       *portfolio.PortfolioService
	ve        *portfolio.ValuationEngine
	marketSvc *service.MarketService // nil when SSE is not available
}

// NewPortfolioHandler creates a PortfolioHandler.
func NewPortfolioHandler(svc *portfolio.PortfolioService, ve *portfolio.ValuationEngine) *PortfolioHandler {
	return &PortfolioHandler{svc: svc, ve: ve}
}

// WithMarketService attaches a MarketService for SSE streaming support.
func (h *PortfolioHandler) WithMarketService(ms *service.MarketService) *PortfolioHandler {
	h.marketSvc = ms
	return h
}

// requireJWTUser extracts the authenticated user ID from JWT claims and rejects
// API token access. Returns the user ID and true if the request should proceed.
func (h *PortfolioHandler) requireJWTUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	method, _ := middleware.AuthMethodFromContext(r.Context())
	if method == "api_token" {
		h.writeJSONError(w, http.StatusForbidden, "portfolio endpoints require JWT session auth")
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

// --- Holdings ---

// ListHoldings handles GET /api/v1/portfolio/holdings.
func (h *PortfolioHandler) ListHoldings(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	holdings, err := h.svc.ListHoldings(r.Context(), userID)
	if err != nil {
		slog.Error("list holdings failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if holdings == nil {
		holdings = []domain.Holding{}
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"data": holdings})
}

// CreateHolding handles POST /api/v1/portfolio/holdings.
func (h *PortfolioHandler) CreateHolding(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	var req struct {
		AssetSymbol   string   `json:"asset_symbol"`
		Quantity      float64  `json:"quantity"`
		AvgBuyPrice   *float64 `json:"avg_buy_price,omitempty"`
		QuoteCurrency string   `json:"quote_currency"`
		Notes         string   `json:"notes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	quoteCurrency := req.QuoteCurrency
	if quoteCurrency == "" {
		quoteCurrency = "USD"
	}

	holding := &domain.Holding{
		AssetSymbol:   req.AssetSymbol,
		Quantity:      req.Quantity,
		AvgBuyPrice:   req.AvgBuyPrice,
		QuoteCurrency: quoteCurrency,
		Source:        "manual",
		Notes:         req.Notes,
	}

	if err := h.svc.CreateHolding(r.Context(), userID, holding); err != nil {
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, holding)
}

// UpdateHolding handles PUT /api/v1/portfolio/holdings/{id}.
func (h *PortfolioHandler) UpdateHolding(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	holdingID, err := h.extractPathID(r.URL.Path, "/api/v1/portfolio/holdings/")
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid holding id")
		return
	}

	var req struct {
		AssetSymbol   string   `json:"asset_symbol"`
		Quantity      float64  `json:"quantity"`
		AvgBuyPrice   *float64 `json:"avg_buy_price,omitempty"`
		QuoteCurrency string   `json:"quote_currency"`
		Notes         string   `json:"notes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	quoteCurrency := req.QuoteCurrency
	if quoteCurrency == "" {
		quoteCurrency = "USD"
	}

	holding := &domain.Holding{
		ID:            holdingID,
		AssetSymbol:   req.AssetSymbol,
		Quantity:      req.Quantity,
		AvgBuyPrice:   req.AvgBuyPrice,
		QuoteCurrency: quoteCurrency,
		Source:        "manual",
		Notes:         req.Notes,
	}

	if err := h.svc.UpdateHolding(r.Context(), userID, holding); err != nil {
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, holding)
}

// DeleteHolding handles DELETE /api/v1/portfolio/holdings/{id}.
func (h *PortfolioHandler) DeleteHolding(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	holdingID, err := h.extractPathID(r.URL.Path, "/api/v1/portfolio/holdings/")
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid holding id")
		return
	}

	if err := h.svc.DeleteHolding(r.Context(), userID, holdingID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Valuation ---

// GetValuation handles GET /api/v1/portfolio/valuation.
func (h *PortfolioHandler) GetValuation(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	quoteCurrency := r.URL.Query().Get("quote")
	if quoteCurrency == "" {
		quoteCurrency = "USD"
	}

	valuation, err := h.ve.ComputeValuation(r.Context(), userID, quoteCurrency)
	if err != nil {
		slog.Error("compute valuation failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.writeJSON(w, http.StatusOK, valuation)
}

// GetHistory handles GET /api/v1/portfolio/history.
func (h *PortfolioHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	quoteCurrency := r.URL.Query().Get("quote")
	if quoteCurrency == "" {
		quoteCurrency = "USD"
	}

	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "7d"
	}

	points, err := h.ve.ComputeHistorical(r.Context(), userID, quoteCurrency, timeRange)
	if err != nil {
		slog.Error("compute historical failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"data": points})
}

// --- Exchange Credentials ---

// CreateCredential handles POST /api/v1/portfolio/credentials.
func (h *PortfolioHandler) CreateCredential(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	var req struct {
		Exchange  string `json:"exchange"`
		APIKey    string `json:"api_key"`
		APISecret string `json:"api_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Exchange == "" || req.APIKey == "" || req.APISecret == "" {
		h.writeJSONError(w, http.StatusBadRequest, "exchange, api_key, and api_secret are required")
		return
	}

	cred, err := h.svc.CreateCredential(r.Context(), userID, req.Exchange, req.APIKey, req.APISecret)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, cred)
}

// ListCredentials handles GET /api/v1/portfolio/credentials.
func (h *PortfolioHandler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	creds, err := h.svc.ListCredentials(r.Context(), userID)
	if err != nil {
		slog.Error("list credentials failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if creds == nil {
		creds = []domain.ExchangeCredential{}
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"data": creds})
}

// DeleteCredential handles DELETE /api/v1/portfolio/credentials/{id}.
func (h *PortfolioHandler) DeleteCredential(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	credID, err := h.extractPathID(r.URL.Path, "/api/v1/portfolio/credentials/")
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid credential id")
		return
	}

	if err := h.svc.DeleteCredential(r.Context(), userID, credID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SyncCredential handles POST /api/v1/portfolio/credentials/{id}/sync.
func (h *PortfolioHandler) SyncCredential(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	// Extract credential ID from path: /api/v1/portfolio/credentials/{id}/sync
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/portfolio/credentials/")
	path = strings.TrimSuffix(path, "/sync")
	credID, err := uuid.Parse(path)
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid credential id")
		return
	}

	if err := h.svc.SyncCredential(r.Context(), userID, credID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}

// --- Sync All ---

// SyncAll handles POST /api/v1/portfolio/sync-all.
func (h *PortfolioHandler) SyncAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	result := h.svc.SyncAll(r.Context(), userID)
	h.writeJSON(w, http.StatusOK, result)
}

// --- Wallets ---

// CreateWallet handles POST /api/v1/portfolio/wallets.
func (h *PortfolioHandler) CreateWallet(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	var req struct {
		Chain   string `json:"chain"`
		Address string `json:"address"`
		Label   string `json:"label,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Chain == "" || req.Address == "" {
		h.writeJSONError(w, http.StatusBadRequest, "chain and address are required")
		return
	}

	wallet, err := h.svc.CreateWallet(r.Context(), userID, req.Chain, req.Address, req.Label)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, wallet)
}

// ListWallets handles GET /api/v1/portfolio/wallets.
func (h *PortfolioHandler) ListWallets(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	wallets, err := h.svc.ListWallets(r.Context(), userID)
	if err != nil {
		slog.Error("list wallets failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if wallets == nil {
		wallets = []domain.WalletAddress{}
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"data": wallets})
}

// DeleteWallet handles DELETE /api/v1/portfolio/wallets/{id}.
func (h *PortfolioHandler) DeleteWallet(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	walletID, err := h.extractPathID(r.URL.Path, "/api/v1/portfolio/wallets/")
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid wallet id")
		return
	}

	if err := h.svc.DeleteWallet(r.Context(), userID, walletID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SyncWallet handles POST /api/v1/portfolio/wallets/{id}/sync.
func (h *PortfolioHandler) SyncWallet(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	// Extract wallet ID from path: /api/v1/portfolio/wallets/{id}/sync
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/portfolio/wallets/")
	path = strings.TrimSuffix(path, "/sync")
	walletID, err := uuid.Parse(path)
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid wallet id")
		return
	}

	if err := h.svc.SyncWallet(r.Context(), userID, walletID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}

// --- Ledger ---

// ConnectLedger handles POST /api/v1/portfolio/ledger/connect.
// Accepts a multipart file upload of a Ledger Live JSON export.
func (h *PortfolioHandler) ConnectLedger(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	// 10 MB limit for the multipart form.
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid multipart form or file too large")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "failed to read uploaded file")
		return
	}

	imported, err := h.svc.ImportLedgerExport(r.Context(), userID, data)
	if err != nil {
		if strings.Contains(err.Error(), "parsing ledger export") {
			h.writeJSONError(w, http.StatusBadRequest, "invalid ledger export file")
			return
		}
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]int{"imported": imported})
}

// SyncLedger handles POST /api/v1/portfolio/ledger/sync.
// With file-based import there is no stored credential to re-sync.
// Users should re-upload their Ledger Live export instead.
func (h *PortfolioHandler) SyncLedger(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{
		"message": "Ledger sync is not supported with file upload. Please re-upload your Ledger Live export.",
	})
}

// DisconnectLedger handles DELETE /api/v1/portfolio/ledger.
func (h *PortfolioHandler) DisconnectLedger(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	if err := h.svc.DisconnectLedger(r.Context(), userID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- SSE Stream ---

// StreamPortfolio handles GET /api/v1/portfolio/stream.
// It sends an initial portfolio valuation snapshot, then subscribes to ticker
// updates and pushes new valuations whenever a held asset's price changes.
// The connection closes when the client disconnects or the JWT session expires.
func (h *PortfolioHandler) StreamPortfolio(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireJWTUser(w, r)
	if !ok {
		return
	}

	if h.marketSvc == nil {
		h.writeJSONError(w, http.StatusServiceUnavailable, "portfolio streaming not available")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	quoteCurrency := r.URL.Query().Get("quote")
	if quoteCurrency == "" {
		quoteCurrency = "USD"
	}

	// Determine JWT expiry from claims for connection lifetime.
	claims, _ := middleware.ClaimsFromContext(r.Context())
	var sessionDeadline <-chan time.Time
	if claims != nil && claims.ExpiresAt != nil {
		remaining := time.Until(claims.ExpiresAt.Time)
		if remaining <= 0 {
			h.writeJSONError(w, http.StatusUnauthorized, "session expired")
			return
		}
		sessionDeadline = time.After(remaining)
	}

	ctx := r.Context()

	// Send initial portfolio valuation snapshot.
	val, err := h.ve.ComputeValuation(ctx, userID, quoteCurrency)
	if err != nil {
		slog.Error("portfolio stream initial valuation failed", "error", err)
		h.writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	data, _ := json.Marshal(val)
	if err := writeSSEEvent(w, "portfolio", data); err != nil {
		slog.Warn("portfolio stream initial write failed", "error", err)
		return
	}
	flusher.Flush()

	// Build the set of held asset symbols for filtering ticker updates.
	holdings, err := h.svc.ListHoldings(ctx, userID)
	if err != nil {
		slog.Error("portfolio stream list holdings failed", "error", err)
		return
	}
	heldAssets := portfolio.HeldAssetPairs(holdings, quoteCurrency)

	// Subscribe to ticker updates.
	sub := h.marketSvc.Subscribe()
	defer h.marketSvc.Unsubscribe(sub)

	for {
		select {
		case <-ctx.Done():
			return
		case <-sessionDeadline:
			slog.Debug("portfolio stream closing: JWT session expired", "user_id", userID)
			return
		case <-sub.Updates():
			pairs := sub.DrainPendingPairs()
			if len(pairs) == 0 {
				continue
			}

			// Check if any updated pair's base asset is in the user's holdings.
			relevant := portfolio.HasRelevantUpdate(heldAssets, pairs)
			if !relevant {
				continue
			}

			// Recompute full valuation and push.
			newVal, err := h.ve.ComputeValuation(ctx, userID, quoteCurrency)
			if err != nil {
				slog.Warn("portfolio stream valuation failed", "error", err)
				continue
			}

			valData, _ := json.Marshal(newVal)
			if err := writeSSEEvent(w, "portfolio", valData); err != nil {
				slog.Warn("portfolio stream write failed", "error", err)
				return
			}
			flusher.Flush()
		}
	}
}

// writeSSEEvent writes a named SSE event with the given data.
func writeSSEEvent(w http.ResponseWriter, event string, data []byte) error {
	_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(data))
	return err
}

// --- Helpers ---

// extractPathID extracts a UUID from the URL path after the given prefix.
func (h *PortfolioHandler) extractPathID(path, prefix string) (uuid.UUID, error) {
	idStr := strings.TrimPrefix(path, prefix)
	idStr = strings.Split(idStr, "/")[0]
	return uuid.Parse(idStr)
}

// handleServiceError maps portfolio service errors to HTTP responses.
func (h *PortfolioHandler) handleServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, portfolio.ErrForbidden) {
		h.writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}

	if errors.Is(err, portfolio.ErrDuplicateCredential) {
		h.writeJSONError(w, http.StatusConflict, err.Error())
		return
	}

	// Map known validation errors to 400.
	if errors.Is(err, portfolio.ErrNonPositiveQuantity) ||
		errors.Is(err, portfolio.ErrInvalidAssetSymbol) ||
		errors.Is(err, portfolio.ErrUnsupportedExchange) ||
		errors.Is(err, portfolio.ErrUnsupportedChain) ||
		errors.Is(err, portfolio.ErrInvalidEthereumAddress) ||
		errors.Is(err, portfolio.ErrInvalidSolanaAddress) ||
		errors.Is(err, portfolio.ErrInvalidBitcoinAddress) {
		h.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	slog.Error("portfolio service error", "error", err)
	h.writeJSONError(w, http.StatusInternalServerError, "internal error")
}

func (h *PortfolioHandler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *PortfolioHandler) writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
