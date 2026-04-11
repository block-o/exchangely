package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"pgregory.net/rapid"
)

// TestPropertyRateLimitTierResolution verifies that for any role string in
// {"user", "premium", "admin"}, LimitForRole(role) returns the configured tier
// limit for that role. For any unknown role string, it returns the user tier
// limit as a safe default.
func TestPropertyRateLimitTierResolution(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw random but valid tier limits so the property holds for any config.
		userLimit := rapid.IntRange(1, 10000).Draw(t, "userLimit")
		premiumLimit := rapid.IntRange(1, 10000).Draw(t, "premiumLimit")
		adminLimit := rapid.IntRange(1, 10000).Draw(t, "adminLimit")
		ipLimit := rapid.IntRange(1, 10000).Draw(t, "ipLimit")

		cfg := APIRateLimitConfig{
			UserLimit:    userLimit,
			PremiumLimit: premiumLimit,
			AdminLimit:   adminLimit,
			IPLimit:      ipLimit,
			Window:       time.Minute,
		}

		rl := NewAPIRateLimiter(nil, cfg) // repo not needed for LimitForRole

		// Choose between a known role and an arbitrary unknown role string.
		scenario := rapid.SampledFrom([]string{
			"known_role",
			"unknown_role",
		}).Draw(t, "scenario")

		switch scenario {
		case "known_role":
			role := rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "role")
			got := rl.LimitForRole(role)

			var expected int
			switch role {
			case "user":
				expected = userLimit
			case "premium":
				expected = premiumLimit
			case "admin":
				expected = adminLimit
			}

			if got != expected {
				t.Fatalf("LimitForRole(%q) = %d, want %d", role, got, expected)
			}

		case "unknown_role":
			// Generate an arbitrary string that is NOT one of the known roles.
			role := rapid.StringMatching(`[a-zA-Z0-9_-]{0,50}`).Draw(t, "unknownRole")
			if role == "user" || role == "premium" || role == "admin" {
				// If we happen to draw a known role, skip this iteration.
				return
			}

			got := rl.LimitForRole(role)
			if got != userLimit {
				t.Fatalf("LimitForRole(%q) = %d, want user default %d", role, got, userLimit)
			}
		}
	})
}

// mockRateLimitRepo is a configurable mock of RateLimitRepository that returns
// preset counts for CheckAndIncrement and CheckIPAndIncrement.
type mockRateLimitRepo struct {
	tokenCount int
	ipCount    int
}

func (m *mockRateLimitRepo) CheckAndIncrement(_ context.Context, _ *uuid.UUID, _ *uuid.UUID, _ string, _ time.Duration) (int, error) {
	return m.tokenCount, nil
}

func (m *mockRateLimitRepo) CheckIPAndIncrement(_ context.Context, _ string, _ time.Duration) (int, error) {
	return m.ipCount, nil
}

func (m *mockRateLimitRepo) PruneExpired(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

// TestPropertyRateLimitEnforcement verifies that for any request count and
// configured limit, if the count within the sliding window exceeds the limit,
// the RateLimitResult has Allowed: false, Remaining: 0, and a ResetAt time in
// the future. If the count is under the limit, Allowed is true and Remaining
// equals limit - count.
func TestPropertyRateLimitEnforcement(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random tier limits.
		userLimit := rapid.IntRange(1, 1000).Draw(t, "userLimit")
		premiumLimit := rapid.IntRange(1, 1000).Draw(t, "premiumLimit")
		adminLimit := rapid.IntRange(1, 1000).Draw(t, "adminLimit")
		ipLimit := rapid.IntRange(1, 1000).Draw(t, "ipLimit")

		// Generate random counts returned by the mock repo.
		tokenCount := rapid.IntRange(0, 2000).Draw(t, "tokenCount")
		ipCount := rapid.IntRange(0, 2000).Draw(t, "ipCount")

		// Pick a role to test.
		role := rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "role")

		cfg := APIRateLimitConfig{
			UserLimit:    userLimit,
			PremiumLimit: premiumLimit,
			AdminLimit:   adminLimit,
			IPLimit:      ipLimit,
			Window:       time.Minute,
		}

		repo := &mockRateLimitRepo{
			tokenCount: tokenCount,
			ipCount:    ipCount,
		}

		rl := NewAPIRateLimiter(repo, cfg)

		tokenID := uuid.New()
		userID := uuid.New()
		ip := "192.168.1.1"

		before := time.Now()
		result, err := rl.Check(context.Background(), &tokenID, &userID, role, ip)
		after := time.Now()

		if err != nil {
			t.Fatalf("Check returned unexpected error: %v", err)
		}

		// Determine the expected limit for the chosen role.
		limit := rl.LimitForRole(role)

		// Verify Limit field matches the role's tier.
		if result.Limit != limit {
			t.Fatalf("result.Limit = %d, want %d (role=%q)", result.Limit, limit, role)
		}

		// Verify Allowed: true iff both tokenCount <= limit AND ipCount <= ipLimit.
		expectedAllowed := tokenCount <= limit && ipCount <= ipLimit
		if result.Allowed != expectedAllowed {
			t.Fatalf("result.Allowed = %v, want %v (tokenCount=%d, limit=%d, ipCount=%d, ipLimit=%d)",
				result.Allowed, expectedAllowed, tokenCount, limit, ipCount, ipLimit)
		}

		// Verify Remaining: min(limit - tokenCount, ipLimit - ipCount), clamped to 0.
		tokenRemaining := limit - tokenCount
		if tokenRemaining < 0 {
			tokenRemaining = 0
		}
		ipRemaining := ipLimit - ipCount
		if ipRemaining < 0 {
			ipRemaining = 0
		}
		expectedRemaining := tokenRemaining
		if ipRemaining < expectedRemaining {
			expectedRemaining = ipRemaining
		}
		if result.Remaining != expectedRemaining {
			t.Fatalf("result.Remaining = %d, want %d (tokenCount=%d, limit=%d, ipCount=%d, ipLimit=%d)",
				result.Remaining, expectedRemaining, tokenCount, limit, ipCount, ipLimit)
		}

		// Verify ResetAt is in the future (within the window).
		expectedResetMin := before.Add(cfg.Window)
		expectedResetMax := after.Add(cfg.Window)
		if result.ResetAt.Before(expectedResetMin) || result.ResetAt.After(expectedResetMax) {
			t.Fatalf("result.ResetAt = %v, want between %v and %v",
				result.ResetAt, expectedResetMin, expectedResetMax)
		}
	})
}

// errorRateLimitRepo is a mock RateLimitRepository that returns configurable
// errors from CheckAndIncrement and CheckIPAndIncrement.
type errorRateLimitRepo struct {
	checkAndIncrementErr   error
	checkIPAndIncrementErr error
}

func (m *errorRateLimitRepo) CheckAndIncrement(_ context.Context, _ *uuid.UUID, _ *uuid.UUID, _ string, _ time.Duration) (int, error) {
	return 0, m.checkAndIncrementErr
}

func (m *errorRateLimitRepo) CheckIPAndIncrement(_ context.Context, _ string, _ time.Duration) (int, error) {
	return 0, m.checkIPAndIncrementErr
}

func (m *errorRateLimitRepo) PruneExpired(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

// TestFailOpenOnDBError verifies that if PostgreSQL is unreachable during a
// rate limit check, the request is allowed (fail-open).
func TestFailOpenOnDBError(t *testing.T) {
	cfg := DefaultAPIRateLimitConfig()
	tokenID := uuid.New()
	userID := uuid.New()
	ip := "10.0.0.1"

	t.Run("CheckAndIncrement fails", func(t *testing.T) {
		repo := &errorRateLimitRepo{
			checkAndIncrementErr:   errSimulatedDB,
			checkIPAndIncrementErr: nil,
		}
		rl := NewAPIRateLimiter(repo, cfg)

		result, err := rl.Check(context.Background(), &tokenID, &userID, "user", ip)
		if err != nil {
			t.Fatalf("Check returned unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Fatal("expected Allowed=true on CheckAndIncrement DB error (fail-open)")
		}
		if result.Limit != cfg.UserLimit {
			t.Fatalf("result.Limit = %d, want %d", result.Limit, cfg.UserLimit)
		}
		if result.Remaining != cfg.UserLimit {
			t.Fatalf("result.Remaining = %d, want %d (full quota on fail-open)", result.Remaining, cfg.UserLimit)
		}
	})

	t.Run("CheckIPAndIncrement fails", func(t *testing.T) {
		repo := &errorRateLimitRepo{
			checkAndIncrementErr:   nil,
			checkIPAndIncrementErr: errSimulatedDB,
		}
		rl := NewAPIRateLimiter(repo, cfg)

		result, err := rl.Check(context.Background(), &tokenID, &userID, "user", ip)
		if err != nil {
			t.Fatalf("Check returned unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Fatal("expected Allowed=true on CheckIPAndIncrement DB error (fail-open)")
		}
		if result.Limit != cfg.UserLimit {
			t.Fatalf("result.Limit = %d, want %d", result.Limit, cfg.UserLimit)
		}
		if result.Remaining != cfg.UserLimit {
			t.Fatalf("result.Remaining = %d, want %d (full quota on fail-open)", result.Remaining, cfg.UserLimit)
		}
	})
}

// errSimulatedDB is a sentinel error used to simulate a database failure.
var errSimulatedDB = context.DeadlineExceeded

// TestPremiumRoleRateLimitTier verifies that premium role users receive premium
// tier rate limits and that role changes apply new tier limits on next request.
//
// **Validates: Requirements 10.1, 10.2, 10.3**
func TestPremiumRoleRateLimitTier(t *testing.T) {
	cfg := APIRateLimitConfig{
		UserLimit:    100,
		PremiumLimit: 500,
		AdminLimit:   1000,
		IPLimit:      200,
		Window:       time.Minute,
	}

	rl := NewAPIRateLimiter(nil, cfg)

	// Test that premium role gets premium tier limit.
	premiumLimit := rl.LimitForRole("premium")
	if premiumLimit != cfg.PremiumLimit {
		t.Errorf("LimitForRole(premium) = %d, want %d", premiumLimit, cfg.PremiumLimit)
	}

	// Test that user role gets user tier limit.
	userLimit := rl.LimitForRole("user")
	if userLimit != cfg.UserLimit {
		t.Errorf("LimitForRole(user) = %d, want %d", userLimit, cfg.UserLimit)
	}

	// Test that admin role gets admin tier limit.
	adminLimit := rl.LimitForRole("admin")
	if adminLimit != cfg.AdminLimit {
		t.Errorf("LimitForRole(admin) = %d, want %d", adminLimit, cfg.AdminLimit)
	}

	// Verify premium limit is between user and admin limits (as configured).
	if premiumLimit <= userLimit {
		t.Errorf("Premium limit (%d) should be greater than user limit (%d)", premiumLimit, userLimit)
	}
	if premiumLimit >= adminLimit {
		t.Errorf("Premium limit (%d) should be less than admin limit (%d)", premiumLimit, adminLimit)
	}
}

// TestRoleChangeAppliesNewTierLimit verifies that when a user's role changes,
// the new tier limit is applied on the next request without requiring a server restart.
//
// **Validates: Requirements 10.3**
func TestRoleChangeAppliesNewTierLimit(t *testing.T) {
	cfg := APIRateLimitConfig{
		UserLimit:    100,
		PremiumLimit: 500,
		AdminLimit:   1000,
		IPLimit:      200,
		Window:       time.Minute,
	}

	// Create a mock repo that returns a count below all tier limits.
	repo := &mockRateLimitRepo{
		tokenCount: 50,
		ipCount:    50,
	}

	rl := NewAPIRateLimiter(repo, cfg)

	tokenID := uuid.New()
	userID := uuid.New()
	ip := "192.168.1.1"

	// First request as "user" role.
	result1, err := rl.Check(context.Background(), &tokenID, &userID, "user", ip)
	if err != nil {
		t.Fatalf("Check returned unexpected error: %v", err)
	}
	if result1.Limit != cfg.UserLimit {
		t.Errorf("First request: Limit = %d, want %d (user tier)", result1.Limit, cfg.UserLimit)
	}
	if !result1.Allowed {
		t.Error("First request: expected Allowed=true")
	}

	// Second request as "premium" role (simulating role change).
	result2, err := rl.Check(context.Background(), &tokenID, &userID, "premium", ip)
	if err != nil {
		t.Fatalf("Check returned unexpected error: %v", err)
	}
	if result2.Limit != cfg.PremiumLimit {
		t.Errorf("Second request: Limit = %d, want %d (premium tier)", result2.Limit, cfg.PremiumLimit)
	}
	if !result2.Allowed {
		t.Error("Second request: expected Allowed=true")
	}

	// Third request as "admin" role (simulating another role change).
	result3, err := rl.Check(context.Background(), &tokenID, &userID, "admin", ip)
	if err != nil {
		t.Fatalf("Check returned unexpected error: %v", err)
	}
	if result3.Limit != cfg.AdminLimit {
		t.Errorf("Third request: Limit = %d, want %d (admin tier)", result3.Limit, cfg.AdminLimit)
	}
	if !result3.Allowed {
		t.Error("Third request: expected Allowed=true")
	}

	// Verify that the tier limit changes immediately without server restart.
	// This is guaranteed by the fact that LimitForRole is called on every request
	// and reads from the tiers map, which is immutable after initialization.
}
