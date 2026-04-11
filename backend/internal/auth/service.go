package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Service errors returned to callers.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrRateLimited        = errors.New("too many requests")
	ErrIPBlocked          = errors.New("ip blocked")
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionExpired     = errors.New("session expired")
	ErrUserNotFound       = errors.New("user not found")
	ErrCSRFStateMismatch  = errors.New("CSRF state mismatch")
	ErrOAuthExchange      = errors.New("OAuth code exchange failed")
	ErrOAuthUserInfo      = errors.New("failed to fetch Google user info")
	ErrAccountDisabled    = errors.New("account disabled")
)

const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"

	// refreshTokenBytes is the number of random bytes for a refresh token.
	refreshTokenBytes = 32
	// csrfStateBytes is the number of random bytes for CSRF state.
	csrfStateBytes = 32
	// bootstrapPasswordLen is the length of the auto-generated admin password.
	bootstrapPasswordLen = 24
	// minBcryptCostDefault enforces a minimum bcrypt cost factor.
	minBcryptCostDefault = 12
)

// Service implements the core authentication logic.
type Service struct {
	users         UserRepository
	sessions      SessionRepository
	cfg           Config
	rateLimiter   *RateLimiter
	httpClient    *http.Client
	minBcryptCost int // minimum bcrypt cost; defaults to minBcryptCostDefault
}

// NewService creates a new auth Service with a rate limiter configured for
// 5 attempts per 15-minute window.
func NewService(users UserRepository, sessions SessionRepository, cfg Config) *Service {
	return &Service{
		users:         users,
		sessions:      sessions,
		cfg:           cfg,
		rateLimiter:   NewRateLimiter(5, 15*time.Minute),
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		minBcryptCost: minBcryptCostDefault,
	}
}

// GoogleLogin generates a Google OAuth authorization URL with a random CSRF state.
func (s *Service) GoogleLogin() (authURL string, state string, err error) {
	stateBytes := make([]byte, csrfStateBytes)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", fmt.Errorf("generating CSRF state: %w", err)
	}
	state = base64.URLEncoding.EncodeToString(stateBytes)

	params := url.Values{
		"client_id":     {s.cfg.GoogleClientID},
		"redirect_uri":  {s.cfg.GoogleRedirectURI},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
	}
	authURL = googleAuthURL + "?" + params.Encode()
	return authURL, state, nil
}

// googleTokenResponse is the JSON response from Google's token endpoint.
type googleTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// googleUserInfo is the JSON response from Google's userinfo endpoint.
type googleUserInfo struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// GoogleCallback exchanges the authorization code for tokens, fetches the user
// profile from Google, upserts the user, and issues JWT + refresh tokens.
// The ip parameter is used to record IP-based rate limiting on OAuth failures.
func (s *Service) GoogleCallback(ctx context.Context, code, state, expectedState, ip string) (accessToken string, refreshToken string, user User, err error) {
	// Validate CSRF state using constant-time comparison via subtle is ideal,
	// but for string state comparison, exact match is sufficient since the state
	// is a random nonce, not a secret.
	if state != expectedState {
		if ip != "" {
			s.rateLimiter.RecordIP(ip)
		}
		return "", "", User{}, ErrCSRFStateMismatch
	}

	// Exchange authorization code for tokens.
	googleToken, err := s.exchangeCode(ctx, code)
	if err != nil {
		if ip != "" {
			s.rateLimiter.RecordIP(ip)
		}
		return "", "", User{}, fmt.Errorf("%w: %v", ErrOAuthExchange, err)
	}

	// Fetch user info from Google.
	info, err := s.fetchGoogleUserInfo(ctx, googleToken.AccessToken)
	if err != nil {
		if ip != "" {
			s.rateLimiter.RecordIP(ip)
		}
		return "", "", User{}, fmt.Errorf("%w: %v", ErrOAuthUserInfo, err)
	}

	// Upsert user.
	user, err = s.upsertGoogleUser(ctx, info)
	if err != nil {
		return "", "", User{}, fmt.Errorf("upserting Google user: %w", err)
	}

	// Check if account is disabled.
	if user.Disabled {
		slog.Info("auth event",
			"event", "oauth_disabled_account",
			"email", maskEmail(user.Email),
			"method", "google",
		)
		return "", "", User{}, ErrAccountDisabled
	}

	// Issue tokens.
	accessToken, err = IssueAccessToken(user, s.cfg.JWTSecret, s.cfg.JWTExpiry)
	if err != nil {
		return "", "", User{}, fmt.Errorf("issuing access token: %w", err)
	}

	refreshToken, err = s.createSession(ctx, user.ID, "")
	if err != nil {
		return "", "", User{}, fmt.Errorf("creating session: %w", err)
	}

	slog.Info("auth event",
		"event", "login_success",
		"email", maskEmail(user.Email),
		"method", "google",
	)

	return accessToken, refreshToken, user, nil
}

// LocalLogin validates email/password credentials, checks rate limits, and issues tokens.
func (s *Service) LocalLogin(ctx context.Context, email, password, userAgent, ip string) (accessToken string, refreshToken string, user User, err error) {
	email = TrimInput(email)

	// Check IP-based rate limit first (fail2ban style).
	if !s.rateLimiter.AllowIP(ip) {
		slog.Warn("auth event",
			"event", "login_ip_blocked",
			"email", maskEmail(email),
			"ip", ip,
			"user_agent", userAgent,
		)
		return "", "", User{}, ErrIPBlocked
	}

	if !s.rateLimiter.Allow(email) {
		slog.Warn("auth event",
			"event", "login_rate_limited",
			"email", maskEmail(email),
			"ip", ip,
			"user_agent", userAgent,
		)
		return "", "", User{}, ErrRateLimited
	}

	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return "", "", User{}, fmt.Errorf("finding user: %w", err)
	}
	if u == nil || u.PasswordHash == nil {
		// Record failed attempt even for non-existent users to prevent enumeration.
		s.rateLimiter.Record(email)
		s.rateLimiter.RecordIP(ip)
		slog.Info("auth event",
			"event", "login_failed",
			"email", maskEmail(email),
			"ip", ip,
			"user_agent", userAgent,
			"method", "local",
		)
		return "", "", User{}, ErrInvalidCredentials
	}

	// Check if account is disabled.
	if u.Disabled {
		slog.Info("auth event",
			"event", "login_disabled_account",
			"email", maskEmail(email),
			"ip", ip,
			"user_agent", userAgent,
			"method", "local",
		)
		return "", "", User{}, ErrAccountDisabled
	}

	// Constant-time bcrypt comparison.
	if err := bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(password)); err != nil {
		s.rateLimiter.Record(email)
		s.rateLimiter.RecordIP(ip)
		slog.Info("auth event",
			"event", "login_failed",
			"email", maskEmail(email),
			"ip", ip,
			"user_agent", userAgent,
			"method", "local",
		)
		return "", "", User{}, ErrInvalidCredentials
	}

	// Successful login — reset rate limiters.
	s.rateLimiter.Reset(email)
	s.rateLimiter.ResetIP(ip)

	accessToken, err = IssueAccessToken(*u, s.cfg.JWTSecret, s.cfg.JWTExpiry)
	if err != nil {
		return "", "", User{}, fmt.Errorf("issuing access token: %w", err)
	}

	refreshToken, err = s.createSession(ctx, u.ID, userAgent)
	if err != nil {
		return "", "", User{}, fmt.Errorf("creating session: %w", err)
	}

	slog.Info("auth event",
		"event", "login_success",
		"email", maskEmail(u.Email),
		"ip", ip,
		"user_agent", userAgent,
		"method", "local",
	)

	return accessToken, refreshToken, *u, nil
}

// RefreshToken validates a refresh token, rotates the session, and issues new tokens.
func (s *Service) RefreshToken(ctx context.Context, rawRefreshToken string, userAgent string) (accessToken string, newRefreshToken string, err error) {
	tokenHash := hashToken(rawRefreshToken)

	session, err := s.sessions.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return "", "", fmt.Errorf("finding session: %w", err)
	}
	if session == nil {
		return "", "", ErrSessionNotFound
	}

	if time.Now().After(session.ExpiresAt) {
		// Clean up expired session.
		_ = s.sessions.Delete(ctx, session.ID)
		return "", "", ErrSessionExpired
	}

	// Delete old session.
	if err := s.sessions.Delete(ctx, session.ID); err != nil {
		return "", "", fmt.Errorf("deleting old session: %w", err)
	}

	// Look up the user for token issuance.
	u, err := s.users.FindByID(ctx, session.UserID)
	if err != nil {
		return "", "", fmt.Errorf("finding user: %w", err)
	}
	if u == nil {
		return "", "", ErrUserNotFound
	}

	// Check if account is disabled.
	if u.Disabled {
		slog.Info("auth event",
			"event", "refresh_disabled_account",
			"email", maskEmail(u.Email),
		)
		return "", "", ErrAccountDisabled
	}

	// Issue new access token.
	accessToken, err = IssueAccessToken(*u, s.cfg.JWTSecret, s.cfg.JWTExpiry)
	if err != nil {
		return "", "", fmt.Errorf("issuing access token: %w", err)
	}

	// Create new session with rotated refresh token.
	newRefreshToken, err = s.createSession(ctx, u.ID, userAgent)
	if err != nil {
		return "", "", fmt.Errorf("creating new session: %w", err)
	}

	slog.Info("auth event",
		"event", "token_refresh",
		"email", maskEmail(u.Email),
	)

	return accessToken, newRefreshToken, nil
}

// Logout invalidates the session associated with the given refresh token.
func (s *Service) Logout(ctx context.Context, rawRefreshToken string) error {
	tokenHash := hashToken(rawRefreshToken)

	session, err := s.sessions.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return fmt.Errorf("finding session: %w", err)
	}
	if session == nil {
		// Already logged out or invalid token — treat as success.
		return nil
	}

	if err := s.sessions.Delete(ctx, session.ID); err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}

	slog.Info("auth event",
		"event", "logout",
	)

	return nil
}

// ChangePassword validates the current password, enforces complexity rules on the
// new password, updates the hash, and invalidates all sessions for the user.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("finding user: %w", err)
	}
	if u == nil || u.PasswordHash == nil {
		return ErrInvalidCredentials
	}

	// Constant-time bcrypt comparison for current password.
	if err := bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}

	// Validate new password complexity.
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}

	// Hash new password with minimum bcrypt cost.
	cost := s.cfg.BcryptCost
	if cost < s.minBcryptCost {
		cost = s.minBcryptCost
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), cost)
	if err != nil {
		return fmt.Errorf("hashing new password: %w", err)
	}

	// Update password hash and clear must_change_password flag.
	if err := s.users.UpdatePasswordHash(ctx, userID, string(hash), false); err != nil {
		return fmt.Errorf("updating password hash: %w", err)
	}

	// Invalidate all sessions for this user.
	if err := s.sessions.DeleteAllForUser(ctx, userID); err != nil {
		return fmt.Errorf("deleting sessions: %w", err)
	}

	slog.Info("auth event",
		"event", "password_change",
		"email", maskEmail(u.Email),
	)

	return nil
}

// Me returns the user profile for the given user ID.
func (s *Service) Me(ctx context.Context, userID uuid.UUID) (User, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return User{}, fmt.Errorf("finding user: %w", err)
	}
	if u == nil {
		return User{}, ErrUserNotFound
	}
	return *u, nil
}

// AuthMethods returns which authentication methods are currently enabled,
// based on the configured auth mode and the presence of required credentials.
func (s *Service) AuthMethods() AuthMethodsResponse {
	return AuthMethodsResponse{
		Google: s.ssoEnabled(),
		Local:  s.localEnabled(),
	}
}

// ssoEnabled returns true when the auth mode includes SSO and Google credentials are configured.
func (s *Service) ssoEnabled() bool {
	return strings.Contains(s.cfg.AuthMode, "sso") &&
		s.cfg.GoogleClientID != "" && s.cfg.GoogleClientSecret != ""
}

// localEnabled returns true when the auth mode includes local.
func (s *Service) localEnabled() bool {
	return strings.Contains(s.cfg.AuthMode, "local")
}

// ValidateAccessToken parses and validates a JWT access token, returning the claims.
func (s *Service) ValidateAccessToken(tokenString string) (*Claims, error) {
	return ValidateAccessToken(tokenString, s.cfg.JWTSecret)
}

// BootstrapAdmin creates the local admin account on first startup if configured.
// If BACKEND_ADMIN_EMAIL is set and no user with that email exists, it generates
// a random password, creates the user with role=admin and must_change_password=true,
// and logs the password once.
func (s *Service) BootstrapAdmin(ctx context.Context) error {
	if s.cfg.AdminEmail == "" {
		return nil
	}

	existing, err := s.users.FindByEmail(ctx, s.cfg.AdminEmail)
	if err != nil {
		return fmt.Errorf("checking existing admin: %w", err)
	}
	if existing != nil {
		slog.Info("admin bootstrap skipped — user already exists",
			"email", maskEmail(s.cfg.AdminEmail),
		)
		return nil
	}

	// Skip bootstrap if any active admin already exists in the system.
	admins, _, err := s.users.ListWithFilters(ctx, "", "admin", "active", 1, 0)
	if err != nil {
		return fmt.Errorf("checking existing admins: %w", err)
	}
	if len(admins) > 0 {
		slog.Info("admin bootstrap skipped — active admin already exists",
			"existing_admin", maskEmail(admins[0].Email),
		)
		return nil
	}

	// Generate a cryptographically random password.
	password, err := generateRandomPassword(bootstrapPasswordLen)
	if err != nil {
		return fmt.Errorf("generating admin password: %w", err)
	}

	// Hash with minimum bcrypt cost.
	cost := s.cfg.BcryptCost
	if cost < s.minBcryptCost {
		cost = s.minBcryptCost
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return fmt.Errorf("hashing admin password: %w", err)
	}

	hashStr := string(hash)
	user := &User{
		ID:                 uuid.New(),
		Email:              s.cfg.AdminEmail,
		Name:               "Admin",
		Role:               "admin",
		PasswordHash:       &hashStr,
		MustChangePassword: true,
	}

	if err := s.users.Create(ctx, user); err != nil {
		return fmt.Errorf("creating admin user: %w", err)
	}

	slog.Info("Local admin created — this password will not be shown again.",
		"email", s.cfg.AdminEmail,
		"password", password,
	)

	return nil
}

// exchangeCode exchanges an OAuth authorization code for Google tokens.
func (s *Service) exchangeCode(ctx context.Context, code string) (*googleTokenResponse, error) {
	data := url.Values{
		"code":          {code},
		"client_id":     {s.cfg.GoogleClientID},
		"client_secret": {s.cfg.GoogleClientSecret},
		"redirect_uri":  {s.cfg.GoogleRedirectURI},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp googleTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}
	return &tokenResp, nil
}

// fetchGoogleUserInfo retrieves the user profile from Google's userinfo endpoint.
func (s *Service) fetchGoogleUserInfo(ctx context.Context, accessToken string) (*googleUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("userinfo endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var info googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding userinfo response: %w", err)
	}
	return &info, nil
}

// upsertGoogleUser creates or updates a user from a Google profile.
func (s *Service) upsertGoogleUser(ctx context.Context, info *googleUserInfo) (User, error) {
	existing, err := s.users.FindByGoogleID(ctx, info.ID)
	if err != nil {
		return User{}, err
	}

	if existing != nil {
		// Update name and avatar only.
		existing.Name = info.Name
		existing.AvatarURL = info.Picture
		if err := s.users.Update(ctx, existing); err != nil {
			return User{}, err
		}
		return *existing, nil
	}

	// Create new user.
	googleID := info.ID
	user := &User{
		ID:        uuid.New(),
		Email:     info.Email,
		Name:      info.Name,
		AvatarURL: info.Picture,
		Role:      "user",
		GoogleID:  &googleID,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return User{}, err
	}

	slog.Info("auth event",
		"event", "user_signup",
		"email", maskEmail(user.Email),
		"method", "google",
		"user_id", user.ID,
	)

	return *user, nil
}

// createSession generates a random refresh token, hashes it, stores the session,
// and returns the raw token.
func (s *Service) createSession(ctx context.Context, userID uuid.UUID, userAgent string) (string, error) {
	rawToken, err := generateRandomToken(refreshTokenBytes)
	if err != nil {
		return "", fmt.Errorf("generating refresh token: %w", err)
	}

	session := &Session{
		ID:               uuid.New(),
		UserID:           userID,
		RefreshTokenHash: hashToken(rawToken),
		ExpiresAt:        time.Now().Add(s.cfg.RefreshTokenExpiry),
		UserAgent:        userAgent,
	}

	if err := s.sessions.Create(ctx, session); err != nil {
		return "", err
	}

	return rawToken, nil
}

// hashToken returns the hex-encoded SHA-256 hash of a raw token string.
//
// SHA-256 is intentional here — this hashes high-entropy, cryptographically
// random API tokens and refresh tokens (32 bytes from crypto/rand), NOT
// user-chosen passwords. Passwords use bcrypt (see BootstrapAdmin,
// ChangePassword). For random tokens, SHA-256 is the industry standard
// (GitHub, Stripe, AWS) because the input is already infeasible to brute-force
// and a slow KDF would add unnecessary latency to every authenticated request.
func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw)) // #nosec G401 -- not password hashing; see comment above
	return hex.EncodeToString(h[:])
}

// generateRandomToken generates a base64url-encoded random token of n bytes.
func generateRandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// generateRandomPassword generates a random password of the given length using
// alphanumeric characters and common special characters.
func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

// maskEmail masks an email for logging: "a***n@example.com".
func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return "***"
	}
	local := parts[0]
	if len(local) <= 2 {
		return local[:1] + "***@" + parts[1]
	}
	return string(local[0]) + "***" + string(local[len(local)-1]) + "@" + parts[1]
}
