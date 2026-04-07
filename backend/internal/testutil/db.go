package testutil

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/storage/postgres"
)

func SetupTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("EXCHANGELY_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/exchangely_test?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Skipf("Skipping integration test: database not available at %s", dsn)
	}

	return db
}
