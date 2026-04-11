package service

import (
	"context"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/news"
)

type mockNewsRepo struct {
	items []news.News
	err   error
}

func (m *mockNewsRepo) UpsertNews(ctx context.Context, items []news.News) error {
	m.items = append(m.items, items...)
	return m.err
}

func (m *mockNewsRepo) ListNews(ctx context.Context, limit int) ([]news.News, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.items) > limit {
		return m.items[:limit], nil
	}
	return m.items, nil
}

func TestNewsService_List(t *testing.T) {
	repo := &mockNewsRepo{
		items: []news.News{
			{ID: "1", Title: "News 1", PublishedAt: time.Now()},
			{ID: "2", Title: "News 2", PublishedAt: time.Now().Add(-time.Hour)},
		},
	}
	svc := NewNewsService(repo)

	items, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != "1" {
		t.Errorf("expected ID 1, got %s", items[0].ID)
	}
}

func TestNewsService_SubscribeNotify(t *testing.T) {
	repo := &mockNewsRepo{}
	svc := NewNewsService(repo)

	ch := svc.Subscribe()
	defer svc.Unsubscribe(ch)

	svc.notify()

	select {
	case <-ch:
		// works
	case <-time.After(100 * time.Millisecond):
		t.Errorf("expected notification, but timed out")
	}
}

func TestParseRSSDate(t *testing.T) {
	dates := []string{
		"Mon, 02 Jan 2006 15:04:05 GMT",
		"02 Jan 2006 15:04:05 -0700",
		"02 Jan 2006 15:04:05 UTC",
	}
	for _, d := range dates {
		t.Run(d, func(t *testing.T) {
			_, err := parseRSSDate(d)
			if err != nil {
				t.Errorf("failed to parse date %q: %v", d, err)
			}
		})
	}
}

func TestNewsService_FetchSourceUnknown(t *testing.T) {
	repo := &mockNewsRepo{}
	svc := NewNewsService(repo)

	err := svc.FetchSource(context.Background(), "unknown_source")
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
}

func TestNewsService_SourceURLMapContainsAllSources(t *testing.T) {
	// Verify the source URL map has entries for all known source keys.
	expectedSources := []string{"coindesk", "cointelegraph", "theblock"}
	for _, src := range expectedSources {
		if _, ok := sourceURLMap[src]; !ok {
			t.Fatalf("sourceURLMap missing entry for %q", src)
		}
	}
}
