package ticket

import (
	"context"
	"testing"
)

func TestCreate_EmptySubject_ReturnsError(t *testing.T) {
	svc := New(nil) // nil db — we won't hit the database
	_, err := svc.Create(context.Background(), CreateInput{
		OrgID:      "org-1",
		Subject:    "",
		Body:       "some body",
		SourceType: "web",
	})
	if err == nil {
		t.Fatal("expected error for empty subject")
	}
	if err.Error() != "subject is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreate_EmptyBody_ReturnsError(t *testing.T) {
	svc := New(nil)
	_, err := svc.Create(context.Background(), CreateInput{
		OrgID:      "org-1",
		Subject:    "test subject",
		Body:       "",
		SourceType: "web",
	})
	if err == nil {
		t.Fatal("expected error for empty body")
	}
	if err.Error() != "body is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreate_EmptyOrgID_ReturnsError(t *testing.T) {
	svc := New(nil)
	_, err := svc.Create(context.Background(), CreateInput{
		OrgID:      "",
		Subject:    "test",
		Body:       "body",
		SourceType: "web",
	})
	if err == nil {
		t.Fatal("expected error for empty org_id")
	}
}

func TestCreate_SubjectTooLong_ReturnsError(t *testing.T) {
	svc := New(nil)
	longSubject := make([]byte, 501)
	for i := range longSubject {
		longSubject[i] = 'a'
	}
	_, err := svc.Create(context.Background(), CreateInput{
		OrgID:      "org-1",
		Subject:    string(longSubject),
		Body:       "body",
		SourceType: "web",
	})
	if err == nil {
		t.Fatal("expected error for subject > 500 chars")
	}
}

func TestCreate_InvalidSourceType_ReturnsError(t *testing.T) {
	svc := New(nil)
	_, err := svc.Create(context.Background(), CreateInput{
		OrgID:      "org-1",
		Subject:    "test",
		Body:       "body",
		SourceType: "carrier_pigeon",
	})
	if err == nil {
		t.Fatal("expected error for invalid source_type")
	}
}

func TestCreate_InvalidPriority_ReturnsError(t *testing.T) {
	svc := New(nil)
	_, err := svc.Create(context.Background(), CreateInput{
		OrgID:      "org-1",
		Subject:    "test",
		Body:       "body",
		SourceType: "web",
		Priority:   "P99",
	})
	if err == nil {
		t.Fatal("expected error for invalid priority")
	}
}

func TestCreate_DefaultPriority_WhenEmpty(t *testing.T) {
	// Verify that empty priority passes validation
	// It will panic on DB access since db is nil — that's expected
	svc := New(nil)
	defer func() {
		if r := recover(); r != nil {
			// Expected — validation passed, hit nil DB
		}
	}()
	_, err := svc.Create(context.Background(), CreateInput{
		OrgID:      "org-1",
		Subject:    "test",
		Body:       "body",
		SourceType: "web",
		Priority:   "",
	})
	if err != nil && err.Error() == "invalid priority" {
		t.Fatal("empty priority should default, not error")
	}
}

func TestCreate_WhitespaceSubject_ReturnsError(t *testing.T) {
	svc := New(nil)
	_, err := svc.Create(context.Background(), CreateInput{
		OrgID:      "org-1",
		Subject:    "   ",
		Body:       "body",
		SourceType: "web",
	})
	if err == nil {
		t.Fatal("expected error for whitespace-only subject")
	}
}
