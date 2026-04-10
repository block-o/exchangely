package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"pgregory.net/rapid"
)

// testConfig returns a Config suitable for testing with short expiries and low bcrypt cost.
func testConfig() Config {
	return Config{
		JWTSecret:          []byte("test-secret-at-least-16-bytes!!"),
		JWTExpiry:          15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         bcrypt.MinCost,
		AdminEmail:         "",
	}
}

// newTestService creates a Service with bcrypt.MinCost as the minimum, making
// password hashing fast for property-based tests that run many iterations.
func newTestService(users UserRepository, sessions SessionRepository, cfg Config) *Service {
	svc := NewService(users, sessions, cfg)
	svc.minBcryptCost = bcrypt.MinCost
	return svc
}

// createTestUser creates a user in the mock repo with a bcrypt-hashed password and returns it.
func createTestUser(t interface{ Fatalf(string, ...any) }, users *mockUserRepo, email, password, role string) User {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt hash failed: %v", err)
	}
	hashStr := string(hash)
	u := User{
		ID:           uuid.New(),
		Email:        email,
		Role:         role,
		PasswordHash: &hashStr,
	}
	if err := users.Create(context.Background(), &u); err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	return u
}

// =============================================================================
// Property 3: Google OAuth user upsert correctness
// =============================================================================

// TestPropertyGoogleOAuthUpsertCorrectness verifies Property 3.
//
// For any Google profile, if no User with that google_id exists, the upsert SHALL
// create a new User with role "user" and all profile fields populated. If a User
// with that google_id already exists, the upsert SHALL update only name and
// avatar_url, leaving role, email, and google_id unchanged.
//
// **Validates: Requirements 2.3, 2.4**
func TestPropertyGoogleOAuthUpsertCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		users := newMockUserRepo()
		sessions := newMockSessionRepo()
		cfg := testConfig()
		svc := NewService(users, sessions, cfg)

		googleID := rapid.StringMatching(`[a-z0-9]{10,20}`).Draw(t, "googleID")
		email := rapid.StringMatching(`[a-z]{1,10}@[a-z]{1,8}\.[a-z]{2,4}`).Draw(t, "email")
		name := rapid.StringMatching(`[A-Za-z ]{1,30}`).Draw(t, "name")
		avatar := rapid.StringMatching(`https://[a-z]{1,10}\.[a-z]{2,4}/[a-z]{1,10}`).Draw(t, "avatar")

		info := &googleUserInfo{
			ID:      googleID,
			Email:   email,
			Name:    name,
			Picture: avatar,
		}

		// First upsert: should create a new user.
		user, err := svc.upsertGoogleUser(context.Background(), info)
		if err != nil {
			t.Fatalf("first upsert failed: %v", err)
		}
		if user.Role != "user" {
			t.Fatalf("new user role: got %q, want %q", user.Role, "user")
		}
		if user.Email != email {
			t.Fatalf("new user email: got %q, want %q", user.Email, email)
		}
		if user.Name != name {
			t.Fatalf("new user name: got %q, want %q", user.Name, name)
		}
		if user.AvatarURL != avatar {
			t.Fatalf("new user avatar: got %q, want %q", user.AvatarURL, avatar)
		}
		if user.GoogleID == nil || *user.GoogleID != googleID {
			t.Fatalf("new user google_id: got %v, want %q", user.GoogleID, googleID)
		}

		// Second upsert with updated name and avatar: should update only those fields.
		newName := rapid.StringMatching(`[A-Za-z ]{1,30}`).Draw(t, "newName")
		newAvatar := rapid.StringMatching(`https://[a-z]{1,10}\.[a-z]{2,4}/[a-z]{1,10}`).Draw(t, "newAvatar")
		info2 := &googleUserInfo{
			ID:      googleID,
			Email:   "different@example.com", // email in profile changed, but should not update
			Name:    newName,
			Picture: newAvatar,
		}

		updated, err := svc.upsertGoogleUser(context.Background(), info2)
		if err != nil {
			t.Fatalf("second upsert failed: %v", err)
		}
		// Role, email, and google_id must remain unchanged.
		if updated.Role != user.Role {
			t.Fatalf("role changed after update: got %q, want %q", updated.Role, user.Role)
		}
		if updated.Email != user.Email {
			t.Fatalf("email changed after update: got %q, want %q", updated.Email, user.Email)
		}
		if updated.GoogleID == nil || *updated.GoogleID != googleID {
			t.Fatalf("google_id changed after update")
		}
		// Name and avatar should be updated.
		if updated.Name != newName {
			t.Fatalf("name not updated: got %q, want %q", updated.Name, newName)
		}
		if updated.AvatarURL != newAvatar {
			t.Fatalf("avatar not updated: got %q, want %q", updated.AvatarURL, newAvatar)
		}
	})
}

// =============================================================================
// Property 4: Refresh token rotation
// =============================================================================

// TestPropertyRefreshTokenRotation verifies Property 4.
//
// For any valid session, calling RefreshToken SHALL delete the old session,
// create a new session with a different refresh token hash, and return a valid
// new access token.
//
// **Validates: Requirements 4.4**
func TestPropertyRefreshTokenRotation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		users := newMockUserRepo()
		sessions := newMockSessionRepo()
		cfg := testConfig()
		svc := NewService(users, sessions, cfg)

		email := rapid.StringMatching(`[a-z]{1,10}@[a-z]{1,8}\.[a-z]{2,4}`).Draw(t, "email")
		role := rapid.SampledFrom([]string{"admin", "user"}).Draw(t, "role")
		user := createTestUser(t, users, email, "Str0ng!Pass12", role)

		// Create a session via the service's internal helper.
		rawToken, err := svc.createSession(context.Background(), user.ID, "test-agent")
		if err != nil {
			t.Fatalf("createSession failed: %v", err)
		}

		// Record the old session hash.
		oldHash := hashToken(rawToken)
		oldSession, _ := sessions.FindByTokenHash(context.Background(), oldHash)
		if oldSession == nil {
			t.Fatal("old session not found after creation")
		}

		// Refresh the token.
		newAccessToken, newRefreshToken, err := svc.RefreshToken(context.Background(), rawToken, "test-agent")
		if err != nil {
			t.Fatalf("RefreshToken failed: %v", err)
		}

		// Old session must be deleted.
		gone, _ := sessions.FindByTokenHash(context.Background(), oldHash)
		if gone != nil {
			t.Fatal("old session still exists after refresh")
		}

		// New session must exist with a different hash.
		newHash := hashToken(newRefreshToken)
		if newHash == oldHash {
			t.Fatal("new refresh token hash equals old hash")
		}
		newSession, _ := sessions.FindByTokenHash(context.Background(), newHash)
		if newSession == nil {
			t.Fatal("new session not found after refresh")
		}

		// New access token must be valid.
		claims, err := ValidateAccessToken(newAccessToken, cfg.JWTSecret)
		if err != nil {
			t.Fatalf("new access token invalid: %v", err)
		}
		if claims.Sub != user.ID.String() {
			t.Fatalf("access token sub mismatch: got %q, want %q", claims.Sub, user.ID.String())
		}
	})
}

// =============================================================================
// Property 5: Invalid refresh tokens are rejected
// =============================================================================

// TestPropertyInvalidRefreshTokensRejected verifies Property 5.
//
// For any refresh token that is expired, does not match any active session, or
// is an arbitrary random string, RefreshToken SHALL return an error.
//
// **Validates: Requirements 4.5**
func TestPropertyInvalidRefreshTokensRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		users := newMockUserRepo()
		sessions := newMockSessionRepo()
		cfg := testConfig()
		svc := NewService(users, sessions, cfg)

		scenario := rapid.SampledFrom([]string{"random", "expired", "nonmatching"}).Draw(t, "scenario")

		switch scenario {
		case "random":
			// A completely random string should not match any session.
			randomToken := rapid.StringN(10, 100, -1).Draw(t, "randomToken")
			_, _, err := svc.RefreshToken(context.Background(), randomToken, "agent")
			if err == nil {
				t.Fatal("expected error for random refresh token, got nil")
			}

		case "expired":
			// Create a user and an expired session.
			email := rapid.StringMatching(`[a-z]{1,10}@[a-z]{1,8}\.[a-z]{2,4}`).Draw(t, "email")
			user := createTestUser(t, users, email, "Str0ng!Pass12", "user")

			rawToken, err := generateRandomToken(refreshTokenBytes)
			if err != nil {
				t.Fatalf("generateRandomToken failed: %v", err)
			}
			expiredSession := &Session{
				ID:               uuid.New(),
				UserID:           user.ID,
				RefreshTokenHash: hashToken(rawToken),
				ExpiresAt:        time.Now().Add(-1 * time.Hour), // expired
				UserAgent:        "test",
			}
			if err := sessions.Create(context.Background(), expiredSession); err != nil {
				t.Fatalf("create expired session failed: %v", err)
			}

			_, _, err = svc.RefreshToken(context.Background(), rawToken, "agent")
			if err == nil {
				t.Fatal("expected error for expired refresh token, got nil")
			}
			if !errors.Is(err, ErrSessionExpired) {
				t.Fatalf("expected ErrSessionExpired, got: %v", err)
			}

		case "nonmatching":
			// Create a user and a valid session, but try to refresh with a different token.
			email := rapid.StringMatching(`[a-z]{1,10}@[a-z]{1,8}\.[a-z]{2,4}`).Draw(t, "email")
			user := createTestUser(t, users, email, "Str0ng!Pass12", "user")

			// Create a real session.
			_, err := svc.createSession(context.Background(), user.ID, "agent")
			if err != nil {
				t.Fatalf("createSession failed: %v", err)
			}

			// Try to refresh with a different token.
			wrongToken, _ := generateRandomToken(refreshTokenBytes)
			_, _, err = svc.RefreshToken(context.Background(), wrongToken, "agent")
			if err == nil {
				t.Fatal("expected error for non-matching refresh token, got nil")
			}
		}
	})
}

// =============================================================================
// Property 6: Logout deletes session
// =============================================================================

// TestPropertyLogoutDeletesSession verifies Property 6.
//
// For any active session, calling Logout with the corresponding refresh token
// SHALL delete the session row, leaving zero sessions for that token hash.
//
// **Validates: Requirements 4.6**
func TestPropertyLogoutDeletesSession(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		users := newMockUserRepo()
		sessions := newMockSessionRepo()
		cfg := testConfig()
		svc := NewService(users, sessions, cfg)

		email := rapid.StringMatching(`[a-z]{1,10}@[a-z]{1,8}\.[a-z]{2,4}`).Draw(t, "email")
		user := createTestUser(t, users, email, "Str0ng!Pass12", "user")

		// Create a session.
		rawToken, err := svc.createSession(context.Background(), user.ID, "test-agent")
		if err != nil {
			t.Fatalf("createSession failed: %v", err)
		}

		// Verify session exists.
		tokenHash := hashToken(rawToken)
		s, _ := sessions.FindByTokenHash(context.Background(), tokenHash)
		if s == nil {
			t.Fatal("session not found before logout")
		}

		// Logout.
		if err := svc.Logout(context.Background(), rawToken); err != nil {
			t.Fatalf("Logout failed: %v", err)
		}

		// Session must be gone.
		s, _ = sessions.FindByTokenHash(context.Background(), tokenHash)
		if s != nil {
			t.Fatal("session still exists after logout")
		}
	})
}

// =============================================================================
// Property 7: CSRF state validation on OAuth callback
// =============================================================================

// TestPropertyCSRFStateValidation verifies Property 7.
//
// For any pair of state values where the callback state does not equal the
// expected state, GoogleCallback SHALL reject the request with ErrCSRFStateMismatch.
// Only when the two state values are equal SHALL the callback proceed past state
// validation (it will fail later on the HTTP exchange, which is expected).
//
// **Validates: Requirements 5.6**
func TestPropertyCSRFStateValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		users := newMockUserRepo()
		sessions := newMockSessionRepo()
		cfg := testConfig()
		svc := NewService(users, sessions, cfg)

		state1 := rapid.StringN(1, 64, -1).Draw(t, "state1")
		state2 := rapid.StringN(1, 64, -1).Draw(t, "state2")

		// Ensure the states are different for the mismatch test.
		if state1 == state2 {
			state2 = state2 + "x"
		}

		// Mismatched states must produce ErrCSRFStateMismatch.
		_, _, _, err := svc.GoogleCallback(context.Background(), "fake-code", state1, state2, "127.0.0.1")
		if !errors.Is(err, ErrCSRFStateMismatch) {
			t.Fatalf("expected ErrCSRFStateMismatch for mismatched states, got: %v", err)
		}
	})
}

// =============================================================================
// Property 9: Admin bootstrap creates correct user
// =============================================================================

// TestPropertyAdminBootstrapCreatesCorrectUser verifies Property 9.
//
// For any valid email configured in AdminEmail, when no user with that email
// exists, BootstrapAdmin SHALL create a User with role "admin",
// must_change_password=true, a valid bcrypt password hash with cost >= the
// configured minimum, and a null google_id.
//
// **Validates: Requirements 11.2**
func TestPropertyAdminBootstrapCreatesCorrectUser(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		users := newMockUserRepo()
		sessions := newMockSessionRepo()
		cfg := testConfig()

		email := rapid.StringMatching(`[a-z]{1,10}@[a-z]{1,8}\.[a-z]{2,4}`).Draw(t, "adminEmail")
		cfg.AdminEmail = email

		svc := newTestService(users, sessions, cfg)

		if err := svc.BootstrapAdmin(context.Background()); err != nil {
			t.Fatalf("BootstrapAdmin failed: %v", err)
		}

		// Verify the user was created.
		u, err := users.FindByEmail(context.Background(), email)
		if err != nil {
			t.Fatalf("FindByEmail failed: %v", err)
		}
		if u == nil {
			t.Fatal("admin user not created")
			return // unreachable but satisfies staticcheck
		}

		// Role must be admin.
		if u.Role != "admin" {
			t.Fatalf("role: got %q, want %q", u.Role, "admin")
		}

		// must_change_password must be true.
		if !u.MustChangePassword {
			t.Fatal("must_change_password should be true")
		}

		// Password hash must be non-empty and valid bcrypt with cost >= service minimum.
		if u.PasswordHash == nil || *u.PasswordHash == "" {
			t.Fatal("password hash is empty")
		}
		cost, err := bcrypt.Cost([]byte(*u.PasswordHash))
		if err != nil {
			t.Fatalf("bcrypt.Cost failed: %v", err)
		}
		if cost < svc.minBcryptCost {
			t.Fatalf("bcrypt cost: got %d, want >= %d", cost, svc.minBcryptCost)
		}

		// google_id must be nil.
		if u.GoogleID != nil {
			t.Fatalf("google_id should be nil, got %v", *u.GoogleID)
		}
	})
}

// =============================================================================
// Property 10: Admin bootstrap is idempotent
// =============================================================================

// TestPropertyAdminBootstrapIdempotent verifies Property 10.
//
// For any existing user matching AdminEmail, re-running BootstrapAdmin SHALL not
// modify the user's password_hash, role, must_change_password, or any other field.
//
// **Validates: Requirements 11.4**
func TestPropertyAdminBootstrapIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		users := newMockUserRepo()
		sessions := newMockSessionRepo()
		cfg := testConfig()

		email := rapid.StringMatching(`[a-z]{1,10}@[a-z]{1,8}\.[a-z]{2,4}`).Draw(t, "adminEmail")
		cfg.AdminEmail = email

		svc := newTestService(users, sessions, cfg)

		// First bootstrap creates the admin.
		if err := svc.BootstrapAdmin(context.Background()); err != nil {
			t.Fatalf("first BootstrapAdmin failed: %v", err)
		}

		// Snapshot the user state after first bootstrap.
		u1, _ := users.FindByEmail(context.Background(), email)
		if u1 == nil {
			t.Fatal("admin user not created after first bootstrap")
			return // unreachable but satisfies staticcheck
		}
		origHash := ""
		if u1.PasswordHash != nil {
			origHash = *u1.PasswordHash
		}
		origRole := u1.Role
		origMustChange := u1.MustChangePassword
		origID := u1.ID

		// Second bootstrap should be a no-op.
		if err := svc.BootstrapAdmin(context.Background()); err != nil {
			t.Fatalf("second BootstrapAdmin failed: %v", err)
		}

		// Verify nothing changed.
		u2, _ := users.FindByEmail(context.Background(), email)
		if u2 == nil {
			t.Fatal("admin user disappeared after second bootstrap")
			return // unreachable but satisfies staticcheck
		}
		if u2.ID != origID {
			t.Fatalf("user ID changed: got %v, want %v", u2.ID, origID)
		}
		if u2.Role != origRole {
			t.Fatalf("role changed: got %q, want %q", u2.Role, origRole)
		}
		if u2.MustChangePassword != origMustChange {
			t.Fatalf("must_change_password changed: got %v, want %v", u2.MustChangePassword, origMustChange)
		}
		currentHash := ""
		if u2.PasswordHash != nil {
			currentHash = *u2.PasswordHash
		}
		if currentHash != origHash {
			t.Fatal("password_hash changed after second bootstrap")
		}
	})
}

// =============================================================================
// Property 14: Auth error responses are generic
// =============================================================================

// TestPropertyAuthErrorResponsesAreGeneric verifies Property 14.
//
// For any failed authentication attempt — whether the email does not exist, the
// password is wrong, or the account has no password — the service SHALL return
// the same ErrInvalidCredentials error, indistinguishable across failure modes.
//
// **Validates: Requirements 11.7, 12.5**
func TestPropertyAuthErrorResponsesAreGeneric(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		users := newMockUserRepo()
		sessions := newMockSessionRepo()
		cfg := testConfig()
		svc := NewService(users, sessions, cfg)

		// Create a user with a known password.
		knownEmail := rapid.StringMatching(`[a-z]{1,10}@[a-z]{1,8}\.[a-z]{2,4}`).Draw(t, "knownEmail")
		knownPassword := "Str0ng!Pass12"
		createTestUser(t, users, knownEmail, knownPassword, "user")

		scenario := rapid.SampledFrom([]string{"nonexistent_email", "wrong_password"}).Draw(t, "scenario")

		var err error
		switch scenario {
		case "nonexistent_email":
			// Email that doesn't exist in the system.
			fakeEmail := rapid.StringMatching(`nonexist[a-z]{1,5}@[a-z]{1,5}\.[a-z]{2,3}`).Draw(t, "fakeEmail")
			_, _, _, err = svc.LocalLogin(context.Background(), fakeEmail, "anypassword", "agent", "127.0.0.1")

		case "wrong_password":
			// Correct email but wrong password.
			wrongPass := rapid.StringMatching(`[A-Za-z0-9!@#]{12,20}`).Draw(t, "wrongPass")
			// Ensure it's actually different from the known password.
			if wrongPass == knownPassword {
				wrongPass = wrongPass + "x"
			}
			_, _, _, err = svc.LocalLogin(context.Background(), knownEmail, wrongPass, "agent", "127.0.0.1")
		}

		// All failure modes must return ErrInvalidCredentials.
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("scenario %q: expected ErrInvalidCredentials, got: %v", scenario, err)
		}
	})
}

// =============================================================================
// Property 16: Password change invalidates all sessions
// =============================================================================

// TestPropertyPasswordChangeInvalidatesAllSessions verifies Property 16.
//
// For any user with one or more active sessions, successfully changing the
// password SHALL delete all session rows for that user, leaving zero active sessions.
//
// **Validates: Requirements 12.14**
func TestPropertyPasswordChangeInvalidatesAllSessions(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		users := newMockUserRepo()
		sessions := newMockSessionRepo()
		cfg := testConfig()
		svc := newTestService(users, sessions, cfg)

		email := rapid.StringMatching(`[a-z]{1,10}@[a-z]{1,8}\.[a-z]{2,4}`).Draw(t, "email")
		currentPassword := "Str0ng!Pass12"
		user := createTestUser(t, users, email, currentPassword, "user")

		// Create N sessions (1 to 5).
		numSessions := rapid.IntRange(1, 5).Draw(t, "numSessions")
		for i := 0; i < numSessions; i++ {
			_, err := svc.createSession(context.Background(), user.ID, "agent")
			if err != nil {
				t.Fatalf("createSession %d failed: %v", i, err)
			}
		}

		// Verify sessions exist.
		count := sessions.sessionsForUser(user.ID)
		if count != numSessions {
			t.Fatalf("expected %d sessions before change, got %d", numSessions, count)
		}

		// Change password with a valid new password.
		newPassword := "N3wStr0ng!Pass"
		if err := svc.ChangePassword(context.Background(), user.ID, currentPassword, newPassword); err != nil {
			t.Fatalf("ChangePassword failed: %v", err)
		}

		// All sessions must be deleted.
		count = sessions.sessionsForUser(user.ID)
		if count != 0 {
			t.Fatalf("expected 0 sessions after password change, got %d", count)
		}
	})
}

// =============================================================================
// Unit test: LocalLogin returns ErrIPBlocked when IP is blocked
// =============================================================================

// TestLocalLoginReturnsErrIPBlocked verifies that LocalLogin returns ErrIPBlocked
// when the IP has exceeded the IP-based rate limit threshold.
func TestLocalLoginReturnsErrIPBlocked(t *testing.T) {
	users := newMockUserRepo()
	sessions := newMockSessionRepo()
	cfg := testConfig()
	svc := newTestService(users, sessions, cfg)

	// Create a user so the email is valid.
	createTestUser(t, users, "admin@example.com", "Str0ng!Pass12", "admin")

	ip := "192.168.1.100"

	// Record 20 failed IP attempts to trigger the IP ban.
	for i := 0; i < 20; i++ {
		svc.rateLimiter.RecordIP(ip)
	}

	// Now LocalLogin should return ErrIPBlocked.
	_, _, _, err := svc.LocalLogin(context.Background(), "admin@example.com", "Str0ng!Pass12", "test-agent", ip)
	if !errors.Is(err, ErrIPBlocked) {
		t.Fatalf("expected ErrIPBlocked, got: %v", err)
	}
}
