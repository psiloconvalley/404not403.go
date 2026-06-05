package auth

import (
	"testing"
)

func TestGenerateAPIKey_ReturnsKeyAndHash(t *testing.T) {
	raw, hash, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}

	if raw == "" {
		t.Error("raw key should not be empty")
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}

	// Raw key should be 64 hex chars (32 bytes)
	if len(raw) != 64 {
		t.Errorf("raw key should be 64 chars, got %d", len(raw))
	}

	// Hash should be 64 hex chars (SHA256)
	if len(hash) != 64 {
		t.Errorf("hash should be 64 chars, got %d", len(hash))
	}
}

func TestGenerateAPIKey_UniqueKeys(t *testing.T) {
	raw1, _, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey 1 failed: %v", err)
	}

	raw2, _, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey 2 failed: %v", err)
	}

	if raw1 == raw2 {
		t.Error("two generated keys should be different")
	}
}

func TestHashAPIKey_Deterministic(t *testing.T) {
	key := "abc123def456abc123def456abc123def456abc123def456abc123def456abcd"

	hash1 := HashAPIKey(key)
	hash2 := HashAPIKey(key)

	if hash1 != hash2 {
		t.Error("hashing the same key twice should produce the same hash")
	}
}

func TestHashAPIKey_DifferentKeys(t *testing.T) {
	hash1 := HashAPIKey("key-one-abcdef1234567890abcdef1234567890abcdef1234567890abcdef12")
	hash2 := HashAPIKey("key-two-abcdef1234567890abcdef1234567890abcdef1234567890abcdef12")

	if hash1 == hash2 {
		t.Error("different keys should produce different hashes")
	}
}
