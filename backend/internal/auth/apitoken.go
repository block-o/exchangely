package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// APITokenService errors.
var (
	ErrTokenLimitReached = errors.New("token limit reached")
	ErrTokenNotFound     = errors.New("not found")
	ErrTokenRevoked      = errors.New("token revoked")
	ErrTokenExpired      = errors.New("token expired")
	ErrTokenUnauthorized = errors.New("unauthorized")
)

// apiTokenBytes is the number of random bytes used to generate an API token.
const apiTokenBytes = 32

// apiTokenPrefix is prepended to every raw API token.
const apiTokenPrefix = "exly_"

// APITokenConfig holds configuration for the API token service.
type APITokenConfig struct {
	MaxTokensPerUser int
	TokenExpiry      time.Duration
}

// DefaultAPITokenConfig returns the default configuration.
func DefaultAPITokenConfig() APITokenConfig {
	return APITokenConfig{
		MaxTokensPerUser: 5,
		TokenExpiry:      90 * 24 * time.Hour, // 90 days
	}
}

// APITokenService manages API token CRUD operations.
type APITokenService struct {
	tokens APITokenRepository
	users  UserRepository
	cfg    APITokenConfig
}

// NewAPITokenService creates a new APITokenService.
func NewAPITokenService(tokens APITokenRepository, users UserRepository, cfg APITokenConfig) *APITokenService {
	return &APITokenService{
		tokens: tokens,
		users:  users,
		cfg:    cfg,
	}
}

// CreateToken generates a new API token for the given user. It returns the raw
// token (shown exactly once) and the persisted APIToken record. The raw token
// is prefixed with "exly_" and the SHA-256 hash is stored in the database.
func (s *APITokenService) CreateToken(ctx context.Context, userID uuid.UUID, label string) (string, APIToken, error) {
	// Enforce max active tokens per user.
	count, err := s.tokens.CountActiveByUserID(ctx, userID)
	if err != nil {
		return "", APIToken{}, fmt.Errorf("counting active tokens: %w", err)
	}
	if count >= s.cfg.MaxTokensPerUser {
		return "", APIToken{}, ErrTokenLimitReached
	}

	// Generate random bytes and prefix with exly_.
	raw, err := generateRandomToken(apiTokenBytes)
	if err != nil {
		return "", APIToken{}, fmt.Errorf("generating token: %w", err)
	}
	rawToken := apiTokenPrefix + raw

	now := time.Now()
	token := APIToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: hashToken(rawToken),
		Label:     label,
		Prefix:    rawToken[:8],
		CreatedAt: now,
		ExpiresAt: now.Add(s.cfg.TokenExpiry),
	}

	if err := s.tokens.Create(ctx, &token); err != nil {
		return "", APIToken{}, fmt.Errorf("storing token: %w", err)
	}

	slog.Info("auth event",
		"event", "api_token_created",
		"user_id", userID.String(),
		"token_id", token.ID.String(),
		"label", label,
	)

	return rawToken, token, nil
}

// ListTokens returns all tokens for the given user ordered by created_at desc.
func (s *APITokenService) ListTokens(ctx context.Context, userID uuid.UUID) ([]APIToken, error) {
	tokens, err := s.tokens.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing tokens: %w", err)
	}
	return tokens, nil
}

// RevokeToken revokes a token by ID, enforcing user ownership. Returns
// ErrTokenNotFound if the token doesn't belong to the user. Idempotent
// on already-revoked tokens.
func (s *APITokenService) RevokeToken(ctx context.Context, tokenID uuid.UUID, userID uuid.UUID) error {
	if err := s.tokens.Revoke(ctx, tokenID, userID); err != nil {
		return err
	}

	slog.Info("auth event",
		"event", "api_token_revoked",
		"user_id", userID.String(),
		"token_id", tokenID.String(),
	)

	return nil
}

// ValidateToken hashes the raw token, looks it up by hash, rejects revoked or
// expired tokens, and returns the APIToken and associated User.
func (s *APITokenService) ValidateToken(ctx context.Context, rawToken string) (*APIToken, *User, error) {
	tokenHash := hashToken(rawToken)

	token, err := s.tokens.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, nil, fmt.Errorf("finding token: %w", err)
	}
	if token == nil {
		return nil, nil, ErrTokenUnauthorized
	}

	if token.RevokedAt != nil {
		return nil, nil, ErrTokenUnauthorized
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, nil, ErrTokenUnauthorized
	}

	user, err := s.users.FindByID(ctx, token.UserID)
	if err != nil {
		return nil, nil, fmt.Errorf("finding user: %w", err)
	}
	if user == nil {
		return nil, nil, ErrTokenUnauthorized
	}

	return token, user, nil
}

// TouchLastUsed fires an async goroutine to update the token's last_used_at
// timestamp, keeping it off the request hot path.
func (s *APITokenService) TouchLastUsed(ctx context.Context, tokenID uuid.UUID) {
	go func() {
		if err := s.tokens.UpdateLastUsedAt(context.Background(), tokenID, time.Now()); err != nil {
			slog.Warn("failed to update token last_used_at",
				"token_id", tokenID.String(),
				"error", err,
			)
		}
	}()
}
