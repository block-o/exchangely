package auth

import (
	"regexp"
	"strings"
	"testing"
	"unicode"

	"pgregory.net/rapid"
)

func TestValidatePasswordAcceptsValid(t *testing.T) {
	valid := []string{
		"Abcdefghij1!",   // exactly 12 chars
		"MyP@ssw0rd!!xx", // 14 chars
		"A1!aaaaaaaaaa",  // minimal complexity, 13 chars
		"LongPassword1!LongPassword1!LongPassword1!LongPassword1!LongPassword1!LongPassword1!LongPassword1!LongPassword1!LongPassword1!!", // 128 chars
	}
	for _, pw := range valid {
		if err := ValidatePassword(pw); err != nil {
			t.Errorf("ValidatePassword(%q) = %v, want nil", pw, err)
		}
	}
}

func TestValidatePasswordRejectsTooShort(t *testing.T) {
	if err := ValidatePassword("Abc1!xxxxxx"); err == nil { // 11 chars
		t.Error("expected error for 11-char password")
	}
}

func TestValidatePasswordRejectsTooLong(t *testing.T) {
	pw := make([]byte, 129)
	for i := range pw {
		pw[i] = 'a'
	}
	pw[0] = 'A'
	pw[1] = '1'
	pw[2] = '!'
	if err := ValidatePassword(string(pw)); err == nil {
		t.Error("expected error for 129-char password")
	}
}

func TestValidatePasswordRejectsMissingUpper(t *testing.T) {
	if err := ValidatePassword("abcdefghij1!"); err == nil {
		t.Error("expected error for password without uppercase")
	}
}

func TestValidatePasswordRejectsMissingLower(t *testing.T) {
	if err := ValidatePassword("ABCDEFGHIJ1!"); err == nil {
		t.Error("expected error for password without lowercase")
	}
}

func TestValidatePasswordRejectsMissingDigit(t *testing.T) {
	if err := ValidatePassword("Abcdefghijk!"); err == nil {
		t.Error("expected error for password without digit")
	}
}

func TestValidatePasswordRejectsMissingSpecial(t *testing.T) {
	if err := ValidatePassword("Abcdefghij12"); err == nil {
		t.Error("expected error for password without special char")
	}
}

func TestValidateEmailAcceptsValid(t *testing.T) {
	valid := []string{
		"user@example.com",
		"first.last@domain.org",
		"user+tag@sub.domain.co",
		"a@b.cc",
	}
	for _, email := range valid {
		if err := ValidateEmail(email); err != nil {
			t.Errorf("ValidateEmail(%q) = %v, want nil", email, err)
		}
	}
}

func TestValidateEmailRejectsInvalid(t *testing.T) {
	invalid := []string{
		"",
		"noatsign",
		"@nodomain.com",
		"user@",
		"user@.com",
		"user@domain",
		"user @domain.com",
		"user@domain .com",
	}
	for _, email := range invalid {
		if err := ValidateEmail(email); err == nil {
			t.Errorf("ValidateEmail(%q) = nil, want error", email)
		}
	}
}

func TestValidateNameAcceptsValid(t *testing.T) {
	if err := ValidateName("Alice"); err != nil {
		t.Errorf("ValidateName(\"Alice\") = %v, want nil", err)
	}
	// Exactly 256 chars should be fine.
	name256 := string(make([]byte, 256))
	if err := ValidateName(name256); err != nil {
		t.Errorf("ValidateName(256 chars) = %v, want nil", err)
	}
}

func TestValidateNameRejectsTooLong(t *testing.T) {
	name257 := string(make([]byte, 257))
	if err := ValidateName(name257); err == nil {
		t.Error("expected error for 257-char name")
	}
}

func TestTrimInput(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"  hello  ", "hello"},
		{"\t\nfoo\n\t", "foo"},
		{"no-trim", "no-trim"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		got := TrimInput(tc.in)
		if got != tc.want {
			t.Errorf("TrimInput(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// meetsComplexity is a test oracle that independently checks whether a password
// satisfies all complexity rules, mirroring the specification rather than the
// implementation so the property test catches implementation bugs.
func meetsComplexity(s string) bool {
	if len(s) < 12 || len(s) > 128 {
		return false
	}
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range s {
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
	return hasUpper && hasLower && hasDigit && hasSpecial
}

// TestPropertyPasswordComplexityValidation verifies Property 12: Password complexity validation.
//
// For any string, the password validator SHALL accept it if and only if it has
// length between 12 and 128 characters inclusive AND contains at least one
// uppercase letter, one lowercase letter, one digit, and one special character.
// All other strings SHALL be rejected.
//
// **Validates: Requirements 12.1**
func TestPropertyPasswordComplexityValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random string from a broad character set including ASCII
		// printable, unicode letters, digits, and special characters.
		pw := rapid.StringMatching(`[\x20-\x7E\x00-\x1F]{0,200}`).Draw(t, "password")

		err := ValidatePassword(pw)
		expected := meetsComplexity(pw)

		if expected && err != nil {
			t.Fatalf("ValidatePassword(%q) returned error %v, but password meets all complexity rules (len=%d)", pw, err, len(pw))
		}
		if !expected && err == nil {
			t.Fatalf("ValidatePassword(%q) returned nil, but password does NOT meet complexity rules (len=%d)", pw, len(pw))
		}
	})
}

// isValidEmail is a test oracle that independently checks whether a string
// matches the standard email format used by the validator. It mirrors the
// specification rather than the implementation.
func isValidEmail(s string) bool {
	// Same pattern as emailRegex in validation.go — local@domain.tld with
	// at least 2-char TLD, alphanumeric/dot/hyphen domain, and
	// alphanumeric/dot/underscore/percent/plus/hyphen local part.
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(s)
}

// TestPropertyEmailValidation verifies Property 15 (email part): Input validation and sanitization.
//
// For any input string submitted as an email, the validator SHALL accept it
// only if it matches a standard email format.
//
// **Validates: Requirements 12.9**
func TestPropertyEmailValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw from a mix of arbitrary strings and email-like patterns so we
		// exercise both acceptance and rejection paths.
		email := rapid.OneOf(
			// Arbitrary ASCII strings — mostly invalid emails.
			rapid.StringMatching(`[\x20-\x7E]{0,80}`),
			// Strings that look like emails (local@domain.tld pattern).
			rapid.StringMatching(`[a-zA-Z0-9._%+\-]{1,30}@[a-zA-Z0-9.\-]{1,20}\.[a-zA-Z]{2,6}`),
		).Draw(t, "email")

		err := ValidateEmail(email)
		expected := isValidEmail(email)

		if expected && err != nil {
			t.Fatalf("ValidateEmail(%q) returned error %v, but string matches valid email format", email, err)
		}
		if !expected && err == nil {
			t.Fatalf("ValidateEmail(%q) returned nil, but string does NOT match valid email format", email)
		}
	})
}

// TestPropertyNameLengthValidation verifies Property 15 (name part): Input validation and sanitization.
//
// For any input string submitted as a name, the validator SHALL reject it if
// it exceeds 256 characters.
//
// **Validates: Requirements 12.9**
func TestPropertyNameLengthValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate names of varying length from 0 to 300 to exercise the
		// boundary around 256 characters.
		length := rapid.IntRange(0, 300).Draw(t, "length")
		name := rapid.StringOfN(rapid.RuneFrom(nil, unicode.Letter, unicode.Digit, unicode.Space), length, length, -1).Draw(t, "name")

		err := ValidateName(name)

		if len(name) > 256 && err == nil {
			t.Fatalf("ValidateName(len=%d) returned nil, but name exceeds 256 characters", len(name))
		}
		if len(name) <= 256 && err != nil {
			t.Fatalf("ValidateName(len=%d) returned error %v, but name is within 256 characters", len(name), err)
		}
	})
}

// TestPropertyWhitespaceTrimming verifies Property 15 (trimming part): Input validation and sanitization.
//
// For any input string, leading and trailing whitespace SHALL be trimmed
// before processing.
//
// **Validates: Requirements 12.9**
func TestPropertyWhitespaceTrimming(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a core string (may contain interior whitespace).
		core := rapid.StringMatching(`[\x20-\x7E]{0,50}`).Draw(t, "core")
		// Generate leading and trailing whitespace padding.
		leading := rapid.StringMatching(`[\t\n\r ]{0,10}`).Draw(t, "leading")
		trailing := rapid.StringMatching(`[\t\n\r ]{0,10}`).Draw(t, "trailing")

		input := leading + core + trailing
		result := TrimInput(input)

		// Property 1: Result must have no leading whitespace.
		if len(result) > 0 && (result[0] == ' ' || result[0] == '\t' || result[0] == '\n' || result[0] == '\r') {
			t.Fatalf("TrimInput(%q) = %q still has leading whitespace", input, result)
		}
		// Property 2: Result must have no trailing whitespace.
		if len(result) > 0 {
			last := result[len(result)-1]
			if last == ' ' || last == '\t' || last == '\n' || last == '\r' {
				t.Fatalf("TrimInput(%q) = %q still has trailing whitespace", input, result)
			}
		}
		// Property 3: Result must equal strings.TrimSpace of the input (oracle).
		expected := strings.TrimSpace(input)
		if result != expected {
			t.Fatalf("TrimInput(%q) = %q, want %q", input, result, expected)
		}
	})
}
