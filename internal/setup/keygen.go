package setup

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateAPIKey generates a cryptographically random 64-character hex string API key.
func GenerateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
