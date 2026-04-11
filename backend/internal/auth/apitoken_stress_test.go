package auth

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

// atomicAPITokenRepo wraps mockAPITokenRepo and serialises the
// CountActiveByUserID → Create pair so the max-5-active invariant can be
// verified under concurrent access. This simulates the row-level locking a
// real PostgreSQL transaction would provide.
type atomicAPITokenRepo struct {
	inner *mockAPITokenRepo
	mu    sync.Mutex // guards the count-then-create critical section
}

func newAtomicAPITokenRepo() *atomicAPITokenRepo {
	return &atomicAPITokenRepo{inner: newMockAPITokenRepo()}
}

func (r *atomicAPITokenRepo) Create(ctx context.Context, token *APIToken) error {
	// Create is called inside the locked section via CreateToken, but the
	// lock is held by CountActiveByUserID's caller. We delegate directly.
	return r.inner.Create(ctx, token)
}

func (r *atomicAPITokenRepo) FindByTokenHash(ctx context.Context, hash string) (*APIToken, error) {
	return r.inner.FindByTokenHash(ctx, hash)
}

func (r *atomicAPITokenRepo) ListByUserID(ctx context.Context, uid uuid.UUID) ([]APIToken, error) {
	return r.inner.ListByUserID(ctx, uid)
}

func (r *atomicAPITokenRepo) CountActiveByUserID(ctx context.Context, uid uuid.UUID) (int, error) {
	return r.inner.CountActiveByUserID(ctx, uid)
}

func (r *atomicAPITokenRepo) Revoke(ctx context.Context, id uuid.UUID, uid uuid.UUID) error {
	return r.inner.Revoke(ctx, id, uid)
}

func (r *atomicAPITokenRepo) UpdateLastUsedAt(ctx context.Context, id uuid.UUID, t time.Time) error {
	return r.inner.UpdateLastUsedAt(ctx, id, t)
}

// lockedCreateToken serialises the entire CreateToken call so that the
// count-check + insert pair is atomic, matching real DB transaction semantics.
func (r *atomicAPITokenRepo) lockedCreateToken(svc *APITokenService, ctx context.Context, userID uuid.UUID, label string) (string, APIToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return svc.CreateToken(ctx, userID, label)
}

// TestStress_ConcurrentTokenCreation creates tokens from 100 concurrent
// goroutines for 10 distinct users and verifies the max-5-active invariant
// holds under contention. Fails if total duration exceeds 5 seconds.
func TestStress_ConcurrentTokenCreation(t *testing.T) {
	repo := newAtomicAPITokenRepo()
	userRepo := newMockUserRepo()

	cfg := DefaultAPITokenConfig() // MaxTokensPerUser = 5

	svc := NewAPITokenService(repo, userRepo, cfg)

	// Create 10 distinct users.
	const numUsers = 10
	users := make([]uuid.UUID, numUsers)
	for i := 0; i < numUsers; i++ {
		uid := uuid.New()
		users[i] = uid
		_ = userRepo.Create(context.Background(), &User{
			ID:    uid,
			Email: fmt.Sprintf("stress-%d@test.com", i),
			Name:  fmt.Sprintf("Stress User %d", i),
			Role:  "user",
		})
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	var created atomic.Int64
	var rejected atomic.Int64

	start := time.Now()

	for g := 0; g < goroutines; g++ {
		go func(idx int) {
			defer wg.Done()
			userID := users[idx%numUsers]
			label := fmt.Sprintf("token-g%d", idx)

			_, _, err := repo.lockedCreateToken(svc, context.Background(), userID, label)
			switch err {
			case nil:
				created.Add(1)
			case ErrTokenLimitReached:
				rejected.Add(1)
			default:
				t.Errorf("unexpected error for goroutine %d: %v", idx, err)
			}
		}(g)
	}

	wg.Wait()
	duration := time.Since(start)

	t.Logf("Created %d tokens, rejected %d (limit reached) in %v", created.Load(), rejected.Load(), duration)

	if duration > 5*time.Second {
		t.Errorf("Stress test too slow: %v (max 5s)", duration)
	}

	// Verify the max-5-active invariant for every user.
	for i, uid := range users {
		count, err := repo.CountActiveByUserID(context.Background(), uid)
		if err != nil {
			t.Fatalf("CountActiveByUserID for user %d: %v", i, err)
		}
		if count > cfg.MaxTokensPerUser {
			t.Errorf("user %d has %d active tokens, max allowed is %d", i, count, cfg.MaxTokensPerUser)
		}
	}

	// Each user gets exactly 10 goroutines (100/10). With max 5 tokens,
	// we expect exactly 5 created per user = 50 total, 50 rejected.
	expectedCreated := int64(numUsers * cfg.MaxTokensPerUser)
	if created.Load() != expectedCreated {
		t.Errorf("expected %d created tokens, got %d", expectedCreated, created.Load())
	}

	expectedRejected := int64(goroutines) - expectedCreated
	if rejected.Load() != expectedRejected {
		t.Errorf("expected %d rejected tokens, got %d", expectedRejected, rejected.Load())
	}
}

// countingRateLimitRepo is a mock RateLimitRepository that atomically
// increments a counter on each CheckAndIncrement call, simulating the
// sliding window counter behaviour.
type countingRateLimitRepo struct {
	mu    sync.Mutex
	count int
}

func (r *countingRateLimitRepo) CheckAndIncrement(_ context.Context, _ *uuid.UUID, _ *uuid.UUID, _ string, _ time.Duration) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count++
	return r.count, nil
}

func (r *countingRateLimitRepo) CheckIPAndIncrement(_ context.Context, _ string, _ time.Duration) (int, error) {
	// IP check always under limit for this stress test.
	return 1, nil
}

func (r *countingRateLimitRepo) PruneExpired(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

// TestStress_RateLimitSlidingWindow simulates 1000 sequential Check calls with
// a counting mock repo and verifies the correct allowed/denied counts match the
// configured limit. Fails if total duration exceeds 2 seconds.
func TestStress_RateLimitSlidingWindow(t *testing.T) {
	const totalRequests = 1000
	const userLimit = 100

	repo := &countingRateLimitRepo{}

	cfg := APIRateLimitConfig{
		UserLimit:    userLimit,
		PremiumLimit: 500,
		AdminLimit:   1000,
		IPLimit:      10000, // high IP limit so it doesn't interfere
		Window:       time.Minute,
	}

	rl := NewAPIRateLimiter(repo, cfg)

	tokenID := uuid.New()
	userID := uuid.New()
	ip := "10.0.0.1"

	var allowed, denied int

	start := time.Now()

	for i := 0; i < totalRequests; i++ {
		result, err := rl.Check(context.Background(), &tokenID, &userID, "user", ip)
		if err != nil {
			t.Fatalf("Check call %d returned unexpected error: %v", i, err)
		}
		if result.Allowed {
			allowed++
		} else {
			denied++
		}
	}

	duration := time.Since(start)

	t.Logf("Allowed %d, denied %d out of %d requests in %v", allowed, denied, totalRequests, duration)

	if duration > 2*time.Second {
		t.Errorf("Stress test too slow: %v (max 2s)", duration)
	}

	// The counting repo increments on every call. Calls 1..100 return
	// counts 1..100 which are <= userLimit, so they are allowed.
	// Calls 101..1000 return counts 101..1000 which exceed userLimit.
	if allowed != userLimit {
		t.Errorf("expected %d allowed requests, got %d", userLimit, allowed)
	}

	expectedDenied := totalRequests - userLimit
	if denied != expectedDenied {
		t.Errorf("expected %d denied requests, got %d", expectedDenied, denied)
	}
}
