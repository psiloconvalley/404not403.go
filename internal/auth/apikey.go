package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// GenerateAPIKey creates a cryptographically secure random API key.
// Returns the raw key (shown once to user) and the hash (stored in DB).
func GenerateAPIKey() (raw string, hash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("failed to generate API key: %w", err)
	}

	raw = hex.EncodeToString(bytes)
	hash = HashAPIKey(raw)

	return raw, hash, nil
}

// HashAPIKey returns the SHA256 hash of an API key.
// Used to look up keys in the database without storing plaintext.
func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
