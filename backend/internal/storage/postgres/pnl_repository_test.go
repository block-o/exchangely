package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/google/uuid"
)

func newPnLTestFixture(t *testing.T) (*PnLRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewPnLRepository(db), mock
}

func samplePnLSnapshot() portfolio.PnLSnapshot {
	return portfolio.PnLSnapshot{
		ID:                uuid.MustParse("cccccccc-dddd-eeee-ffff-000000000001"),
		UserID:            uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		ReferenceCurrency: "USD",
		TotalRealized:     1500.50,
		TotalUnrealized:   3200.75,
		TotalPnL:          4701.25,
		HasApproximate:    true,
		ExcludedCount:     2,
		Assets: []portfolio.AssetPnL{
			{
				AssetSymbol:      "BTC",
				RealizedPnL:      1000.0,
				UnrealizedPnL:    2500.0,
				TotalPnL:         3500.0,
				TransactionCount: 5,
			},
			{
				AssetSymbol:      "ETH",
				RealizedPnL:      500.50,
				UnrealizedPnL:    700.75,
				TotalPnL:         1201.25,
				TransactionCount: 3,
			},
		},
		ComputedAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestPnLRepository_Upsert(t *testing.T) {
	repo, mock := newPnLTestFixture(t)
	ctx := context.Background()
	snap := samplePnLSnapshot()

	assetsJSON, err := json.Marshal(snap.Assets)
	if err != nil {
		t.Fatalf("marshal assets: %v", err)
	}

	returnedID := uuid.MustParse("cccccccc-dddd-eeee-ffff-000000000001")
	rows := sqlmock.NewRows([]string{"id"}).AddRow(returnedID)

	mock.ExpectQuery(`INSERT INTO pnl_snapshots`).
		WithArgs(
			snap.ID, snap.UserID, snap.ReferenceCurrency,
			snap.TotalRealized, snap.TotalUnrealized, snap.TotalPnL,
			snap.HasApproximate, snap.ExcludedCount, assetsJSON, snap.ComputedAt,
		).
		WillReturnRows(rows)

	err = repo.Upsert(ctx, &snap)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if snap.ID != returnedID {
		t.Errorf("expected ID %v, got %v", returnedID, snap.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPnLRepository_FindByUser_Found(t *testing.T) {
	repo, mock := newPnLTestFixture(t)
	ctx := context.Background()

	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	snapID := uuid.MustParse("cccccccc-dddd-eeee-ffff-000000000001")
	computedAt := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	assets := []portfolio.AssetPnL{
		{AssetSymbol: "BTC", RealizedPnL: 1000.0, UnrealizedPnL: 2500.0, TotalPnL: 3500.0, TransactionCount: 5},
	}
	assetsJSON, _ := json.Marshal(assets)

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "reference_currency", "total_realized", "total_unrealized",
		"total_pnl", "has_approximate", "excluded_count", "assets_json", "computed_at",
	}).AddRow(
		snapID, userID, "USD", 1500.50, 3200.75,
		4701.25, true, 2, assetsJSON, computedAt,
	)

	mock.ExpectQuery(`SELECT .+ FROM pnl_snapshots WHERE user_id = .+ AND reference_currency = .+`).
		WithArgs(userID, "USD").
		WillReturnRows(rows)

	result, err := repo.FindByUser(ctx, userID, "USD")
	if err != nil {
		t.Fatalf("FindByUser failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != snapID {
		t.Errorf("expected ID %v, got %v", snapID, result.ID)
	}
	if result.TotalPnL != 4701.25 {
		t.Errorf("expected TotalPnL 4701.25, got %f", result.TotalPnL)
	}
	if result.HasApproximate != true {
		t.Error("expected HasApproximate true")
	}
	if len(result.Assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(result.Assets))
	}
	if result.Assets[0].AssetSymbol != "BTC" {
		t.Errorf("expected asset BTC, got %s", result.Assets[0].AssetSymbol)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPnLRepository_FindByUser_NotFound(t *testing.T) {
	repo, mock := newPnLTestFixture(t)
	ctx := context.Background()

	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	mock.ExpectQuery(`SELECT .+ FROM pnl_snapshots WHERE user_id = .+ AND reference_currency = .+`).
		WithArgs(userID, "EUR").
		WillReturnError(sql.ErrNoRows)

	result, err := repo.FindByUser(ctx, userID, "EUR")
	if err != nil {
		t.Fatalf("FindByUser should not return error for not found, got: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result for not found")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
