package main

import (
	"context"
	"log"
	"os"

	postgresrepo "github.com/block-o/exchangely/backend/internal/storage/postgres"
)

func main() {
	dsn := os.Getenv("BACKEND_DATABASE_URL")
	if dsn == "" {
		log.Fatal("BACKEND_DATABASE_URL is required")
	}

	db, err := postgresrepo.Open(context.Background(), dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := postgresrepo.Migrate(context.Background(), db, "migrations"); err != nil {
		log.Fatal(err)
	}
}
