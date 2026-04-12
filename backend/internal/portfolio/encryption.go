package portfolio

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/google/uuid"
	"golang.org/x/crypto/hkdf"
)

var (
	ErrInvalidMasterKey = errors.New("master key must be 64 hex characters (32 bytes)")
	ErrCiphertextTamper = errors.New("ciphertext tamper detected: GCM authentication failed")
	ErrDecryptionFailed = errors.New("decryption failed")
)

// KeyEncryptionService provides AES-256-GCM encryption with per-user key
// derivation via HKDF-SHA256 from a server-side master key.
type KeyEncryptionService struct {
	masterKey []byte // 32 bytes
}

// NewKeyEncryptionService creates a new encryption service from a hex-encoded
// 32-byte master key. Returns an error if the key is not exactly 64 hex chars.
func NewKeyEncryptionService(masterKeyHex string) (*KeyEncryptionService, error) {
	decoded, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid hex encoding: %v", ErrInvalidMasterKey, err)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("%w: got %d bytes", ErrInvalidMasterKey, len(decoded))
	}
	return &KeyEncryptionService{masterKey: decoded}, nil
}

// DeriveUserKey derives a 32-byte AES-256 key for a specific user using
// HKDF-SHA256 with the master key as input keying material and the user ID
// as the info parameter.
func (s *KeyEncryptionService) DeriveUserKey(userID uuid.UUID) ([]byte, error) {
	hkdfReader := hkdf.New(sha256.New, s.masterKey, nil, userID[:])
	derived := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, derived); err != nil {
		return nil, fmt.Errorf("key derivation failed for user %s: %w", userID, err)
	}
	return derived, nil
}

// Encrypt encrypts plaintext using AES-256-GCM with the provided key and a
// random nonce. Returns the ciphertext and nonce.
func (s *KeyEncryptionService) Encrypt(key, plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM with the provided key and
// nonce. Returns a tamper-detected error if the GCM authentication tag fails.
func (s *KeyEncryptionService) Decrypt(key, ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		slog.Warn("GCM authentication tag verification failed",
			"error", err,
			"nonce_len", len(nonce),
			"ciphertext_len", len(ciphertext),
		)
		return nil, ErrCiphertextTamper
	}

	return plaintext, nil
}

// EncryptForUser derives the user-specific key and encrypts the plaintext.
func (s *KeyEncryptionService) EncryptForUser(userID uuid.UUID, plaintext []byte) (ciphertext, nonce []byte, err error) {
	key, err := s.DeriveUserKey(userID)
	if err != nil {
		return nil, nil, err
	}
	return s.Encrypt(key, plaintext)
}

// DecryptForUser derives the user-specific key and decrypts the ciphertext.
func (s *KeyEncryptionService) DecryptForUser(userID uuid.UUID, ciphertext, nonce []byte) ([]byte, error) {
	key, err := s.DeriveUserKey(userID)
	if err != nil {
		return nil, err
	}
	return s.Decrypt(key, ciphertext, nonce)
}
