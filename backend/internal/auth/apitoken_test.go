package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"pgregory.net/rapid"
)

// TestPropertyTokenStatusDerivation verifies that for any APIToken with
// arbitrary revoked_at and expires_at values, TokenStatus() returns "revoked"
// if revoked_at is non-nil, "expired" if expires_at is before now() and not
// revoked, and "active" otherwise. The revoked check takes priority over the
// expired check.
func TestPropertyTokenStatusDerivation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw a scenario to determine the combination of revoked_at and expires_at.
		scenario := rapid.SampledFrom([]string{
			"revoked_and_expired",
			"revoked_not_expired",
			"not_revoked_expired",
			"active",
		}).Draw(t, "scenario")

		now := time.Now()

		var revokedAt *time.Time
		var expiresAt time.Time

		switch scenario {
		case "revoked_and_expired":
			// Both revoked and expired — revoked takes priority.
			revokedTime := now.Add(-time.Duration(rapid.IntRange(1, 86400).Draw(t, "revokedSecsAgo")) * time.Second)
			revokedAt = &revokedTime
			expiresAt = now.Add(-time.Duration(rapid.IntRange(1, 86400).Draw(t, "expiredSecsAgo")) * time.Second)

		case "revoked_not_expired":
			// Revoked but not yet expired — should still be "revoked".
			revokedTime := now.Add(-time.Duration(rapid.IntRange(1, 86400).Draw(t, "revokedSecsAgo")) * time.Second)
			revokedAt = &revokedTime
			expiresAt = now.Add(time.Duration(rapid.IntRange(1, 86400).Draw(t, "expiresInSecs")) * time.Second)

		case "not_revoked_expired":
			// Not revoked but expired.
			revokedAt = nil
			expiresAt = now.Add(-time.Duration(rapid.IntRange(1, 86400).Draw(t, "expiredSecsAgo")) * time.Second)

		case "active":
			// Not revoked and not expired.
			revokedAt = nil
			expiresAt = now.Add(time.Duration(rapid.IntRange(1, 86400).Draw(t, "expiresInSecs")) * time.Second)
		}

		token := APIToken{
			ID:        uuid.New(),
			UserID:    uuid.New(),
			TokenHash: "fakehash",
			Label:     "test-token",
			Prefix:    "exly_abc",
			CreatedAt: now.Add(-24 * time.Hour),
			RevokedAt: revokedAt,
			ExpiresAt: expiresAt,
		}

		status := token.TokenStatus()

		switch scenario {
		case "revoked_and_expired", "revoked_not_expired":
			if status != "revoked" {
				t.Fatalf("scenario %q: expected status %q, got %q (revokedAt=%v, expiresAt=%v)",
					scenario, "revoked", status, revokedAt, expiresAt)
			}
		case "not_revoked_expired":
			if status != "expired" {
				t.Fatalf("scenario %q: expected status %q, got %q (revokedAt=%v, expiresAt=%v)",
					scenario, "expired", status, revokedAt, expiresAt)
			}
		case "active":
			if status != "active" {
				t.Fatalf("scenario %q: expected status %q, got %q (revokedAt=%v, expiresAt=%v)",
					scenario, "active", status, revokedAt, expiresAt)
			}
		}
	})
}

// TestPropertyTokenGenerationRoundTrip verifies that for any authenticated
// user and any valid label string, generating an API token produces a raw token
// that: (a) starts with the exly_ prefix, (b) has SHA-256(raw) equal to the
// stored token_hash, (c) has expires_at within 1 second of created_at + 90 days,
// and (d) has a raw token length of at least len("exly_") + 43 characters.
func TestPropertyTokenGenerationRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Set up mock repos and service with default config (90-day expiry).
		tokenRepo := newMockAPITokenRepo()
		userRepo := newMockUserRepo()
		cfg := DefaultAPITokenConfig()
		svc := NewAPITokenService(tokenRepo, userRepo, cfg)

		// Generate a random user and persist it so the service can find it.
		userID := uuid.New()
		label := rapid.StringMatching(`[a-zA-Z0-9 _-]{1,50}`).Draw(t, "label")

		// Create the token.
		rawToken, token, err := svc.CreateToken(t.Context(), userID, label)
		if err != nil {
			t.Fatalf("CreateToken failed: %v", err)
		}

		// (a) Raw token starts with "exly_" prefix.
		if !strings.HasPrefix(rawToken, "exly_") {
			t.Fatalf("raw token does not start with exly_ prefix: %q", rawToken)
		}

		// (b) SHA-256(raw) == stored token_hash.
		h := sha256.Sum256([]byte(rawToken))
		computedHash := hex.EncodeToString(h[:])
		if computedHash != token.TokenHash {
			t.Fatalf("SHA-256 mismatch: computed %q, stored %q", computedHash, token.TokenHash)
		}

		// Also verify the hash stored in the mock repo matches.
		stored, err := tokenRepo.FindByTokenHash(t.Context(), computedHash)
		if err != nil {
			t.Fatalf("FindByTokenHash failed: %v", err)
		}
		if stored == nil {
			t.Fatal("token not found in repo by computed hash")
		} else if stored.ID != token.ID {
			t.Fatalf("repo token ID mismatch: got %v, want %v", stored.ID, token.ID)
		}

		// (c) expires_at within 1 second of created_at + 90 days.
		expected90d := token.CreatedAt.Add(90 * 24 * time.Hour)
		diff := token.ExpiresAt.Sub(expected90d)
		if diff < 0 {
			diff = -diff
		}
		if diff > time.Second {
			t.Fatalf("expires_at drift too large: created_at=%v, expires_at=%v, expected=%v, diff=%v",
				token.CreatedAt, token.ExpiresAt, expected90d, diff)
		}

		// (d) Raw token length >= len("exly_") + 43.
		minLen := len("exly_") + 43
		if len(rawToken) < minLen {
			t.Fatalf("raw token too short: got %d, want >= %d", len(rawToken), minLen)
		}
	})
}

// TestPropertyMaxActiveTokensInvariant verifies that for any user and any
// sequence of create and revoke operations, the number of active (non-revoked,
// non-expired) tokens for that user never exceeds 5. If a user already has 5
// active tokens, a create operation returns an error.
func TestPropertyMaxActiveTokensInvariant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		tokenRepo := newMockAPITokenRepo()
		userRepo := newMockUserRepo()
		cfg := DefaultAPITokenConfig() // MaxTokensPerUser = 5
		svc := NewAPITokenService(tokenRepo, userRepo, cfg)

		userID := uuid.New()
		ctx := t.Context()

		// Track created token IDs so we can revoke them.
		var createdTokenIDs []uuid.UUID

		// Generate a random sequence of operations (between 1 and 30).
		numOps := rapid.IntRange(1, 30).Draw(t, "numOps")

		for i := 0; i < numOps; i++ {
			// Choose an operation: "create" or "revoke".
			// Only allow revoke if we have tokens to revoke.
			op := "create"
			if len(createdTokenIDs) > 0 {
				op = rapid.SampledFrom([]string{"create", "revoke"}).Draw(t, "op")
			}

			switch op {
			case "create":
				label := rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "label")
				_, token, err := svc.CreateToken(ctx, userID, label)

				// Count active tokens after the operation.
				activeCount, countErr := tokenRepo.CountActiveByUserID(ctx, userID)
				if countErr != nil {
					t.Fatalf("CountActiveByUserID failed: %v", countErr)
				}

				if err != nil {
					// If create failed, it must be because we hit the limit.
					if err != ErrTokenLimitReached {
						t.Fatalf("CreateToken returned unexpected error: %v", err)
					}
					// Active count should be at the max.
					if activeCount < cfg.MaxTokensPerUser {
						t.Fatalf("CreateToken returned ErrTokenLimitReached but active count is %d (max %d)",
							activeCount, cfg.MaxTokensPerUser)
					}
				} else {
					createdTokenIDs = append(createdTokenIDs, token.ID)
				}

				// Invariant: active count must never exceed MaxTokensPerUser.
				if activeCount > cfg.MaxTokensPerUser {
					t.Fatalf("active token count %d exceeds max %d", activeCount, cfg.MaxTokensPerUser)
				}

			case "revoke":
				// Pick a random token to revoke from the created list.
				idx := rapid.IntRange(0, len(createdTokenIDs)-1).Draw(t, "revokeIdx")
				tokenID := createdTokenIDs[idx]

				err := svc.RevokeToken(ctx, tokenID, userID)
				if err != nil {
					t.Fatalf("RevokeToken failed: %v", err)
				}

				// Verify active count after revocation.
				activeCount, countErr := tokenRepo.CountActiveByUserID(ctx, userID)
				if countErr != nil {
					t.Fatalf("CountActiveByUserID failed: %v", countErr)
				}
				if activeCount > cfg.MaxTokensPerUser {
					t.Fatalf("active token count %d exceeds max %d after revoke", activeCount, cfg.MaxTokensPerUser)
				}
			}
		}
	})
}

// TestPropertyValidTokenResolvesToCorrectUser verifies that for any valid
// (active, non-revoked, non-expired) API token, validating the raw token
// returns the user identity matching the token's user_id, with the correct
// role attached.
func TestPropertyValidTokenResolvesToCorrectUser(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		tokenRepo := newMockAPITokenRepo()
		userRepo := newMockUserRepo()
		cfg := DefaultAPITokenConfig()
		svc := NewAPITokenService(tokenRepo, userRepo, cfg)

		ctx := t.Context()

		// Generate a random user with a random role.
		role := rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "role")
		userID := uuid.New()
		user := &User{
			ID:    userID,
			Email: rapid.StringMatching(`[a-z]{3,10}@example\.com`).Draw(t, "email"),
			Name:  rapid.StringMatching(`[A-Za-z ]{2,20}`).Draw(t, "name"),
			Role:  role,
		}

		// Persist the user in the mock repo so ValidateToken can find it.
		if err := userRepo.Create(ctx, user); err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		// Create a token for this user.
		label := rapid.StringMatching(`[a-zA-Z0-9 _-]{1,30}`).Draw(t, "label")
		rawToken, createdToken, err := svc.CreateToken(ctx, userID, label)
		if err != nil {
			t.Fatalf("CreateToken failed: %v", err)
		}

		// Validate the raw token.
		returnedToken, returnedUser, err := svc.ValidateToken(ctx, rawToken)
		if err != nil {
			t.Fatalf("ValidateToken failed: %v", err)
		}

		// Verify the returned token matches the created token.
		if returnedToken.ID != createdToken.ID {
			t.Fatalf("token ID mismatch: got %v, want %v", returnedToken.ID, createdToken.ID)
		}
		if returnedToken.UserID != userID {
			t.Fatalf("token UserID mismatch: got %v, want %v", returnedToken.UserID, userID)
		}

		// Verify the returned user matches the original user identity and role.
		if returnedUser.ID != userID {
			t.Fatalf("user ID mismatch: got %v, want %v", returnedUser.ID, userID)
		}
		if returnedUser.Email != user.Email {
			t.Fatalf("user email mismatch: got %q, want %q", returnedUser.Email, user.Email)
		}
		if returnedUser.Role != role {
			t.Fatalf("user role mismatch: got %q, want %q", returnedUser.Role, role)
		}
		if returnedUser.Name != user.Name {
			t.Fatalf("user name mismatch: got %q, want %q", returnedUser.Name, user.Name)
		}
	})
}

// TestPropertyInvalidTokensAreRejected verifies that for any API token that is
// either (a) not present in the database, (b) revoked, or (c) expired,
// validation returns an unauthorized error.
func TestPropertyInvalidTokensAreRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		tokenRepo := newMockAPITokenRepo()
		userRepo := newMockUserRepo()
		cfg := DefaultAPITokenConfig()
		svc := NewAPITokenService(tokenRepo, userRepo, cfg)

		ctx := t.Context()

		// Create a user so that token creation works.
		userID := uuid.New()
		user := &User{
			ID:    userID,
			Email: rapid.StringMatching(`[a-z]{3,10}@example\.com`).Draw(t, "email"),
			Name:  rapid.StringMatching(`[A-Za-z ]{2,20}`).Draw(t, "name"),
			Role:  rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "role"),
		}
		if err := userRepo.Create(ctx, user); err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		scenario := rapid.SampledFrom([]string{
			"missing_token",
			"revoked_token",
			"expired_token",
		}).Draw(t, "scenario")

		switch scenario {
		case "missing_token":
			// (a) Generate a completely random token string that doesn't exist in the repo.
			randomSuffix := rapid.StringMatching(`[a-zA-Z0-9]{32,64}`).Draw(t, "randomSuffix")
			fakeRawToken := "exly_" + randomSuffix

			_, _, err := svc.ValidateToken(ctx, fakeRawToken)
			if err != ErrTokenUnauthorized {
				t.Fatalf("missing token: expected ErrTokenUnauthorized, got %v", err)
			}

		case "revoked_token":
			// (b) Create a valid token, then revoke it, then try to validate.
			label := rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "label")
			rawToken, createdToken, err := svc.CreateToken(ctx, userID, label)
			if err != nil {
				t.Fatalf("CreateToken failed: %v", err)
			}

			// Revoke the token.
			if err := svc.RevokeToken(ctx, createdToken.ID, userID); err != nil {
				t.Fatalf("RevokeToken failed: %v", err)
			}

			// Validation must fail with unauthorized.
			_, _, err = svc.ValidateToken(ctx, rawToken)
			if err != ErrTokenUnauthorized {
				t.Fatalf("revoked token: expected ErrTokenUnauthorized, got %v", err)
			}

		case "expired_token":
			// (c) Create a valid token, then manipulate the mock repo to set expires_at in the past.
			label := rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "label")
			rawToken, createdToken, err := svc.CreateToken(ctx, userID, label)
			if err != nil {
				t.Fatalf("CreateToken failed: %v", err)
			}

			// Directly manipulate the mock repo to expire the token.
			expiredTime := time.Now().Add(-time.Duration(rapid.IntRange(1, 86400).Draw(t, "expiredSecsAgo")) * time.Second)
			tokenRepo.mu.Lock()
			if tok, ok := tokenRepo.byID[createdToken.ID]; ok {
				tok.ExpiresAt = expiredTime
			}
			if tok, ok := tokenRepo.byHash[createdToken.TokenHash]; ok {
				tok.ExpiresAt = expiredTime
			}
			tokenRepo.mu.Unlock()

			// Validation must fail with unauthorized.
			_, _, err = svc.ValidateToken(ctx, rawToken)
			if err != ErrTokenUnauthorized {
				t.Fatalf("expired token: expected ErrTokenUnauthorized, got %v", err)
			}
		}
	})
}

// TestPropertyCrossUserTokenIsolation verifies that for any two distinct users
// A and B, if user A owns a token, then user B attempting to revoke that token
// by ID receives a not-found error, and the token remains unchanged.
func TestPropertyCrossUserTokenIsolation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		tokenRepo := newMockAPITokenRepo()
		userRepo := newMockUserRepo()
		cfg := DefaultAPITokenConfig()
		svc := NewAPITokenService(tokenRepo, userRepo, cfg)

		ctx := t.Context()

		// Generate two distinct users.
		userAID := uuid.New()
		userBID := uuid.New()
		// Ensure they are distinct (astronomically unlikely to collide, but be explicit).
		for userBID == userAID {
			userBID = uuid.New()
		}

		// Create one or more tokens for user A.
		numTokens := rapid.IntRange(1, 5).Draw(t, "numTokens")
		type tokenRecord struct {
			id    uuid.UUID
			raw   string
			token APIToken
		}
		var tokensA []tokenRecord

		for i := 0; i < numTokens; i++ {
			label := rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "label")
			rawToken, tok, err := svc.CreateToken(ctx, userAID, label)
			if err != nil {
				t.Fatalf("CreateToken for user A failed: %v", err)
			}
			tokensA = append(tokensA, tokenRecord{id: tok.ID, raw: rawToken, token: tok})
		}

		// Pick a random token from user A for user B to attempt to revoke.
		idx := rapid.IntRange(0, len(tokensA)-1).Draw(t, "targetIdx")
		targetToken := tokensA[idx]

		// Snapshot the token state before the revocation attempt.
		beforeToken, err := tokenRepo.FindByTokenHash(ctx, targetToken.token.TokenHash)
		if err != nil {
			t.Fatalf("FindByTokenHash failed: %v", err)
		}
		if beforeToken == nil {
			t.Fatal("target token not found in repo before revocation attempt")
			return // unreachable; helps staticcheck see nil is excluded below
		}

		// User B attempts to revoke user A's token — should get ErrTokenNotFound.
		err = svc.RevokeToken(ctx, targetToken.id, userBID)
		if err != ErrTokenNotFound {
			t.Fatalf("expected ErrTokenNotFound when user B revokes user A's token, got: %v", err)
		}

		// Verify the token is unchanged after the failed revocation attempt.
		afterToken, err := tokenRepo.FindByTokenHash(ctx, targetToken.token.TokenHash)
		if err != nil {
			t.Fatalf("FindByTokenHash failed after revocation attempt: %v", err)
		}
		if afterToken == nil {
			t.Fatal("target token disappeared from repo after revocation attempt")
			return // unreachable; helps staticcheck see nil is excluded below
		}

		// RevokedAt should be unchanged.
		if beforeToken.RevokedAt == nil && afterToken.RevokedAt != nil {
			t.Fatalf("token was unexpectedly revoked: revokedAt went from nil to %v", afterToken.RevokedAt)
		}
		if beforeToken.RevokedAt != nil && afterToken.RevokedAt == nil {
			t.Fatal("token revokedAt was unexpectedly cleared")
		}
		if beforeToken.RevokedAt != nil && afterToken.RevokedAt != nil && !beforeToken.RevokedAt.Equal(*afterToken.RevokedAt) {
			t.Fatalf("token revokedAt changed: before=%v, after=%v", beforeToken.RevokedAt, afterToken.RevokedAt)
		}

		// All other fields should remain the same.
		if afterToken.ID != beforeToken.ID {
			t.Fatalf("token ID changed: before=%v, after=%v", beforeToken.ID, afterToken.ID)
		}
		if afterToken.UserID != beforeToken.UserID {
			t.Fatalf("token UserID changed: before=%v, after=%v", beforeToken.UserID, afterToken.UserID)
		}
		if afterToken.Label != beforeToken.Label {
			t.Fatalf("token Label changed: before=%q, after=%q", beforeToken.Label, afterToken.Label)
		}
		if afterToken.TokenHash != beforeToken.TokenHash {
			t.Fatalf("token TokenHash changed")
		}
		if !afterToken.ExpiresAt.Equal(beforeToken.ExpiresAt) {
			t.Fatalf("token ExpiresAt changed: before=%v, after=%v", beforeToken.ExpiresAt, afterToken.ExpiresAt)
		}
	})
}

// TestPropertyRevocationIdempotence verifies that for any API token, revoking
// it twice is idempotent: the first revocation sets revoked_at, and the second
// revocation succeeds without error and leaves the token state unchanged.
func TestPropertyRevocationIdempotence(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		tokenRepo := newMockAPITokenRepo()
		userRepo := newMockUserRepo()
		cfg := DefaultAPITokenConfig()
		svc := NewAPITokenService(tokenRepo, userRepo, cfg)

		ctx := t.Context()

		// Generate a random user.
		userID := uuid.New()

		// Create a token with a random label.
		label := rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "label")
		_, createdToken, err := svc.CreateToken(ctx, userID, label)
		if err != nil {
			t.Fatalf("CreateToken failed: %v", err)
		}

		// First revocation — should succeed.
		err = svc.RevokeToken(ctx, createdToken.ID, userID)
		if err != nil {
			t.Fatalf("first RevokeToken failed: %v", err)
		}

		// Snapshot the token state after the first revocation.
		afterFirst, err := tokenRepo.FindByTokenHash(ctx, createdToken.TokenHash)
		if err != nil {
			t.Fatalf("FindByTokenHash after first revoke failed: %v", err)
		}
		if afterFirst == nil {
			t.Fatal("token not found after first revocation")
			return // unreachable; helps staticcheck see nil is excluded below
		}
		if afterFirst.RevokedAt == nil {
			t.Fatal("RevokedAt should be set after first revocation")
			return // unreachable; helps staticcheck see nil is excluded below
		}
		firstRevokedAt := *afterFirst.RevokedAt

		// Second revocation — should succeed without error (idempotent).
		err = svc.RevokeToken(ctx, createdToken.ID, userID)
		if err != nil {
			t.Fatalf("second RevokeToken should succeed (idempotent), got error: %v", err)
		}

		// Verify the token state is unchanged after the second revocation.
		afterSecond, err := tokenRepo.FindByTokenHash(ctx, createdToken.TokenHash)
		if err != nil {
			t.Fatalf("FindByTokenHash after second revoke failed: %v", err)
		}
		if afterSecond == nil {
			t.Fatal("token not found after second revocation")
			return // unreachable; helps staticcheck see nil is excluded below
		}
		if afterSecond.RevokedAt == nil {
			t.Fatal("RevokedAt should still be set after second revocation")
			return // unreachable; helps staticcheck see nil is excluded below
		}

		// RevokedAt must be identical — the second revoke must not change it.
		if !firstRevokedAt.Equal(*afterSecond.RevokedAt) {
			t.Fatalf("RevokedAt changed between revocations: first=%v, second=%v",
				firstRevokedAt, *afterSecond.RevokedAt)
		}

		// All other fields should remain unchanged.
		if afterSecond.ID != afterFirst.ID {
			t.Fatalf("token ID changed: first=%v, second=%v", afterFirst.ID, afterSecond.ID)
		}
		if afterSecond.UserID != afterFirst.UserID {
			t.Fatalf("token UserID changed: first=%v, second=%v", afterFirst.UserID, afterSecond.UserID)
		}
		if afterSecond.Label != afterFirst.Label {
			t.Fatalf("token Label changed: first=%q, second=%q", afterFirst.Label, afterSecond.Label)
		}
		if afterSecond.TokenHash != afterFirst.TokenHash {
			t.Fatalf("token TokenHash changed")
		}
		if !afterSecond.ExpiresAt.Equal(afterFirst.ExpiresAt) {
			t.Fatalf("token ExpiresAt changed: first=%v, second=%v", afterFirst.ExpiresAt, afterSecond.ExpiresAt)
		}
	})
}

// TestPropertyTokenListCompletenessAndOrdering verifies that for any user with
// N tokens (active, revoked, or expired), listing their tokens returns exactly
// N results, each with a prefix matching the first 8 characters of the original
// raw token, and the list is ordered by created_at descending.
func TestPropertyTokenListCompletenessAndOrdering(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		tokenRepo := newMockAPITokenRepo()
		userRepo := newMockUserRepo()
		cfg := DefaultAPITokenConfig() // MaxTokensPerUser = 5
		svc := NewAPITokenService(tokenRepo, userRepo, cfg)

		ctx := t.Context()
		userID := uuid.New()

		// Draw total number of tokens to create (1–5).
		totalTokens := rapid.IntRange(1, 5).Draw(t, "totalTokens")

		// For each token, decide its final state: "active", "revoked", or "expired".
		// We must ensure we never exceed 5 active tokens at any point during creation.
		// Strategy: create all tokens first (they start active), then revoke/expire
		// some of them. Since totalTokens <= 5, we won't hit the limit during creation.
		type tokenInfo struct {
			raw   string
			token APIToken
			state string
		}
		tokens := make([]tokenInfo, 0, totalTokens)

		for i := 0; i < totalTokens; i++ {
			label := rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "label")
			rawToken, tok, err := svc.CreateToken(ctx, userID, label)
			if err != nil {
				t.Fatalf("CreateToken %d failed: %v", i, err)
			}
			tokens = append(tokens, tokenInfo{raw: rawToken, token: tok, state: "active"})
		}

		// Now decide which tokens to revoke or expire.
		for i := range tokens {
			state := rapid.SampledFrom([]string{"active", "revoked", "expired"}).Draw(t, "state")
			tokens[i].state = state

			switch state {
			case "revoked":
				if err := svc.RevokeToken(ctx, tokens[i].token.ID, userID); err != nil {
					t.Fatalf("RevokeToken %d failed: %v", i, err)
				}
			case "expired":
				// Manipulate the mock repo to set expires_at in the past.
				expiredTime := time.Now().Add(-time.Hour)
				tokenRepo.mu.Lock()
				if tok, ok := tokenRepo.byID[tokens[i].token.ID]; ok {
					tok.ExpiresAt = expiredTime
				}
				if tok, ok := tokenRepo.byHash[tokens[i].token.TokenHash]; ok {
					tok.ExpiresAt = expiredTime
				}
				tokenRepo.mu.Unlock()
			}
		}

		// List tokens and verify completeness.
		listed, err := svc.ListTokens(ctx, userID)
		if err != nil {
			t.Fatalf("ListTokens failed: %v", err)
		}

		// Verify exactly N tokens returned.
		if len(listed) != totalTokens {
			t.Fatalf("expected %d tokens, got %d", totalTokens, len(listed))
		}

		// Build a map from token ID to the original raw token for prefix checking.
		rawByID := make(map[uuid.UUID]string, totalTokens)
		for _, ti := range tokens {
			rawByID[ti.token.ID] = ti.raw
		}

		// Verify each listed token has a prefix matching the first 8 chars of the raw token.
		for _, lt := range listed {
			raw, ok := rawByID[lt.ID]
			if !ok {
				t.Fatalf("listed token %v not found in created tokens", lt.ID)
			}
			expectedPrefix := raw[:8]
			if lt.Prefix != expectedPrefix {
				t.Fatalf("token %v prefix mismatch: got %q, want %q", lt.ID, lt.Prefix, expectedPrefix)
			}
		}

		// Verify ordering: created_at descending (newest first).
		for i := 1; i < len(listed); i++ {
			if listed[i].CreatedAt.After(listed[i-1].CreatedAt) {
				t.Fatalf("ordering violation at index %d: %v is after %v",
					i, listed[i].CreatedAt, listed[i-1].CreatedAt)
			}
		}
	})
}
