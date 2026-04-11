package auth

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// User represents an authenticated Exchangely account.
type User struct {
	ID                 uuid.UUID `json:"id"`
	Email              string    `json:"email"`
	Name               string    `json:"name"`
	AvatarURL          string    `json:"avatar_url"`
	Role               string    `json:"role"`
	GoogleID           *string   `json:"-"`
	PasswordHash       *string   `json:"-"`
	HasGoogle          bool      `json:"has_google"`
	HasPassword        bool      `json:"has_password"`
	Disabled           bool      `json:"disabled"`
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Session represents a refresh-token session stored in the database.
type Session struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	RefreshTokenHash string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	UserAgent        string
}

// Claims are the JWT access token claims issued by the auth service.
type Claims struct {
	Sub                string `json:"sub"`
	Email              string `json:"email"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password,omitempty"`
	jwt.RegisteredClaims
}

// AuthMethodsResponse indicates which login methods are currently enabled.
type AuthMethodsResponse struct {
	Google bool `json:"google"`
	Local  bool `json:"local"`
}

// Config holds auth-specific configuration read from environment variables.
type Config struct {
	AuthMode           string // "local", "sso", "local,sso", or "" (disabled)
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string
	JWTSecret          []byte
	JWTExpiry          time.Duration
	RefreshTokenExpiry time.Duration
	BcryptCost         int
	AdminEmail         string
	Env                string // "development" or "production"
}

// UserRepository defines persistence operations for users.
type UserRepository interface {
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByGoogleID(ctx context.Context, googleID string) (*User, error)
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	UpdatePasswordHash(ctx context.Context, userID uuid.UUID, hash string, mustChange bool) error
	ListWithFilters(ctx context.Context, search string, role string, status string, limit int, offset int) ([]User, int, error)
	UpdateRole(ctx context.Context, userID uuid.UUID, role string) error
	UpdateDisabled(ctx context.Context, userID uuid.UUID, disabled bool) error
	SetMustChangePassword(ctx context.Context, userID uuid.UUID, mustChange bool) error
}

// SessionRepository defines persistence operations for refresh-token sessions.
type SessionRepository interface {
	Create(ctx context.Context, session *Session) error
	FindByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteAllForUser(ctx context.Context, userID uuid.UUID) error
	DeleteExpired(ctx context.Context) (int64, error)
}

// APIToken represents a per-user API token stored in PostgreSQL.
type APIToken struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	TokenHash  string     `json:"-"`
	Label      string     `json:"label"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
}

// TokenStatus returns "active", "revoked", or "expired".
func (t APIToken) TokenStatus() string {
	if t.RevokedAt != nil {
		return "revoked"
	}
	if time.Now().After(t.ExpiresAt) {
		return "expired"
	}
	return "active"
}

// APITokenRepository defines persistence operations for API tokens.
type APITokenRepository interface {
	Create(ctx context.Context, token *APIToken) error
	FindByTokenHash(ctx context.Context, tokenHash string) (*APIToken, error)
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]APIToken, error)
	CountActiveByUserID(ctx context.Context, userID uuid.UUID) (int, error)
	Revoke(ctx context.Context, id uuid.UUID, userID uuid.UUID) error
	UpdateLastUsedAt(ctx context.Context, id uuid.UUID, t time.Time) error
}

// RateLimitResult holds the outcome of a rate limit check.
type RateLimitResult struct {
	Allowed   bool
	Limit     int
	Remaining int
	ResetAt   time.Time
}

// RateLimitRepository defines persistence operations for rate limit counters.
type RateLimitRepository interface {
	// CheckAndIncrement atomically inserts a request record and returns the
	// count within the current window. Returns the count and any error.
	CheckAndIncrement(ctx context.Context, tokenID *uuid.UUID, userID *uuid.UUID, ip string, window time.Duration) (count int, err error)
	// CheckIPAndIncrement returns the request count for an IP within the window.
	CheckIPAndIncrement(ctx context.Context, ip string, window time.Duration) (count int, err error)
	// PruneExpired removes rows older than the given window. Called periodically.
	PruneExpired(ctx context.Context, window time.Duration) (int64, error)
}
