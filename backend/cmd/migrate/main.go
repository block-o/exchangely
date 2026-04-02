package main

import (
	"context"
	"log/slog"
	"os"

	postgresrepo "github.com/block-o/exchangely/backend/internal/storage/postgres"
	"github.com/block-o/exchangely/backend/internal/telemetry"
)

func main() {
	logger := telemetry.ConfigureLogger(slog.LevelInfo)
	dsn := os.Getenv("BACKEND_DATABASE_URL")
	if dsn == "" {
		logger.Error("missing required environment variable", "key", "BACKEND_DATABASE_URL")
		os.Exit(1)
	}

	db, err := postgresrepo.Open(context.Background(), dsn)
	if err != nil {
		logger.Error("database open failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := postgresrepo.Migrate(context.Background(), db, "migrations"); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
}
