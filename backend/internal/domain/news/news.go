// Package news defines the domain model for market news items.
package news

import "time"

// News represents a single news headline fetched from an external source.
// It includes metadata about the source and publication time.
type News struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Link        string    `json:"link"`
	Source      string    `json:"source"`
	PublishedAt time.Time `json:"published_at"`
}
