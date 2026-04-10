package auth

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Validation errors returned by the input validators.
var (
	ErrPasswordTooShort  = errors.New("password must be at least 12 characters")
	ErrPasswordTooLong   = errors.New("password must not exceed 128 characters")
	ErrPasswordNoUpper   = errors.New("password must contain at least one uppercase letter")
	ErrPasswordNoLower   = errors.New("password must contain at least one lowercase letter")
	ErrPasswordNoDigit   = errors.New("password must contain at least one digit")
	ErrPasswordNoSpecial = errors.New("password must contain at least one special character")
	ErrInvalidEmail      = errors.New("invalid email format")
	ErrNameTooLong       = errors.New("name must not exceed 256 characters")
)

// emailRegex is a standard email format check. It covers the vast majority of
// valid addresses without trying to implement the full RFC 5322 grammar.
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// ValidatePassword checks that password meets complexity requirements:
//   - Length between 12 and 128 characters inclusive
//   - At least one uppercase letter
//   - At least one lowercase letter
//   - At least one digit
//   - At least one special character (non-alphanumeric, non-space)
func ValidatePassword(password string) error {
	if len(password) < 12 {
		return ErrPasswordTooShort
	}
	if len(password) > 128 {
		return ErrPasswordTooLong
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r):
			hasSpecial = true
		}
	}

	var missing []string
	if !hasUpper {
		missing = append(missing, ErrPasswordNoUpper.Error())
	}
	if !hasLower {
		missing = append(missing, ErrPasswordNoLower.Error())
	}
	if !hasDigit {
		missing = append(missing, ErrPasswordNoDigit.Error())
	}
	if !hasSpecial {
		missing = append(missing, ErrPasswordNoSpecial.Error())
	}

	if len(missing) > 0 {
		return fmt.Errorf("password does not meet complexity requirements: %s", strings.Join(missing, "; "))
	}
	return nil
}

// ValidateEmail checks that email matches a standard email format.
func ValidateEmail(email string) error {
	if !emailRegex.MatchString(email) {
		return ErrInvalidEmail
	}
	return nil
}

// ValidateName checks that name does not exceed 256 characters.
func ValidateName(name string) error {
	if len(name) > 256 {
		return ErrNameTooLong
	}
	return nil
}

// TrimInput trims leading and trailing whitespace from s.
func TrimInput(s string) string {
	return strings.TrimSpace(s)
}
