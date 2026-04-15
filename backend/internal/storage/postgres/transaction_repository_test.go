package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/google/uuid"
)

func newTxTestFixture(t *testing.T) (*TransactionRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewTransactionRepository(db), mock
}

func sampleTransaction() portfolio.Transaction {
	refVal := 50000.0
	return portfolio.Transaction{
		ID:                uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
		UserID:            uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		AssetSymbol:       "BTC",
		Quantity:          1.5,
		Type:              "buy",
		Timestamp:         time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Source:            "binance",
		SourceRef:         "cred-abc",
		ReferenceValue:    &refVal,
		ReferenceCurrency: "USD",
		Resolution:        "hourly",
		ManuallyEdited:    false,
		Notes:             "test note",
	}
}

func TestTransactionRepository_Create(t *testing.T) {
	repo, mock := newTxTestFixture(t)
	ctx := context.Background()
	tx := sampleTransaction()

	now := time.Now().UTC().Truncate(time.Second)
	rows := sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now)

	mock.ExpectQuery(`INSERT INTO portfolio_transactions`).
		WithArgs(
			tx.ID, tx.UserID, tx.AssetSymbol, tx.Quantity, tx.Type, tx.Timestamp,
			tx.Source, tx.SourceRef, tx.ReferenceValue, tx.ReferenceCurrency,
			tx.Resolution, tx.ManuallyEdited, tx.Notes, tx.Fee, tx.FeeCurrency,
		).
		WillReturnRows(rows)

	err := repo.Create(ctx, &tx)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if !tx.CreatedAt.Equal(now) {
		t.Errorf("expected CreatedAt %v, got %v", now, tx.CreatedAt)
	}
	if !tx.UpdatedAt.Equal(now) {
		t.Errorf("expected UpdatedAt %v, got %v", now, tx.UpdatedAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTransactionRepository_UpsertInsertsNewRow(t *testing.T) {
	repo, mock := newTxTestFixture(t)
	ctx := context.Background()
	tx := sampleTransaction()

	now := time.Now().UTC().Truncate(time.Second)
	rows := sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now)

	mock.ExpectQuery(`INSERT INTO portfolio_transactions`).
		WithArgs(
			tx.ID, tx.UserID, tx.AssetSymbol, tx.Quantity, tx.Type, tx.Timestamp,
			tx.Source, tx.SourceRef, tx.ReferenceValue, tx.ReferenceCurrency,
			tx.Resolution, tx.ManuallyEdited, tx.Notes, tx.Fee, tx.FeeCurrency,
		).
		WillReturnRows(rows)

	err := repo.Upsert(ctx, &tx)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if !tx.CreatedAt.Equal(now) {
		t.Errorf("expected CreatedAt %v, got %v", now, tx.CreatedAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTransactionRepository_UpsertSkipsManuallyEdited(t *testing.T) {
	repo, mock := newTxTestFixture(t)
	ctx := context.Background()
	tx := sampleTransaction()

	// When the existing row has manually_edited=true, the WHERE NOT clause
	// prevents the update, so no rows are returned → sql.ErrNoRows.
	mock.ExpectQuery(`INSERT INTO portfolio_transactions`).
		WithArgs(
			tx.ID, tx.UserID, tx.AssetSymbol, tx.Quantity, tx.Type, tx.Timestamp,
			tx.Source, tx.SourceRef, tx.ReferenceValue, tx.ReferenceCurrency,
			tx.Resolution, tx.ManuallyEdited, tx.Notes, tx.Fee, tx.FeeCurrency,
		).
		WillReturnError(sql.ErrNoRows)

	err := repo.Upsert(ctx, &tx)
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows when manually_edited, got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTransactionRepository_FindByID_Found(t *testing.T) {
	repo, mock := newTxTestFixture(t)
	ctx := context.Background()

	txID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	refVal := 50000.0
	now := time.Now().UTC().Truncate(time.Second)
	ts := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "asset_symbol", "quantity", "tx_type", "tx_timestamp",
		"source", "source_ref", "reference_value", "reference_currency",
		"resolution", "manually_edited", "notes", "fee", "fee_currency",
		"created_at", "updated_at",
	}).AddRow(
		txID, userID, "BTC", 1.5, "buy", ts,
		"binance", "cred-abc", &refVal, "USD",
		"hourly", false, "test note", nil, "", now, now,
	)

	mock.ExpectQuery(`SELECT .+ FROM portfolio_transactions WHERE id = .+ AND user_id = .+`).
		WithArgs(txID, userID).
		WillReturnRows(rows)

	result, err := repo.FindByID(ctx, userID, txID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != txID {
		t.Errorf("expected ID %v, got %v", txID, result.ID)
	}
	if result.AssetSymbol != "BTC" {
		t.Errorf("expected AssetSymbol BTC, got %s", result.AssetSymbol)
	}
	if result.UserID != userID {
		t.Errorf("expected UserID %v, got %v", userID, result.UserID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTransactionRepository_FindByID_NotFound(t *testing.T) {
	repo, mock := newTxTestFixture(t)
	ctx := context.Background()

	txID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	mock.ExpectQuery(`SELECT .+ FROM portfolio_transactions WHERE id = .+ AND user_id = .+`).
		WithArgs(txID, userID).
		WillReturnError(sql.ErrNoRows)

	result, err := repo.FindByID(ctx, userID, txID)
	if err != nil {
		t.Fatalf("FindByID should not return error for not found, got: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result for not found")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTransactionRepository_ListByUser_NoFilters(t *testing.T) {
	repo, mock := newTxTestFixture(t)
	ctx := context.Background()

	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	txID1 := uuid.MustParse("aaaaaaaa-0001-0001-0001-000000000001")
	txID2 := uuid.MustParse("aaaaaaaa-0001-0001-0001-000000000002")
	refVal := 50000.0
	now := time.Now().UTC().Truncate(time.Second)
	ts := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "asset_symbol", "quantity", "tx_type", "tx_timestamp",
		"source", "source_ref", "reference_value", "reference_currency",
		"resolution", "manually_edited", "notes", "fee", "fee_currency",
		"created_at", "updated_at",
		"total_count",
	}).
		AddRow(txID1, userID, "BTC", 1.5, "buy", ts, "binance", "cred-abc", &refVal, "USD", "hourly", false, "", nil, "", now, now, 2).
		AddRow(txID2, userID, "ETH", 10.0, "sell", ts, "kraken", "cred-xyz", &refVal, "USD", "daily", false, "", nil, "", now, now, 2)

	mock.ExpectQuery(`SELECT .+ FROM portfolio_transactions WHERE user_id = .+`).
		WithArgs(userID, 50, 0).
		WillReturnRows(rows)

	txs, total, err := repo.ListByUser(ctx, userID, portfolio.ListOptions{})
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(txs) != 2 {
		t.Errorf("expected 2 transactions, got %d", len(txs))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTransactionRepository_ListByUser_WithAssetFilter(t *testing.T) {
	repo, mock := newTxTestFixture(t)
	ctx := context.Background()

	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	txID := uuid.MustParse("aaaaaaaa-0001-0001-0001-000000000001")
	refVal := 50000.0
	now := time.Now().UTC().Truncate(time.Second)
	ts := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "asset_symbol", "quantity", "tx_type", "tx_timestamp",
		"source", "source_ref", "reference_value", "reference_currency",
		"resolution", "manually_edited", "notes", "fee", "fee_currency",
		"created_at", "updated_at",
		"total_count",
	}).
		AddRow(txID, userID, "BTC", 1.5, "buy", ts, "binance", "cred-abc", &refVal, "USD", "hourly", false, "", nil, "", now, now, 1)

	// With asset filter: args are userID, "BTC", pageSize, offset
	mock.ExpectQuery(`SELECT .+ FROM portfolio_transactions WHERE user_id = .+`).
		WithArgs(userID, "BTC", 50, 0).
		WillReturnRows(rows)

	txs, total, err := repo.ListByUser(ctx, userID, portfolio.ListOptions{Asset: "BTC"})
	if err != nil {
		t.Fatalf("ListByUser with asset filter failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if len(txs) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(txs))
	}
	if txs[0].AssetSymbol != "BTC" {
		t.Errorf("expected asset BTC, got %s", txs[0].AssetSymbol)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTransactionRepository_ListByUser_WithPagination(t *testing.T) {
	repo, mock := newTxTestFixture(t)
	ctx := context.Background()

	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	txID := uuid.MustParse("aaaaaaaa-0001-0001-0001-000000000001")
	refVal := 50000.0
	now := time.Now().UTC().Truncate(time.Second)
	ts := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "asset_symbol", "quantity", "tx_type", "tx_timestamp",
		"source", "source_ref", "reference_value", "reference_currency",
		"resolution", "manually_edited", "notes", "fee", "fee_currency",
		"created_at", "updated_at",
		"total_count",
	}).
		AddRow(txID, userID, "BTC", 1.5, "buy", ts, "binance", "cred-abc", &refVal, "USD", "hourly", false, "", nil, "", now, now, 25)

	// Page 3, PageSize 10 → offset = (3-1)*10 = 20
	mock.ExpectQuery(`SELECT .+ FROM portfolio_transactions WHERE user_id = .+`).
		WithArgs(userID, 10, 20).
		WillReturnRows(rows)

	txs, total, err := repo.ListByUser(ctx, userID, portfolio.ListOptions{Page: 3, PageSize: 10})
	if err != nil {
		t.Fatalf("ListByUser with pagination failed: %v", err)
	}
	if total != 25 {
		t.Errorf("expected total 25, got %d", total)
	}
	if len(txs) != 1 {
		t.Errorf("expected 1 transaction on this page, got %d", len(txs))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTransactionRepository_DeleteBySourceRef(t *testing.T) {
	repo, mock := newTxTestFixture(t)
	ctx := context.Background()

	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	mock.ExpectExec(`DELETE FROM portfolio_transactions WHERE user_id = .+ AND source = .+ AND source_ref = .+`).
		WithArgs(userID, "binance", "cred-abc").
		WillReturnResult(sqlmock.NewResult(0, 3))

	err := repo.DeleteBySourceRef(ctx, userID, "binance", "cred-abc")
	if err != nil {
		t.Fatalf("DeleteBySourceRef failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTransactionRepository_CountByUser(t *testing.T) {
	repo, mock := newTxTestFixture(t)
	ctx := context.Background()

	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	rows := sqlmock.NewRows([]string{"count"}).AddRow(42)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM portfolio_transactions WHERE user_id = .+`).
		WithArgs(userID).
		WillReturnRows(rows)

	count, err := repo.CountByUser(ctx, userID)
	if err != nil {
		t.Fatalf("CountByUser failed: %v", err)
	}
	if count != 42 {
		t.Errorf("expected count 42, got %d", count)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTransactionRepository_DistinctCurrencies(t *testing.T) {
	t.Run("returns distinct currencies", func(t *testing.T) {
		repo, mock := newTxTestFixture(t)
		ctx := context.Background()

		userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

		rows := sqlmock.NewRows([]string{"reference_currency"}).
			AddRow("USD").
			AddRow("EUR")
		mock.ExpectQuery(`SELECT DISTINCT reference_currency FROM portfolio_transactions WHERE user_id = .+`).
			WithArgs(userID).
			WillReturnRows(rows)

		currencies, err := repo.DistinctCurrencies(ctx, userID)
		if err != nil {
			t.Fatalf("DistinctCurrencies failed: %v", err)
		}
		if len(currencies) != 2 {
			t.Fatalf("expected 2 currencies, got %d", len(currencies))
		}
		if currencies[0] != "USD" || currencies[1] != "EUR" {
			t.Errorf("expected [USD EUR], got %v", currencies)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("returns empty slice when no rows", func(t *testing.T) {
		repo, mock := newTxTestFixture(t)
		ctx := context.Background()

		userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

		rows := sqlmock.NewRows([]string{"reference_currency"})
		mock.ExpectQuery(`SELECT DISTINCT reference_currency FROM portfolio_transactions WHERE user_id = .+`).
			WithArgs(userID).
			WillReturnRows(rows)

		currencies, err := repo.DistinctCurrencies(ctx, userID)
		if err != nil {
			t.Fatalf("DistinctCurrencies failed: %v", err)
		}
		if len(currencies) != 0 {
			t.Errorf("expected empty slice, got %v", currencies)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}
