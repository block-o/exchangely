package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// BenchmarkRateLimiterCheck measures Check throughput with a mock repository,
// exercising the per-token + per-IP dual check path.
func BenchmarkRateLimiterCheck(b *testing.B) {
	repo := &mockRateLimitRepo{tokenCount: 10, ipCount: 5}
	cfg := APIRateLimitConfig{
		UserLimit:    100,
		PremiumLimit: 500,
		AdminLimit:   1000,
		IPLimit:      200,
		Window:       time.Minute,
	}
	rl := NewAPIRateLimiter(repo, cfg)

	tokenID := uuid.New()
	userID := uuid.New()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rl.Check(ctx, &tokenID, &userID, "user", "192.168.1.1")
		if err != nil {
			b.Fatalf("Check failed: %v", err)
		}
	}
}

// BenchmarkLimitForRole measures tier resolution for known and unknown roles.
func BenchmarkLimitForRole(b *testing.B) {
	cfg := DefaultAPIRateLimitConfig()
	rl := NewAPIRateLimiter(nil, cfg)

	roles := []string{"user", "premium", "admin", "unknown", "guest"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.LimitForRole(roles[i%len(roles)])
	}
}

// BenchmarkRateLimiterCheck_Parallel verifies concurrent rate limit checks
// don't race using b.RunParallel.
func BenchmarkRateLimiterCheck_Parallel(b *testing.B) {
	repo := &mockRateLimitRepo{tokenCount: 10, ipCount: 5}
	cfg := APIRateLimitConfig{
		UserLimit:    100,
		PremiumLimit: 500,
		AdminLimit:   1000,
		IPLimit:      200,
		Window:       time.Minute,
	}
	rl := NewAPIRateLimiter(repo, cfg)

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		tokenID := uuid.New()
		userID := uuid.New()
		for pb.Next() {
			_, err := rl.Check(ctx, &tokenID, &userID, "premium", "10.0.0.1")
			if err != nil {
				b.Errorf("Check failed: %v", err)
			}
		}
	})
}
