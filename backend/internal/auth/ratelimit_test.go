package auth

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// TestPropertyRateLimitingEnforcesThreshold verifies Property 13: Rate limiting enforces threshold.
//
// For any email address, after exactly 5 failed login attempts within a
// 15-minute window, the rate limiter SHALL block subsequent attempts. Fewer
// than 5 failed attempts SHALL be allowed. A successful login SHALL reset
// the counter.
func TestPropertyRateLimitingEnforcesThreshold(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		const maxAttempts = 5
		const window = 15 * time.Minute

		rl := NewRateLimiter(maxAttempts, window)

		// Generate a random email address.
		email := rapid.StringMatching(`[a-z]{3,12}@[a-z]{3,8}\.[a-z]{2,4}`).Draw(t, "email")

		// Generate a random number of failed attempts to record (0–10).
		attempts := rapid.IntRange(0, 10).Draw(t, "attempts")

		for i := 0; i < attempts; i++ {
			if i < maxAttempts {
				// Attempts 1 through 5 must be allowed.
				if !rl.Allow(email) {
					t.Fatalf("Allow(%q) returned false after %d failures, expected true (threshold=%d)", email, i, maxAttempts)
				}
			} else {
				// Attempt 6+ must be blocked.
				if rl.Allow(email) {
					t.Fatalf("Allow(%q) returned true after %d failures, expected false (threshold=%d)", email, i, maxAttempts)
				}
			}
			rl.Record(email)
		}

		// Final check: after all recorded attempts, verify Allow reflects the count.
		if attempts < maxAttempts {
			if !rl.Allow(email) {
				t.Fatalf("Allow(%q) returned false after %d failures (< threshold %d)", email, attempts, maxAttempts)
			}
		} else {
			if rl.Allow(email) {
				t.Fatalf("Allow(%q) returned true after %d failures (>= threshold %d)", email, attempts, maxAttempts)
			}
		}
	})
}

// TestPropertyRateLimitingResetClearsCounter verifies that a successful login
// (Reset) clears the failure counter, allowing new attempts.
func TestPropertyRateLimitingResetClearsCounter(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		const maxAttempts = 5
		const window = 15 * time.Minute

		rl := NewRateLimiter(maxAttempts, window)

		email := rapid.StringMatching(`[a-z]{3,12}@[a-z]{3,8}\.[a-z]{2,4}`).Draw(t, "email")

		// Record some failures (1–10).
		failures := rapid.IntRange(1, 10).Draw(t, "failures")
		for i := 0; i < failures; i++ {
			rl.Record(email)
		}

		// Simulate a successful login — reset the counter.
		rl.Reset(email)

		// After reset, the next attempt must be allowed.
		if !rl.Allow(email) {
			t.Fatalf("Allow(%q) returned false after Reset, expected true (failures before reset: %d)", email, failures)
		}

		// Verify we can record up to maxAttempts again before being blocked.
		for i := 0; i < maxAttempts; i++ {
			if !rl.Allow(email) {
				t.Fatalf("Allow(%q) returned false after Reset + %d new failures, expected true", email, i)
			}
			rl.Record(email)
		}

		// Now at maxAttempts again — should be blocked.
		if rl.Allow(email) {
			t.Fatalf("Allow(%q) returned true after Reset + %d new failures, expected false", email, maxAttempts)
		}
	})
}

// TestPropertyRateLimitingIsolatesEmails verifies that rate limiting for one
// email does not affect a different email.
func TestPropertyRateLimitingIsolatesEmails(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		const maxAttempts = 5
		const window = 15 * time.Minute

		rl := NewRateLimiter(maxAttempts, window)

		// Generate two distinct emails.
		email1 := rapid.StringMatching(`[a-z]{3,12}@[a-z]{3,8}\.[a-z]{2,4}`).Draw(t, "email1")
		email2 := rapid.StringMatching(`[a-z]{3,12}@[a-z]{3,8}\.[a-z]{2,4}`).Draw(t, "email2")
		if email1 == email2 {
			// Extremely unlikely with random generation, but skip if it happens.
			return
		}

		// Exhaust the limit for email1.
		for i := 0; i < maxAttempts; i++ {
			rl.Record(email1)
		}

		// email1 should be blocked.
		if rl.Allow(email1) {
			t.Fatalf("Allow(%q) returned true after %d failures, expected false", email1, maxAttempts)
		}

		// email2 should still be allowed — no cross-contamination.
		if !rl.Allow(email2) {
			t.Fatalf("Allow(%q) returned false, but no failures were recorded for this email", email2)
		}
	})
}

// TestPropertyIPRateLimitingEnforcesThreshold verifies that the IP-based rate
// limiter blocks IPs after 20 failed attempts within the window.
func TestPropertyIPRateLimitingEnforcesThreshold(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		const maxEmailAttempts = 5
		const window = 15 * time.Minute
		const ipThreshold = 20

		rl := NewRateLimiter(maxEmailAttempts, window)

		ip := rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "ip")

		// Generate a random number of failed attempts (0–30).
		attempts := rapid.IntRange(0, 30).Draw(t, "attempts")

		for i := 0; i < attempts; i++ {
			if i < ipThreshold {
				if !rl.AllowIP(ip) {
					t.Fatalf("AllowIP(%q) returned false after %d failures, expected true (threshold=%d)", ip, i, ipThreshold)
				}
			} else {
				if rl.AllowIP(ip) {
					t.Fatalf("AllowIP(%q) returned true after %d failures, expected false (threshold=%d)", ip, i, ipThreshold)
				}
			}
			rl.RecordIP(ip)
		}

		// Final check.
		if attempts < ipThreshold {
			if !rl.AllowIP(ip) {
				t.Fatalf("AllowIP(%q) returned false after %d failures (< threshold %d)", ip, attempts, ipThreshold)
			}
		} else {
			if rl.AllowIP(ip) {
				t.Fatalf("AllowIP(%q) returned true after %d failures (>= threshold %d)", ip, attempts, ipThreshold)
			}
		}
	})
}

// TestPropertyIPRateLimitingProgressiveLockout verifies that progressive lockout
// tiers are applied correctly: 20 failures → tier 0 (15m), 40 → tier 1 (30m),
// 60 → tier 2 (1h).
func TestPropertyIPRateLimitingProgressiveLockout(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		const maxEmailAttempts = 5
		const window = 15 * time.Minute

		rl := NewRateLimiter(maxEmailAttempts, window)

		ip := rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "ip")

		// Record 20 failures → should trigger tier 0.
		for i := 0; i < 20; i++ {
			rl.RecordIP(ip)
		}
		if rl.AllowIP(ip) {
			t.Fatal("expected IP to be blocked after 20 failures (tier 0)")
		}

		// Verify the ban state is at tier 0.
		rl.mu.Lock()
		ban, ok := rl.ipBans[ip]
		if !ok {
			rl.mu.Unlock()
			t.Fatal("expected ban state to exist for IP")
		}
		if ban.tier != 0 {
			rl.mu.Unlock()
			t.Fatalf("expected tier 0, got tier %d", ban.tier)
		}

		// Simulate ban expiry by moving bannedAt back.
		ban.bannedAt = time.Now().Add(-16 * time.Minute)
		rl.mu.Unlock()

		// After tier 0 ban expires, AllowIP should clear the ban but the
		// attempt count is still >= 20, so it will re-ban at tier 0.
		// We need to record more to reach tier 1.
		// First, AllowIP should see the expired ban, remove it, then check counts.
		// With 20 attempts still in window, it will re-ban at tier 0.
		if rl.AllowIP(ip) {
			t.Fatal("expected IP to still be blocked with 20 attempts in window")
		}

		// Record 20 more failures (total 40) → should trigger tier 1.
		for i := 0; i < 20; i++ {
			rl.RecordIP(ip)
		}

		// Expire the current ban.
		rl.mu.Lock()
		if ban, ok := rl.ipBans[ip]; ok {
			ban.bannedAt = time.Now().Add(-16 * time.Minute)
		}
		rl.mu.Unlock()

		// AllowIP should now see 40 attempts and set tier 1.
		if rl.AllowIP(ip) {
			t.Fatal("expected IP to be blocked after 40 failures (tier 1)")
		}

		rl.mu.Lock()
		ban, ok = rl.ipBans[ip]
		if !ok {
			rl.mu.Unlock()
			t.Fatal("expected ban state after 40 failures")
		}
		if ban.tier != 1 {
			rl.mu.Unlock()
			t.Fatalf("expected tier 1, got tier %d", ban.tier)
		}
		// Expire tier 1 ban.
		ban.bannedAt = time.Now().Add(-31 * time.Minute)
		rl.mu.Unlock()

		// Record 20 more (total 60) → should trigger tier 2.
		for i := 0; i < 20; i++ {
			rl.RecordIP(ip)
		}

		// Expire the current ban so AllowIP re-evaluates.
		rl.mu.Lock()
		if ban, ok := rl.ipBans[ip]; ok {
			ban.bannedAt = time.Now().Add(-31 * time.Minute)
		}
		rl.mu.Unlock()

		if rl.AllowIP(ip) {
			t.Fatal("expected IP to be blocked after 60 failures (tier 2)")
		}

		rl.mu.Lock()
		ban, ok = rl.ipBans[ip]
		if !ok {
			rl.mu.Unlock()
			t.Fatal("expected ban state after 60 failures")
		}
		if ban.tier != 2 {
			rl.mu.Unlock()
			t.Fatalf("expected tier 2, got tier %d", ban.tier)
		}
		rl.mu.Unlock()
	})
}

// TestPropertyIPRateLimitingIsolatesIPs verifies that rate limiting for one IP
// does not affect a different IP.
func TestPropertyIPRateLimitingIsolatesIPs(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		const maxEmailAttempts = 5
		const window = 15 * time.Minute

		rl := NewRateLimiter(maxEmailAttempts, window)

		ip1 := rapid.StringMatching(`10\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "ip1")
		ip2 := rapid.StringMatching(`192\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "ip2")

		// Exhaust the limit for ip1.
		for i := 0; i < 20; i++ {
			rl.RecordIP(ip1)
		}

		// ip1 should be blocked.
		if rl.AllowIP(ip1) {
			t.Fatalf("AllowIP(%q) returned true after 20 failures, expected false", ip1)
		}

		// ip2 should still be allowed.
		if !rl.AllowIP(ip2) {
			t.Fatalf("AllowIP(%q) returned false, but no failures were recorded for this IP", ip2)
		}
	})
}

// TestPropertyIPRateLimitingResetClearsState verifies that ResetIP clears all
// failed attempts and any active ban for an IP.
func TestPropertyIPRateLimitingResetClearsState(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		const maxEmailAttempts = 5
		const window = 15 * time.Minute

		rl := NewRateLimiter(maxEmailAttempts, window)

		ip := rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "ip")

		// Record enough failures to trigger a ban.
		failures := rapid.IntRange(1, 30).Draw(t, "failures")
		for i := 0; i < failures; i++ {
			rl.RecordIP(ip)
		}

		// Reset the IP.
		rl.ResetIP(ip)

		// After reset, the IP must be allowed.
		if !rl.AllowIP(ip) {
			t.Fatalf("AllowIP(%q) returned false after ResetIP, expected true (failures before reset: %d)", ip, failures)
		}
	})
}
