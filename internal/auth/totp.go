package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// GenerateTOTPSecret creates a new TOTP secret for a user.
// Returns the secret (for QR code display) and the encrypted version (for DB storage).
func GenerateTOTPSecret(handle string) (secret string, encrypted string, qrURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "404NOT403",
		AccountName: handle,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate TOTP: %w", err)
	}

	secret = key.Secret()

	encrypted, err = encryptSecret(secret)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to encrypt TOTP secret: %w", err)
	}

	qrURL = key.URL()

	return secret, encrypted, qrURL, nil
}

// VerifyTOTP checks a 6-digit code against an encrypted TOTP secret.
func VerifyTOTP(encryptedSecret, code string) (bool, error) {
	secret, err := decryptSecret(encryptedSecret)
	if err != nil {
		return false, fmt.Errorf("failed to decrypt TOTP secret: %w", err)
	}

	valid := totp.Validate(code, secret)
	return valid, nil
}

// ValidateTOTPCode validates a code at a specific time (for testing).
func ValidateTOTPCode(encryptedSecret, code string, t time.Time) (bool, error) {
	secret, err := decryptSecret(encryptedSecret)
	if err != nil {
		return false, err
	}

	valid, err := totp.ValidateCustom(code, secret, t, totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return valid, err
}

// ── AES-256-GCM encryption for TOTP secrets at rest ──────────────────────────

func getEncryptionKey() ([]byte, error) {
	keyHex := os.Getenv("MFA_ENCRYPTION_KEY")
	if keyHex == "" {
		return nil, fmt.Errorf("MFA_ENCRYPTION_KEY not set")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid MFA_ENCRYPTION_KEY: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("MFA_ENCRYPTION_KEY must be 32 bytes (64 hex chars)")
	}
	return key, nil
}

func encryptSecret(plaintext string) (string, error) {
	key, err := getEncryptionKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

func decryptSecret(ciphertextHex string) (string, error) {
	key, err := getEncryptionKey()
	if err != nil {
		return "", err
	}

	ciphertext, err := hex.DecodeString(ciphertextHex)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce := ciphertext[:gcm.NonceSize()]
	ciphertext = ciphertext[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
