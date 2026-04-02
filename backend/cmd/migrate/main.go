package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := os.Getenv("BACKEND_DATABASE_URL")
	if dsn == "" {
		log.Fatal("BACKEND_DATABASE_URL is required")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	files, err := os.ReadDir("migrations")
	if err != nil {
		log.Fatal(err)
	}

	names := make([]string, 0, len(files))
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".up.sql") {
			names = append(names, file.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		body, err := os.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			log.Fatal(err)
		}
		if _, err := db.ExecContext(context.Background(), string(body)); err != nil {
			log.Fatalf("migration %s failed: %v", name, err)
		}
	}
}
