package auth

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// APIRateLimitConfig holds configuration for the API rate limiter.
type APIRateLimitConfig struct {
	UserLimit    int
	PremiumLimit int
	AdminLimit   int
	IPLimit      int
	Window       time.Duration
}

// DefaultAPIRateLimitConfig returns the default configuration.
func DefaultAPIRateLimitConfig() APIRateLimitConfig {
	return APIRateLimitConfig{
		UserLimit:    100,
		PremiumLimit: 500,
		AdminLimit:   1000,
		IPLimit:      200,
		Window:       time.Minute,
	}
}

// APIRateLimiter enforces tiered rate limits using PostgreSQL sliding window counters.
type APIRateLimiter struct {
	repo      RateLimitRepository
	tiers     map[string]int // role → max requests per window
	ipLimit   int
	window    time.Duration
	pruneOnce sync.Once
}

// NewAPIRateLimiter creates a new APIRateLimiter from the given config.
func NewAPIRateLimiter(repo RateLimitRepository, cfg APIRateLimitConfig) *APIRateLimiter {
	return &APIRateLimiter{
		repo: repo,
		tiers: map[string]int{
			"user":    cfg.UserLimit,
			"premium": cfg.PremiumLimit,
			"admin":   cfg.AdminLimit,
		},
		ipLimit: cfg.IPLimit,
		window:  cfg.Window,
	}
}

// LimitForRole returns the configured rate limit for the given role.
// Unknown roles default to the user tier.
func (rl *APIRateLimiter) LimitForRole(role string) int {
	if limit, ok := rl.tiers[role]; ok {
		return limit
	}
	return rl.tiers["user"]
}

// Check performs a per-token/user rate limit check and a per-IP rate limit check.
// It returns the most restrictive result. If either DB call fails, it logs a
// warning and returns Allowed: true (fail-open).
func (rl *APIRateLimiter) Check(ctx context.Context, tokenID *uuid.UUID, userID *uuid.UUID, role string, ip string) (RateLimitResult, error) {
	now := time.Now()
	limit := rl.LimitForRole(role)

	// Per-token/user check.
	tokenCount, err := rl.repo.CheckAndIncrement(ctx, tokenID, userID, ip, rl.window)
	if err != nil {
		slog.Warn("rate limit check failed, allowing request (fail-open)",
			"error", err,
			"check", "token/user",
		)
		return RateLimitResult{
			Allowed:   true,
			Limit:     limit,
			Remaining: limit,
			ResetAt:   now.Add(rl.window),
		}, nil
	}

	// Per-IP check.
	ipCount, err := rl.repo.CheckIPAndIncrement(ctx, ip, rl.window)
	if err != nil {
		slog.Warn("rate limit check failed, allowing request (fail-open)",
			"error", err,
			"check", "ip",
		)
		return RateLimitResult{
			Allowed:   true,
			Limit:     limit,
			Remaining: limit,
			ResetAt:   now.Add(rl.window),
		}, nil
	}

	resetAt := now.Add(rl.window)

	// Determine if either check exceeds its limit.
	tokenAllowed := tokenCount <= limit
	ipAllowed := ipCount <= rl.ipLimit

	allowed := tokenAllowed && ipAllowed

	remaining := limit - tokenCount
	if remaining < 0 {
		remaining = 0
	}

	// If IP limit is more restrictive, use that remaining count.
	ipRemaining := rl.ipLimit - ipCount
	if ipRemaining < 0 {
		ipRemaining = 0
	}
	if ipRemaining < remaining {
		remaining = ipRemaining
	}

	return RateLimitResult{
		Allowed:   allowed,
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   resetAt,
	}, nil
}

// StartPruner launches a background goroutine that calls PruneExpired at most
// once per minute. It is safe to call multiple times; only the first call starts
// the goroutine.
func (rl *APIRateLimiter) StartPruner(ctx context.Context) {
	rl.pruneOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					pruned, err := rl.repo.PruneExpired(ctx, rl.window)
					if err != nil {
						slog.Warn("rate limit prune failed",
							"error", err,
						)
						continue
					}
					if pruned > 0 {
						slog.Info("pruned expired rate limit entries",
							"count", pruned,
						)
					}
				}
			}
		}()
	})
}
