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
}

// SessionRepository defines persistence operations for refresh-token sessions.
type SessionRepository interface {
	Create(ctx context.Context, session *Session) error
	FindByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteAllForUser(ctx context.Context, userID uuid.UUID) error
	DeleteExpired(ctx context.Context) (int64, error)
}
