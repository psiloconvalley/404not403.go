package domain

import "testing"

func TestEventType_IsValid(t *testing.T) {
	for _, e := range AllEventTypes() {
		if !e.IsValid() {
			t.Errorf("expected %q to be valid", e)
		}
	}

	invalid := []EventType{"ticket.deleted", "user.created", ""}
	for _, e := range invalid {
		if e.IsValid() {
			t.Errorf("expected %q to be invalid", e)
		}
	}
}

func TestActorType_IsValid(t *testing.T) {
	valid := []ActorType{ActorUser, ActorSystem, ActorAI, ActorWebhook}
	for _, a := range valid {
		if !a.IsValid() {
			t.Errorf("expected %q to be valid", a)
		}
	}

	if ActorType("robot").IsValid() {
		t.Error("expected 'robot' to be invalid actor type")
	}
}

func TestParseEventType_Valid(t *testing.T) {
	e, err := ParseEventType("ticket.created")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e != EventTicketCreated {
		t.Errorf("expected ticket.created, got %q", e)
	}
}

func TestParseEventType_Invalid(t *testing.T) {
	_, err := ParseEventType("ticket.exploded")
	if err == nil {
		t.Error("expected error for invalid event type")
	}
}
