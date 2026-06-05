package auth

import (
	"strings"
	"testing"
)

func TestHashPassword_ProducesValidFormat(t *testing.T) {
	hash, err := HashPassword("test-password-123!")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	// Must start with $argon2id$
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("hash does not start with $argon2id$: %s", hash)
	}

	// Must have 6 parts when split by $
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Errorf("expected 6 parts, got %d: %s", len(parts), hash)
	}
}

func TestHashPassword_UniqueSalts(t *testing.T) {
	hash1, err := HashPassword("same-password-12!")
	if err != nil {
		t.Fatalf("HashPassword 1 failed: %v", err)
	}

	hash2, err := HashPassword("same-password-12!")
	if err != nil {
		t.Fatalf("HashPassword 2 failed: %v", err)
	}

	// Same password must produce different hashes (different salts)
	if hash1 == hash2 {
		t.Error("two hashes of the same password should differ due to random salt")
	}
}

func TestVerifyPassword_CorrectPassword(t *testing.T) {
	password := "correct-horse-battery"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	valid, err := VerifyPassword(password, hash)
	if err != nil {
		t.Fatalf("VerifyPassword failed: %v", err)
	}
	if !valid {
		t.Error("correct password should verify as true")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	hash, err := HashPassword("correct-password-1!")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	valid, err := VerifyPassword("wrong-password-999!", hash)
	if err != nil {
		t.Fatalf("VerifyPassword failed: %v", err)
	}
	if valid {
		t.Error("wrong password should verify as false")
	}
}

func TestVerifyPassword_MalformedHash(t *testing.T) {
	_, err := VerifyPassword("anything", "not-a-valid-hash")
	if err == nil {
		t.Error("malformed hash should return an error")
	}
}

func TestVerifyPassword_EmptyHash(t *testing.T) {
	_, err := VerifyPassword("anything", "")
	if err == nil {
		t.Error("empty hash should return an error")
	}
}
