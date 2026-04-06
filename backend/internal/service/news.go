package service

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/news"
)

// NewsRepository defines the persistence interface for news items.
type NewsRepository interface {
	UpsertNews(ctx context.Context, items []news.News) error
	ListNews(ctx context.Context, limit int) ([]news.News, error)
}

// NewsService orchestrates news fetching from RSS sources, persistence, and
// real-time notifications via channels.
type NewsService struct {
	repo    NewsRepository
	sources []string

	mu          sync.RWMutex
	subscribers map[chan struct{}]struct{}
}

// NewNewsService initializes a NewsService with a repository and default RSS sources.
func NewNewsService(repo NewsRepository) *NewsService {
	return &NewsService{
		repo: repo,
		sources: []string{
			"https://www.coindesk.com/arc/outboundfeeds/rss/",
			"https://cointelegraph.com/rss",
			"https://www.theblock.co/rss.xml",
		},
		subscribers: make(map[chan struct{}]struct{}),
	}
}

// FetchLatest pulls news items from all sources and persists them to the repository.
// It notifies all active subscribers upon success.
func (s *NewsService) FetchLatest(ctx context.Context) error {
	var allNews []news.News
	for _, url := range s.sources {
		items, err := s.fetchRSS(ctx, url)
		if err != nil {
			slog.Error("failed to fetch RSS", "url", url, "error", err)
			continue
		}
		allNews = append(allNews, items...)
	}

	if len(allNews) == 0 {
		return nil
	}

	if err := s.repo.UpsertNews(ctx, allNews); err != nil {
		return err
	}

	s.notify()
	return nil
}

// List returns the most recent news items up to the specified limit.
func (s *NewsService) List(ctx context.Context, limit int) ([]news.News, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.ListNews(ctx, limit)
}

// Subscribe creates and registers a channel to receive notifications when new news are fetched.
func (s *NewsService) Subscribe() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan struct{}, 1)
	s.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a notification channel from the active subscribers list.
func (s *NewsService) Unsubscribe(ch chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subscribers, ch)
}

func (s *NewsService) notify() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

type rssRoot struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	PubDate string `xml:"pubDate"`
	GUID    string `xml:"guid"`
}

func (s *NewsService) fetchRSS(ctx context.Context, url string) ([]news.News, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	// Add user agent to avoid some blocks
	req.Header.Set("User-Agent", "Exchangely/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var root rssRoot
	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, err
	}

	source := "Source"
	if strings.Contains(url, "coindesk") {
		source = "CoinDesk"
	} else if strings.Contains(url, "cointelegraph") {
		source = "Cointelegraph"
	} else if strings.Contains(url, "theblock") {
		source = "TheBlock"
	}

	var items []news.News
	for _, item := range root.Channel.Items {
		pubAt, err := parseRSSDate(item.PubDate)
		if err != nil {
			slog.Warn("failed to parse RSS date", "date", item.PubDate, "error", err)
			pubAt = time.Now()
		}

		id := item.GUID
		if id == "" {
			id = item.Link
		}

		items = append(items, news.News{
			ID:          id,
			Title:       item.Title,
			Link:        item.Link,
			Source:      source,
			PublishedAt: pubAt.UTC(),
		})
	}

	return items, nil
}

func parseRSSDate(d string) (time.Time, error) {
	layouts := []string{
		time.RFC1123,
		time.RFC1123Z,
		"Mon, 02 Jan 2006 15:04:05 -0700", // RFC1123Z with comma
		"02 Jan 2006 15:04:05 -0700",      // RFC1123Z without weekday
		"02 Jan 2006 15:04:05 MST",        // RFC1123 without weekday
		"2006-01-02T15:04:05Z07:00",       // ISO8601/RFC3339
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, d); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unknown date format: %q", d)
}
