package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// GenesisHash is the root anchor for the first event of any ticket.
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// ── TicketEvent ───────────────────────────────────────────────────────────────

// TicketEvent is an immutable, cryptographically-chained record of a ticket state change.
// Every event contains the hash of its preceding event, creating a tamper-evident audit ledger.
type TicketEvent struct {
	ID           string          `json:"id"`
	TicketID     string          `json:"ticket_id"`
	OrgID        string          `json:"org_id"`
	ActorUserID  *string         `json:"actor_user_id,omitempty"`
	ActorType    string          `json:"actor_type"`
	EventType    string          `json:"event_type"`
	Payload      json.RawMessage `json:"payload"`
	PreviousHash *string         `json:"previous_hash,omitempty"`
	Hash         *string         `json:"hash,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// ChainBrokenReport contains details on any detected ledger tampering.
type ChainBrokenReport struct {
	EventID      string    `json:"event_id"`
	Sequence     int       `json:"sequence"`
	ExpectedHash string    `json:"expected_hash"`
	ActualHash   string    `json:"actual_hash"`
	PreviousHash string    `json:"previous_hash"`
	CreatedAt    time.Time `json:"created_at"`
	Reason       string    `json:"reason"`
}

// ComputeEventHash generates the SHA-256 hash over an event node.
func ComputeEventHash(prevHash, ticketID, orgID string, actorUserID *string, actorType, eventType string, payload json.RawMessage, createdAt time.Time) string {
	actorIDStr := ""
	if actorUserID != nil {
		actorIDStr = *actorUserID
	}
	canonicalPayload := string(payload)
	if len(payload) == 0 {
		canonicalPayload = "{}"
	}
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s",
		prevHash,
		ticketID,
		orgID,
		actorIDStr,
		actorType,
		eventType,
		canonicalPayload,
		createdAt.UTC().Format(time.RFC3339Nano),
	)
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// ── Queries ───────────────────────────────────────────────────────────────────

// RecordEvent inserts a single event with cryptographic chaining.
func RecordEvent(db *sql.DB, orgID, ticketID string, actorUserID *string, actorType, eventType string, payload json.RawMessage) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := RecordEventTx(tx, orgID, ticketID, actorUserID, actorType, eventType, payload); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordEventTx inserts a cryptographically chained event within an existing transaction.
func RecordEventTx(tx *sql.Tx, orgID, ticketID string, actorUserID *string, actorType, eventType string, payload json.RawMessage) error {
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}

	// 1. Fetch latest hash in chain for this ticket
	var prevHash sql.NullString
	err := tx.QueryRow(`
		SELECT hash
		FROM ticket_events
		WHERE org_id = $1 AND ticket_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1`,
		orgID, ticketID,
	).Scan(&prevHash)

	pHash := GenesisHash
	if err == nil && prevHash.Valid && prevHash.String != "" {
		pHash = prevHash.String
	}

	now := time.Now().UTC()
	hash := ComputeEventHash(pHash, ticketID, orgID, actorUserID, actorType, eventType, payload, now)

	_, err = tx.Exec(`
		INSERT INTO ticket_events (
			ticket_id, org_id, actor_user_id, actor_type,
			event_type, payload, previous_hash, hash, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		ticketID, orgID, actorUserID, actorType,
		eventType, payload, pHash, hash, now,
	)
	return err
}

// ListEventsByTicket returns all events for a ticket in chronological order.
func ListEventsByTicket(db *sql.DB, orgID, ticketID string) ([]TicketEvent, error) {
	rows, err := db.Query(`
		SELECT id, ticket_id, org_id, actor_user_id, actor_type,
		       event_type, payload, previous_hash, hash, created_at
		FROM ticket_events
		WHERE org_id = $1 AND ticket_id = $2
		ORDER BY created_at ASC`,
		orgID, ticketID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []TicketEvent
	for rows.Next() {
		var e TicketEvent
		if err := rows.Scan(
			&e.ID, &e.TicketID, &e.OrgID, &e.ActorUserID,
			&e.ActorType, &e.EventType, &e.Payload,
			&e.PreviousHash, &e.Hash, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// ListEventsByOrg returns recent events across all tickets in an org.
func ListEventsByOrg(db *sql.DB, orgID string, limit int) ([]TicketEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := db.Query(`
		SELECT id, ticket_id, org_id, actor_user_id, actor_type,
		       event_type, payload, previous_hash, hash, created_at
		FROM ticket_events
		WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2`,
		orgID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []TicketEvent
	for rows.Next() {
		var e TicketEvent
		if err := rows.Scan(
			&e.ID, &e.TicketID, &e.OrgID, &e.ActorUserID,
			&e.ActorType, &e.EventType, &e.Payload,
			&e.PreviousHash, &e.Hash, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// VerifyTicketChain verifies the complete cryptographic hash chain for a ticket.
// It returns isValid=true if untouched, or false with the exact broken link report.
func VerifyTicketChain(db *sql.DB, orgID, ticketID string) (bool, *ChainBrokenReport, error) {
	events, err := ListEventsByTicket(db, orgID, ticketID)
	if err != nil {
		return false, nil, err
	}
	if len(events) == 0 {
		return true, nil, nil
	}

	expectedPrev := GenesisHash
	for i, ev := range events {
		// 1. Check previous_hash continuity
		actualPrev := ""
		if ev.PreviousHash != nil {
			actualPrev = *ev.PreviousHash
		}
		if actualPrev != expectedPrev {
			return false, &ChainBrokenReport{
				EventID:      ev.ID,
				Sequence:     i + 1,
				ExpectedHash: expectedPrev,
				ActualHash:   actualPrev,
				PreviousHash: actualPrev,
				CreatedAt:    ev.CreatedAt,
				Reason:       "Previous hash does not match prior block hash in chain",
			}, nil
		}

		// 2. Recompute and verify node hash
		expectedHash := ComputeEventHash(expectedPrev, ev.TicketID, ev.OrgID, ev.ActorUserID, ev.ActorType, ev.EventType, ev.Payload, ev.CreatedAt)
		actualHash := ""
		if ev.Hash != nil {
			actualHash = *ev.Hash
		}
		if actualHash != expectedHash {
			return false, &ChainBrokenReport{
				EventID:      ev.ID,
				Sequence:     i + 1,
				ExpectedHash: expectedHash,
				ActualHash:   actualHash,
				PreviousHash: actualPrev,
				CreatedAt:    ev.CreatedAt,
				Reason:       "Event payload, actor, or timestamp altered — signature invalid",
			}, nil
		}

		expectedPrev = expectedHash
	}

	return true, nil, nil
}

// CountEventsByType returns the count of a specific event type for a ticket.
func CountEventsByType(db *sql.DB, orgID, ticketID, eventType string) (int, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM ticket_events
		WHERE org_id = $1 AND ticket_id = $2 AND event_type = $3`,
		orgID, ticketID, eventType,
	).Scan(&count)
	return count, err
}

// ── Payload Helpers ───────────────────────────────────────────────────────────

// EventPayload builds a JSON payload from key-value pairs.
func EventPayload(pairs ...string) json.RawMessage {
	if len(pairs)%2 != 0 {
		return json.RawMessage("{}")
	}
	m := make(map[string]string, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	data, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}
