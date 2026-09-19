// Package secrets provides authenticated encryption for secret values stored
// at rest (Slack bot tokens, API keys, Zoom client secrets, IRC passwords).
//
// The pure-Go SQLite driver does not support SQLCipher, so encryption is done
// per column at the application layer. Ciphertexts carry a version prefix so
// that legacy plaintext rows can be detected and migrated in place.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Prefix marks a value as encrypted with the v1 scheme (AES-256-GCM,
// random 12-byte nonce, base64 raw-std encoded nonce||ciphertext).
const Prefix = "enc:v1:"

// KeyBytes is the required key length: 32 bytes for AES-256.
const KeyBytes = 32

// ErrNoKey is returned when an encrypted value is read but no key is configured.
var ErrNoKey = errors.New("value is encrypted but no database.encryption_key is configured")

// Cipher encrypts and decrypts secret strings. A nil *Cipher is valid and
// passes values through unchanged, so callers can use one unconditionally.
type Cipher struct {
	aead cipher.AEAD
}

// New builds a Cipher from a hex-encoded 32-byte key. An empty key returns a
// nil Cipher, which stores values in plaintext.
func New(hexKey string) (*Cipher, error) {
	hexKey = strings.TrimSpace(hexKey)
	if hexKey == "" {
		return nil, nil
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("decode encryption key: %w", err)
	}
	if len(key) != KeyBytes {
		return nil, fmt.Errorf("encryption key must be %d bytes (%d hex chars), got %d bytes", KeyBytes, KeyBytes*2, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// GenerateKey returns a new random hex-encoded key suitable for New.
func GenerateKey() (string, error) {
	b := make([]byte, KeyBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate encryption key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Enabled reports whether values will actually be encrypted.
func (c *Cipher) Enabled() bool {
	return c != nil
}

// IsEncrypted reports whether a stored value carries the encryption prefix.
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, Prefix)
}

// Seal encrypts plaintext for storage. Empty strings are stored as-is so that
// "no value" remains distinguishable. With a nil Cipher the plaintext is
// returned unchanged.
func (c *Cipher) Seal(plaintext string) (string, error) {
	if c == nil || plaintext == "" {
		return plaintext, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return Prefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

// Open decrypts a stored value. Values without the encryption prefix are
// treated as legacy plaintext and returned unchanged, which lets a database
// written before encryption was enabled keep working until it is migrated.
func (c *Cipher) Open(stored string) (string, error) {
	if !IsEncrypted(stored) {
		return stored, nil
	}
	if c == nil {
		return "", ErrNoKey
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(stored, Prefix))
	if err != nil {
		return "", fmt.Errorf("decode encrypted value: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("decode encrypted value: too short")
	}
	plaintext, err := c.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt value (wrong database.encryption_key?): %w", err)
	}
	return string(plaintext), nil
}

// SealPtr and OpenPtr handle nullable columns.
func (c *Cipher) SealPtr(plaintext *string) (*string, error) {
	if plaintext == nil {
		return nil, nil
	}
	s, err := c.Seal(*plaintext)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Cipher) OpenPtr(stored *string) (*string, error) {
	if stored == nil {
		return nil, nil
	}
	s, err := c.Open(*stored)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
