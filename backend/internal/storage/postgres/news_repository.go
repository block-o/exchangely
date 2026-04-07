package postgres

import (
	"context"
	"database/sql"

	"github.com/block-o/exchangely/backend/internal/domain/news"
)

// NewsRepository persists news items into the PostgreSQL database.
type NewsRepository struct {
	db *sql.DB
}

// NewNewsRepository initializes a NewsRepository with a DB handle.
func NewNewsRepository(db *sql.DB) *NewsRepository {
	return &NewsRepository{db: db}
}

// UpsertNews inserts a collection of news items, updating the existing ones
// based on their ID in case of conflict.
func (r *NewsRepository) UpsertNews(ctx context.Context, items []news.News) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, item := range items {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO news (id, title, link, source, published_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO UPDATE
			SET title = EXCLUDED.title,
			    link = EXCLUDED.link,
			    source = EXCLUDED.source,
			    published_at = EXCLUDED.published_at
		`, item.ID, item.Title, item.Link, item.Source, item.PublishedAt)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListNews returns the latest news items ordered by publication date.
func (r *NewsRepository) ListNews(ctx context.Context, limit int) ([]news.News, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, link, source, published_at
		FROM news
		ORDER BY published_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []news.News
	for rows.Next() {
		var item news.News
		if err := rows.Scan(&item.ID, &item.Title, &item.Link, &item.Source, &item.PublishedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}
