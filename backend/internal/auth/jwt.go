package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Default access token expiry when none is specified.
const DefaultAccessTokenExpiry = 15 * time.Minute

var (
	ErrInvalidToken         = errors.New("invalid or expired token")
	ErrInvalidSigningMethod = errors.New("unexpected signing method")
	ErrMissingClaims        = errors.New("token missing required claims")
)

// IssueAccessToken creates a signed JWT access token for the given user.
// The token contains sub, email, role, must_change_password, iat, and exp claims,
// signed with HMAC-SHA256.
func IssueAccessToken(user User, secret []byte, expiry time.Duration) (string, error) {
	if expiry <= 0 {
		expiry = DefaultAccessTokenExpiry
	}

	now := time.Now()

	claims := Claims{
		Sub:                user.ID.String(),
		Email:              user.Email,
		Role:               user.Role,
		MustChangePassword: user.MustChangePassword,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// ValidateAccessToken parses and validates a JWT access token string.
// It verifies the HMAC-SHA256 signature and checks that all required claims
// (sub, email, role) are present. Returns the parsed Claims or an error.
func ValidateAccessToken(tokenString string, secret []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSigningMethod, token.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	// Reject tokens missing required claims.
	if claims.Sub == "" || claims.Email == "" || claims.Role == "" {
		return nil, ErrMissingClaims
	}

	return claims, nil
}
