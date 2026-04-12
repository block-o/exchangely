package portfolio

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"pgregory.net/rapid"
)

const testMasterKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func newTestEncryptionService(t interface{ Fatalf(string, ...any) }) *KeyEncryptionService {
	svc, err := NewKeyEncryptionService(testMasterKeyHex)
	if err != nil {
		t.Fatalf("failed to create encryption service: %v", err)
	}
	return svc
}

// Feature: portfolio-tracker, Property 1: Encryption round-trip
//
// For any random byte sequence used as plaintext and any user ID,
// encrypting with EncryptForUser and then decrypting with DecryptForUser
// for the same user ID produces the original plaintext.
func TestPropertyEncryptionRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		svc := newTestEncryptionService(t)

		userID := uuid.New()
		plaintext := rapid.SliceOf(rapid.Byte()).Draw(t, "plaintext")

		ciphertext, nonce, err := svc.EncryptForUser(userID, plaintext)
		if err != nil {
			t.Fatalf("EncryptForUser failed: %v", err)
		}

		decrypted, err := svc.DecryptForUser(userID, ciphertext, nonce)
		if err != nil {
			t.Fatalf("DecryptForUser failed: %v", err)
		}

		if !bytes.Equal(plaintext, decrypted) {
			t.Fatalf("round-trip mismatch: plaintext len=%d, decrypted len=%d", len(plaintext), len(decrypted))
		}
	})
}

// Feature: portfolio-tracker, Property 2: Nonce uniqueness
//
// For any sequence of N encryption operations, all generated nonces are unique.
func TestPropertyNonceUniqueness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		svc := newTestEncryptionService(t)

		userID := uuid.New()
		n := rapid.IntRange(2, 100).Draw(t, "n")
		plaintext := rapid.SliceOfN(rapid.Byte(), 1, 64).Draw(t, "plaintext")

		seen := make(map[string]struct{}, n)
		for i := 0; i < n; i++ {
			_, nonce, err := svc.EncryptForUser(userID, plaintext)
			if err != nil {
				t.Fatalf("EncryptForUser iteration %d failed: %v", i, err)
			}
			key := string(nonce)
			if _, exists := seen[key]; exists {
				t.Fatalf("duplicate nonce found at iteration %d out of %d", i, n)
			}
			seen[key] = struct{}{}
		}
	})
}

// Feature: portfolio-tracker, Property 3: Tampered ciphertext detection
//
// For any valid ciphertext produced by the encryption service, flipping any
// bit in the ciphertext causes decryption to return a tamper-detected error.
func TestPropertyTamperedCiphertextDetection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		svc := newTestEncryptionService(t)

		userID := uuid.New()
		plaintext := rapid.SliceOfN(rapid.Byte(), 1, 256).Draw(t, "plaintext")

		ciphertext, nonce, err := svc.EncryptForUser(userID, plaintext)
		if err != nil {
			t.Fatalf("EncryptForUser failed: %v", err)
		}

		// Flip a random bit in the ciphertext.
		tampered := make([]byte, len(ciphertext))
		copy(tampered, ciphertext)
		byteIdx := rapid.IntRange(0, len(tampered)-1).Draw(t, "byteIdx")
		bitIdx := rapid.IntRange(0, 7).Draw(t, "bitIdx")
		tampered[byteIdx] ^= 1 << uint(bitIdx)

		_, err = svc.DecryptForUser(userID, tampered, nonce)
		if err == nil {
			t.Fatalf("expected decryption error for tampered ciphertext, got nil")
		}
	})
}

// Feature: portfolio-tracker, Property 4: Cross-user key isolation
//
// For any two distinct user IDs and any plaintext, data encrypted for user A
// fails to decrypt when using user B's derived key.
func TestPropertyCrossUserKeyIsolation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		svc := newTestEncryptionService(t)

		userA := uuid.New()
		userB := uuid.New()
		// Ensure distinct user IDs.
		for userA == userB {
			userB = uuid.New()
		}

		plaintext := rapid.SliceOfN(rapid.Byte(), 1, 256).Draw(t, "plaintext")

		ciphertext, nonce, err := svc.EncryptForUser(userA, plaintext)
		if err != nil {
			t.Fatalf("EncryptForUser(userA) failed: %v", err)
		}

		_, err = svc.DecryptForUser(userB, ciphertext, nonce)
		if err == nil {
			t.Fatalf("expected decryption to fail for user B with user A's ciphertext")
		}
	})
}

// Feature: portfolio-tracker, Property 5: Key derivation determinism and uniqueness
//
// For any user ID, calling DeriveUserKey multiple times returns the same key.
// For any two distinct user IDs, their derived keys are different.
func TestPropertyKeyDerivationDeterminismAndUniqueness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		svc := newTestEncryptionService(t)

		userA := uuid.New()
		userB := uuid.New()
		for userA == userB {
			userB = uuid.New()
		}

		// Determinism: same user ID produces same key.
		keyA1, err := svc.DeriveUserKey(userA)
		if err != nil {
			t.Fatalf("DeriveUserKey(userA) call 1 failed: %v", err)
		}
		keyA2, err := svc.DeriveUserKey(userA)
		if err != nil {
			t.Fatalf("DeriveUserKey(userA) call 2 failed: %v", err)
		}
		if !bytes.Equal(keyA1, keyA2) {
			t.Fatalf("DeriveUserKey not deterministic: two calls for same user produced different keys")
		}

		// Uniqueness: distinct user IDs produce distinct keys.
		keyB, err := svc.DeriveUserKey(userB)
		if err != nil {
			t.Fatalf("DeriveUserKey(userB) failed: %v", err)
		}
		if bytes.Equal(keyA1, keyB) {
			t.Fatalf("DeriveUserKey produced identical keys for distinct users %s and %s", userA, userB)
		}
	})
}
