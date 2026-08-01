package store

import (
	"encoding/json"
	"testing"
)

// ── Error Type Tests ──────────────────────────────────────────────────────────

func TestStoreError_NotOwner_Message(t *testing.T) {
	err := ErrNotOwner
	expected := "you do not own this resource"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestStoreError_NotFound_Message(t *testing.T) {
	err := ErrNotFound
	expected := "record not found"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestStoreError_LimitReached_Message(t *testing.T) {
	err := ErrLimitReached
	expected := "tier limit reached"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestStoreError_ImplementsErrorInterface(t *testing.T) {
	var err error = ErrNotOwner
	if err == nil {
		t.Error("ErrNotOwner should implement error interface")
	}
}

// ── nullableInt Tests ─────────────────────────────────────────────────────────

func TestNullableInt_Zero_ReturnsNil(t *testing.T) {
	result := nullableInt(0)
	if result != nil {
		t.Errorf("expected nil for 0, got %v", result)
	}
}

func TestNullableInt_Positive_ReturnsValue(t *testing.T) {
	result := nullableInt(200)
	if result != 200 {
		t.Errorf("expected 200, got %v", result)
	}
}

func TestNullableInt_Negative_ReturnsValue(t *testing.T) {
	result := nullableInt(-1)
	if result != -1 {
		t.Errorf("expected -1, got %v", result)
	}
}

// ── Type Serialization Tests ──────────────────────────────────────────────────

func TestUser_PasswordHashOmittedFromJSON(t *testing.T) {
	u := User{
		ID:           "user-1",
		Email:        "test@example.com",
		Handle:       "testuser",
		PasswordHash: "secret-hash-value",
		Role:         "agent",
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("failed to marshal User: %v", err)
	}

	jsonStr := string(data)
	if contains(jsonStr, "secret-hash-value") {
		t.Error("password hash should not appear in JSON output")
	}
	if contains(jsonStr, "password_hash") {
		t.Error("password_hash key should not appear in JSON output")
	}
}

func TestUser_MFASecretOmittedFromJSON(t *testing.T) {
	secret := "totp-secret-value"
	u := User{
		ID:        "user-1",
		MFASecret: &secret,
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("failed to marshal User: %v", err)
	}

	jsonStr := string(data)
	if contains(jsonStr, "totp-secret-value") {
		t.Error("MFA secret should not appear in JSON output")
	}
}

func TestAPIKey_KeyHashOmittedFromJSON(t *testing.T) {
	k := APIKey{
		ID:      "key-1",
		UserID:  "user-1",
		Name:    "test-key",
		KeyHash: "sha256-hash-value",
		Active:  true,
	}

	data, err := json.Marshal(k)
	if err != nil {
		t.Fatalf("failed to marshal APIKey: %v", err)
	}

	jsonStr := string(data)
	if contains(jsonStr, "sha256-hash-value") {
		t.Error("key hash should not appear in JSON output")
	}
}

// ── Helper ────────────────────────────────────────────────────────────────────

func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
