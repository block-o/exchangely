package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
)

type AdvisoryPairLocker struct {
	db *sql.DB
}

func NewAdvisoryPairLocker(db *sql.DB) *AdvisoryPairLocker {
	return &AdvisoryPairLocker{db: db}
}

func (l *AdvisoryPairLocker) Lock(ctx context.Context, pairSymbol string) (func() error, error) {
	conn, err := l.db.Conn(ctx)
	if err != nil {
		return nil, err
	}

	key := advisoryKey(pairSymbol)
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return func() error {
		defer func() {
			_ = conn.Close()
		}()
		if _, err := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, key); err != nil {
			return fmt.Errorf("unlock %s: %w", pairSymbol, err)
		}
		return nil
	}, nil
}

func advisoryKey(pairSymbol string) int64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(pairSymbol))
	return int64(hasher.Sum64())
}
