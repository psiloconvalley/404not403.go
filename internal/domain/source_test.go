package domain

import "testing"

func TestSourceType_IsValid(t *testing.T) {
	for _, s := range AllSourceTypes() {
		if !s.IsValid() {
			t.Errorf("expected %q to be valid", s)
		}
	}

	invalid := []SourceType{"sms", "phone", "carrier_pigeon", ""}
	for _, s := range invalid {
		if s.IsValid() {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestSourceType_IsExternal(t *testing.T) {
	external := []SourceType{SourceEmail, SourceSlack, SourceWeb, SourceAPI}
	for _, s := range external {
		if !s.IsExternal() {
			t.Errorf("expected %q to be external", s)
		}
	}

	internal := []SourceType{SourceApp, SourceSystem}
	for _, s := range internal {
		if s.IsExternal() {
			t.Errorf("expected %q to be internal", s)
		}
	}
}

func TestParseSourceType_Valid(t *testing.T) {
	s, err := ParseSourceType("email")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != SourceEmail {
		t.Errorf("expected email, got %q", s)
	}
}

func TestParseSourceType_Invalid(t *testing.T) {
	_, err := ParseSourceType("fax")
	if err == nil {
		t.Error("expected error for invalid source type")
	}
}
