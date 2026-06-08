package store

import (
	"encoding/json"
	"testing"
)

// ── Error Type Tests ────────────────────────────────────────────────────────

func TestStoreError_MonitorLimit_Message(t *testing.T) {
	err := ErrMonitorLimitReached
	expected := "monitor limit reached — maximum 10 active monitors per user"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestStoreError_NotOwner_Message(t *testing.T) {
	err := ErrNotOwner
	expected := "you do not own this resource"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestStoreError_ImplementsErrorInterface(t *testing.T) {
	var err error = ErrMonitorLimitReached
	if err == nil {
		t.Error("ErrMonitorLimitReached should implement error interface")
	}
}

// ── nullableInt Tests ───────────────────────────────────────────────────────

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

// ── Type Serialization Tests ────────────────────────────────────────────────

func TestMonitor_JSONOmitsNilPointers(t *testing.T) {
	m := Monitor{
		ID:            "test-id",
		URL:           "https://example.com",
		CheckInterval: "24h",
		Active:        true,
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("failed to marshal Monitor: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded["url"] != "https://example.com" {
		t.Errorf("expected url to be https://example.com, got %v", decoded["url"])
	}

	if decoded["id"] != "test-id" {
		t.Errorf("expected id to be test-id, got %v", decoded["id"])
	}
}

func TestUser_PasswordHashOmittedFromJSON(t *testing.T) {
	u := User{
		ID:           "user-1",
		Email:        "test@example.com",
		Handle:       "testuser",
		PasswordHash: "secret-hash-value",
		Role:         "observer",
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

func TestChange_JSONContainsAllFields(t *testing.T) {
	c := Change{
		ID:        "change-1",
		MonitorID: "mon-1",
		URL:       "https://example.com",
		OldStatus: 200,
		NewStatus: 404,
		OldHash:   "abc123",
		NewHash:   "def456",
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("failed to marshal Change: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded["old_status"].(float64) != 200 {
		t.Errorf("expected old_status 200, got %v", decoded["old_status"])
	}
	if decoded["new_status"].(float64) != 404 {
		t.Errorf("expected new_status 404, got %v", decoded["new_status"])
	}
}

// ── Helper ──────────────────────────────────────────────────────────────────

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
