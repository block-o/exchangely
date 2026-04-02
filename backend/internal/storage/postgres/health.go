package postgres

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type HealthChecker struct {
	dsn string
	db  *sql.DB
}

func NewHealthChecker(dsn string) *HealthChecker {
	return &HealthChecker{dsn: dsn}
}

func (h *HealthChecker) Ping(ctx context.Context) error {
	if h.dsn == "" {
		return sql.ErrConnDone
	}

	if h.db == nil {
		db, err := sql.Open("pgx", h.dsn)
		if err != nil {
			return err
		}
		h.db = db
	}

	return h.db.PingContext(ctx)
}
