package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters — tuned for security + performance balance
// These match OWASP recommendations for 2024+
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64MB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword generates an Argon2id hash from a plaintext password.
// Returns a self-contained string: $argon2id$v=19$m=65536,t=3,p=4$salt$hash
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		argonTime,
		argonMemory,
		argonThreads,
		argonKeyLen,
	)

	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		saltB64, hashB64,
	), nil
}

// VerifyPassword checks a plaintext password against an Argon2id hash.
// Uses constant-time comparison to prevent timing attacks.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid hash format")
	}

	var memory uint32
	var time uint32
	var threads uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads)
	if err != nil {
		return false, fmt.Errorf("invalid hash parameters: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("invalid salt encoding: %w", err)
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("invalid hash encoding: %w", err)
	}

	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		time,
		memory,
		threads,
		uint32(len(expectedHash)),
	)

	// Constant-time comparison prevents timing attacks
	if subtle.ConstantTimeCompare(computedHash, expectedHash) == 1 {
		return true, nil
	}

	return false, nil
}
