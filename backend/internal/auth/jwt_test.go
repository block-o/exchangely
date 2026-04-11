package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"pgregory.net/rapid"
)

// TestPropertyJWTRoundTripPreservesIdentity verifies Property 1: JWT round-trip preserves user identity.
//
// For any valid User with any combination of id, email, and role, issuing a JWT
// access token and then validating it SHALL produce Claims containing the same
// user id, email, and role as the original User.
func TestPropertyJWTRoundTripPreservesIdentity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random user identity fields.
		userID := uuid.New()
		email := rapid.StringMatching(`[a-z]{1,15}@[a-z]{1,10}\.[a-z]{2,4}`).Draw(t, "email")
		role := rapid.SampledFrom([]string{"admin", "user"}).Draw(t, "role")

		user := User{
			ID:    userID,
			Email: email,
			Role:  role,
		}

		// Generate a random secret (16–64 bytes).
		secretLen := rapid.IntRange(16, 64).Draw(t, "secretLen")
		secret := make([]byte, secretLen)
		for i := range secret {
			secret[i] = byte(rapid.IntRange(0, 255).Draw(t, "secretByte"))
		}

		// Issue an access token with 15m expiry.
		tokenStr, err := IssueAccessToken(user, secret, 15*time.Minute)
		if err != nil {
			t.Fatalf("IssueAccessToken failed: %v", err)
		}

		// Validate the token with the same secret.
		claims, err := ValidateAccessToken(tokenStr, secret)
		if err != nil {
			t.Fatalf("ValidateAccessToken failed: %v", err)
		}

		// Assert identity is preserved.
		if claims.Sub != userID.String() {
			t.Fatalf("sub mismatch: got %q, want %q", claims.Sub, userID.String())
		}
		if claims.Email != email {
			t.Fatalf("email mismatch: got %q, want %q", claims.Email, email)
		}
		if claims.Role != role {
			t.Fatalf("role mismatch: got %q, want %q", claims.Role, role)
		}
	})
}

// TestPropertyExpiredJWTRejected verifies Property 2 (expired case): Invalid JWTs are rejected.
//
// For any valid User, issuing a JWT with an expiration in the past SHALL cause
// ValidateAccessToken to return an error.
func TestPropertyExpiredJWTRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		userID := uuid.New()
		email := rapid.StringMatching(`[a-z]{1,15}@[a-z]{1,10}\.[a-z]{2,4}`).Draw(t, "email")
		role := rapid.SampledFrom([]string{"admin", "user"}).Draw(t, "role")

		// Generate a random secret.
		secretLen := rapid.IntRange(16, 64).Draw(t, "secretLen")
		secret := make([]byte, secretLen)
		for i := range secret {
			secret[i] = byte(rapid.IntRange(0, 255).Draw(t, "secretByte"))
		}

		// Build a token that is already expired by crafting claims manually.
		// IssueAccessToken clamps non-positive expiry to 15m, so we construct
		// the token directly with an exp in the past.
		past := time.Now().Add(-1 * time.Hour)
		claims := Claims{
			Sub:   userID.String(),
			Email: email,
			Role:  role,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   userID.String(),
				IssuedAt:  jwt.NewNumericDate(past.Add(-1 * time.Hour)),
				ExpiresAt: jwt.NewNumericDate(past),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString(secret)
		if err != nil {
			t.Fatalf("failed to sign expired token: %v", err)
		}

		// Validation must reject the expired token.
		_, err = ValidateAccessToken(tokenStr, secret)
		if err == nil {
			t.Fatal("expected error for expired token, got nil")
		}
	})
}

// TestPropertyWrongSecretJWTRejected verifies Property 2 (wrong-secret case): Invalid JWTs are rejected.
//
// For any valid User, issuing a JWT with one secret and validating with a
// different secret SHALL cause ValidateAccessToken to return an error.
func TestPropertyWrongSecretJWTRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		userID := uuid.New()
		email := rapid.StringMatching(`[a-z]{1,15}@[a-z]{1,10}\.[a-z]{2,4}`).Draw(t, "email")
		role := rapid.SampledFrom([]string{"admin", "user"}).Draw(t, "role")

		user := User{
			ID:    userID,
			Email: email,
			Role:  role,
		}

		// Generate two distinct secrets.
		secretLen := rapid.IntRange(16, 64).Draw(t, "secretLen")
		issueSecret := make([]byte, secretLen)
		for i := range issueSecret {
			issueSecret[i] = byte(rapid.IntRange(0, 255).Draw(t, "issueByte"))
		}

		validateSecret := make([]byte, secretLen)
		for i := range validateSecret {
			validateSecret[i] = byte(rapid.IntRange(0, 255).Draw(t, "validateByte"))
		}

		// Ensure the two secrets differ. If they happen to be identical
		// (astronomically unlikely for len>=16), flip the last byte.
		identical := true
		for i := range issueSecret {
			if issueSecret[i] != validateSecret[i] {
				identical = false
				break
			}
		}
		if identical {
			validateSecret[0] ^= 0xFF
		}

		tokenStr, err := IssueAccessToken(user, issueSecret, 15*time.Minute)
		if err != nil {
			t.Fatalf("IssueAccessToken failed: %v", err)
		}

		// Validation with a different secret must fail.
		_, err = ValidateAccessToken(tokenStr, validateSecret)
		if err == nil {
			t.Fatal("expected error for wrong-secret token, got nil")
		}
	})
}

// TestPropertyMissingClaimsJWTRejected verifies Property 2 (missing-claims case): Invalid JWTs are rejected.
//
// For any JWT where at least one of the required claims (sub, email, role) is
// empty, ValidateAccessToken SHALL return an error.
func TestPropertyMissingClaimsJWTRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random secret.
		secretLen := rapid.IntRange(16, 64).Draw(t, "secretLen")
		secret := make([]byte, secretLen)
		for i := range secret {
			secret[i] = byte(rapid.IntRange(0, 255).Draw(t, "secretByte"))
		}

		// Pick which claim(s) to leave empty. At least one must be empty.
		emptySub := rapid.Bool().Draw(t, "emptySub")
		emptyEmail := rapid.Bool().Draw(t, "emptyEmail")
		emptyRole := rapid.Bool().Draw(t, "emptyRole")

		// Ensure at least one claim is empty.
		if !emptySub && !emptyEmail && !emptyRole {
			emptySub = true
		}

		sub := ""
		if !emptySub {
			sub = uuid.New().String()
		}
		email := ""
		if !emptyEmail {
			email = rapid.StringMatching(`[a-z]{1,15}@[a-z]{1,10}\.[a-z]{2,4}`).Draw(t, "email")
		}
		role := ""
		if !emptyRole {
			role = rapid.SampledFrom([]string{"admin", "user"}).Draw(t, "role")
		}

		now := time.Now()
		claims := Claims{
			Sub:   sub,
			Email: email,
			Role:  role,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   sub,
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString(secret)
		if err != nil {
			t.Fatalf("failed to sign token with missing claims: %v", err)
		}

		// Validation must reject the token due to missing required claims.
		_, err = ValidateAccessToken(tokenStr, secret)
		if err == nil {
			t.Fatalf("expected error for token with missing claims (sub=%q, email=%q, role=%q), got nil", sub, email, role)
		}
	})
}

// TestPropertyMalformedJWTRejected verifies Property 2 (malformed case): Invalid JWTs are rejected.
//
// For any random string that is not a properly structured JWT,
// ValidateAccessToken SHALL return an error.
func TestPropertyMalformedJWTRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random secret.
		secretLen := rapid.IntRange(16, 64).Draw(t, "secretLen")
		secret := make([]byte, secretLen)
		for i := range secret {
			secret[i] = byte(rapid.IntRange(0, 255).Draw(t, "secretByte"))
		}

		// Generate a random string of varying length (0–512 bytes).
		malformed := rapid.StringN(0, 512, -1).Draw(t, "malformedToken")

		_, err := ValidateAccessToken(malformed, secret)
		if err == nil {
			t.Fatalf("expected error for malformed token %q, got nil", malformed)
		}
	})
}

// TestPropertyMustChangePasswordPropagation verifies Property 11: must_change_password propagates to JWT.
//
// For any user with MustChangePassword set to true, the issued JWT access token
// SHALL contain a must_change_password: true claim. For any user with
// MustChangePassword set to false, the claim SHALL be false.
func TestPropertyMustChangePasswordPropagation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random user identity fields.
		userID := uuid.New()
		email := rapid.StringMatching(`[a-z]{1,15}@[a-z]{1,10}\.[a-z]{2,4}`).Draw(t, "email")
		role := rapid.SampledFrom([]string{"admin", "user"}).Draw(t, "role")
		mustChange := rapid.Bool().Draw(t, "mustChangePassword")

		user := User{
			ID:                 userID,
			Email:              email,
			Role:               role,
			MustChangePassword: mustChange,
		}

		// Generate a random secret (16–64 bytes).
		secretLen := rapid.IntRange(16, 64).Draw(t, "secretLen")
		secret := make([]byte, secretLen)
		for i := range secret {
			secret[i] = byte(rapid.IntRange(0, 255).Draw(t, "secretByte"))
		}

		// Issue an access token.
		tokenStr, err := IssueAccessToken(user, secret, 15*time.Minute)
		if err != nil {
			t.Fatalf("IssueAccessToken failed: %v", err)
		}

		// Validate the token.
		claims, err := ValidateAccessToken(tokenStr, secret)
		if err != nil {
			t.Fatalf("ValidateAccessToken failed: %v", err)
		}

		// Assert must_change_password claim matches the user flag.
		if claims.MustChangePassword != mustChange {
			t.Fatalf("MustChangePassword mismatch: got %v, want %v", claims.MustChangePassword, mustChange)
		}
	})
}
