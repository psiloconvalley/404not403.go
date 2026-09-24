package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestComputeEventHash_Deterministic(t *testing.T) {
	prev := GenesisHash
	ticketID := "ticket-123"
	orgID := "org-abc"
	actorID := "user-1"
	actorType := "user"
	eventType := "ticket.created"
	payload := json.RawMessage(`{"source":"email","priority":"P1"}`)
	createdAt := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)

	hash1 := ComputeEventHash(prev, ticketID, orgID, &actorID, actorType, eventType, payload, createdAt)
	hash2 := ComputeEventHash(prev, ticketID, orgID, &actorID, actorType, eventType, payload, createdAt)

	if hash1 != hash2 {
		t.Fatalf("expected deterministic hash, got %s and %s", hash1, hash2)
	}

	if len(hash1) != 64 {
		t.Fatalf("expected SHA-256 64-char hex string, got length %d", len(hash1))
	}
}

func TestComputeEventHash_TamperDetection(t *testing.T) {
	prev := GenesisHash
	ticketID := "ticket-123"
	orgID := "org-abc"
	actorID := "user-1"
	actorType := "user"
	eventType := "ticket.created"
	payload := json.RawMessage(`{"source":"email","priority":"P1"}`)
	createdAt := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)

	originalHash := ComputeEventHash(prev, ticketID, orgID, &actorID, actorType, eventType, payload, createdAt)

	tamperedPayload := json.RawMessage(`{"source":"email","priority":"P0"}`)
	tamperedHash := ComputeEventHash(prev, ticketID, orgID, &actorID, actorType, eventType, tamperedPayload, createdAt)

	if originalHash == tamperedHash {
		t.Fatal("tampering with payload must produce a different cryptographic hash")
	}

	otherActor := "user-2"
	tamperedActorHash := ComputeEventHash(prev, ticketID, orgID, &otherActor, actorType, eventType, payload, createdAt)
	if originalHash == tamperedActorHash {
		t.Fatal("tampering with actor must produce a different cryptographic hash")
	}
}
