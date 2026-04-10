package auth

import (
	"sync"
	"time"
)

// ipBanTier defines a progressive lockout tier for IP-based rate limiting.
type ipBanTier struct {
	threshold int           // cumulative failures to trigger this tier
	duration  time.Duration // how long the IP is blocked
}

var defaultIPBanTiers = []ipBanTier{
	{threshold: 20, duration: 15 * time.Minute},
	{threshold: 40, duration: 30 * time.Minute},
	{threshold: 60, duration: 1 * time.Hour},
}

// ipBanState tracks when an IP was banned and at which tier.
type ipBanState struct {
	bannedAt time.Time
	tier     int // index into the ban tiers
}

// RateLimiter is an in-memory sliding window rate limiter for login attempts.
// It tracks failed login timestamps per email and per IP, and blocks further
// attempts once the threshold is reached within the configured window.
type RateLimiter struct {
	mu          sync.Mutex
	attempts    map[string][]time.Time // per-email attempts
	ipAttempts  map[string][]time.Time // per-IP attempts
	ipBans      map[string]*ipBanState // active IP bans
	maxAttempts int
	window      time.Duration
	ipBanTiers  []ipBanTier
}

// NewRateLimiter creates a RateLimiter that allows maxAttempts failed login
// attempts per email within the given window duration. It also initialises
// IP-based progressive lockout with default tiers.
func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts:    make(map[string][]time.Time),
		ipAttempts:  make(map[string][]time.Time),
		ipBans:      make(map[string]*ipBanState),
		maxAttempts: maxAttempts,
		window:      window,
		ipBanTiers:  defaultIPBanTiers,
	}
}

// Allow reports whether a login attempt is permitted for the given email.
// It prunes expired entries before checking the count.
func (rl *RateLimiter) Allow(email string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.pruneExpired(email)
	return len(rl.attempts[email]) < rl.maxAttempts
}

// Record records a failed login attempt for the given email.
// It prunes expired entries before appending the new timestamp.
func (rl *RateLimiter) Record(email string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.pruneExpired(email)
	rl.attempts[email] = append(rl.attempts[email], time.Now())
}

// Reset clears all failed attempts for the given email.
// Called on successful login.
func (rl *RateLimiter) Reset(email string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.attempts, email)
}

// pruneExpired removes timestamps older than the sliding window for the given
// email. Must be called with rl.mu held.
func (rl *RateLimiter) pruneExpired(email string) {
	entries, ok := rl.attempts[email]
	if !ok {
		return
	}

	cutoff := time.Now().Add(-rl.window)
	// Find the first entry that is still within the window.
	i := 0
	for i < len(entries) && entries[i].Before(cutoff) {
		i++
	}

	if i == len(entries) {
		delete(rl.attempts, email)
	} else if i > 0 {
		rl.attempts[email] = entries[i:]
	}
}

// AllowIP reports whether a login attempt is permitted for the given IP address.
// It checks active bans first, then prunes expired attempts and evaluates
// progressive lockout tiers.
func (rl *RateLimiter) AllowIP(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Check if the IP is in an active ban period.
	if ban, ok := rl.ipBans[ip]; ok {
		if ban.tier < len(rl.ipBanTiers) {
			banEnd := ban.bannedAt.Add(rl.ipBanTiers[ban.tier].duration)
			if time.Now().Before(banEnd) {
				return false
			}
		}
		// Ban expired — remove it.
		delete(rl.ipBans, ip)
	}

	rl.pruneIPExpired(ip)

	count := len(rl.ipAttempts[ip])
	// Check tiers from highest to lowest to find the most severe applicable tier.
	for i := len(rl.ipBanTiers) - 1; i >= 0; i-- {
		if count >= rl.ipBanTiers[i].threshold {
			rl.ipBans[ip] = &ipBanState{
				bannedAt: time.Now(),
				tier:     i,
			}
			return false
		}
	}

	return true
}

// RecordIP records a failed login attempt for the given IP address.
func (rl *RateLimiter) RecordIP(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.pruneIPExpired(ip)
	rl.ipAttempts[ip] = append(rl.ipAttempts[ip], time.Now())
}

// ResetIP clears all failed attempts and any active ban for the given IP address.
// Called on successful login.
func (rl *RateLimiter) ResetIP(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.ipAttempts, ip)
	delete(rl.ipBans, ip)
}

// pruneIPExpired removes timestamps older than the sliding window for the given
// IP address. Must be called with rl.mu held.
func (rl *RateLimiter) pruneIPExpired(ip string) {
	entries, ok := rl.ipAttempts[ip]
	if !ok {
		return
	}

	cutoff := time.Now().Add(-rl.window)
	i := 0
	for i < len(entries) && entries[i].Before(cutoff) {
		i++
	}

	if i == len(entries) {
		delete(rl.ipAttempts, ip)
	} else if i > 0 {
		rl.ipAttempts[ip] = entries[i:]
	}
}
