package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ── TicketEvent ───────────────────────────────────────────────────────────────

// TicketEvent is an immutable record of something that happened to a ticket.
// This table is append-only. No UPDATE. No DELETE. Ever.
// You can reconstruct the complete state of any ticket at any point in time
// by replaying its events in order.
type TicketEvent struct {
	ID          string          `json:"id"`
	TicketID    string          `json:"ticket_id"`
	OrgID       string          `json:"org_id"`
	ActorUserID *string         `json:"actor_user_id,omitempty"`
	ActorType   string          `json:"actor_type"`
	EventType   string          `json:"event_type"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"created_at"`
}

// ── Queries ───────────────────────────────────────────────────────────────────

// RecordEvent inserts a single event into the audit log.
// Use this for standalone events outside of a transaction.
// For events that must be atomic with a ticket change, use RecordEventTx.
func RecordEvent(db *sql.DB, orgID, ticketID string, actorUserID *string, actorType, eventType string, payload json.RawMessage) error {
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}
	_, err := db.Exec(`
		INSERT INTO ticket_events (ticket_id, org_id, actor_user_id, actor_type, event_type, payload)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		ticketID, orgID, actorUserID, actorType, eventType, payload,
	)
	return err
}

// RecordEventTx inserts an event within an existing transaction.
// Used when an event must be atomic with a ticket or comment change.
func RecordEventTx(tx *sql.Tx, orgID, ticketID string, actorUserID *string, actorType, eventType string, payload json.RawMessage) error {
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}
	_, err := tx.Exec(`
		INSERT INTO ticket_events (ticket_id, org_id, actor_user_id, actor_type, event_type, payload)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		ticketID, orgID, actorUserID, actorType, eventType, payload,
	)
	return err
}

// ListEventsByTicket returns all events for a ticket in chronological order.
// This is the complete audit trail for a single ticket.
func ListEventsByTicket(db *sql.DB, orgID, ticketID string) ([]TicketEvent, error) {
	rows, err := db.Query(`
		SELECT id, ticket_id, org_id, actor_user_id, actor_type,
		       event_type, payload, created_at
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
			&e.ActorType, &e.EventType, &e.Payload, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// ListEventsByOrg returns recent events across all tickets in an org.
// Used for the activity feed / dashboard.
func ListEventsByOrg(db *sql.DB, orgID string, limit int) ([]TicketEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := db.Query(`
		SELECT id, ticket_id, org_id, actor_user_id, actor_type,
		       event_type, payload, created_at
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
			&e.ActorType, &e.EventType, &e.Payload, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// CountEventsByType returns the count of a specific event type for a ticket.
// Useful for metrics like "how many times was this ticket reassigned?"
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
// Usage: EventPayload("from", "open", "to", "assigned")
func EventPayload(pairs ...string) json.RawMessage {
	if len(pairs)%2 != 0 {
		return json.RawMessage(`{}`)
	}
	m := make(map[string]string, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	data, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

// ── Comment ───────────────────────────────────────────────────────────────────

// Comment is a message in a ticket's conversation thread.
type Comment struct {
	ID         string    `json:"id"`
	TicketID   string    `json:"ticket_id"`
	AuthorID   *string   `json:"author_id,omitempty"`   // agent, if internal
	CustomerID *string   `json:"customer_id,omitempty"` // customer, if external
	Body       string    `json:"body"`
	IsInternal bool      `json:"is_internal"`
	SourceType string    `json:"source_type"`
	ExternalID *string   `json:"external_id,omitempty"`
	AIDrafted  bool      `json:"ai_drafted"`
	AIAccepted *bool     `json:"ai_accepted,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// CreateCommentParams contains everything needed to add a comment.
type CreateCommentParams struct {
	OrgID      string
	TicketID   string
	AuthorID   *string // agent user ID
	CustomerID *string // customer ID
	Body       string
	IsInternal bool
	SourceType string
	ExternalID *string
	AIDrafted  bool
}

// CreateComment adds a comment to a ticket and records the event.
// Wrapped in a transaction.
func CreateComment(db *sql.DB, p CreateCommentParams) (*Comment, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var c Comment
	err = tx.QueryRow(`
		INSERT INTO comments (
			ticket_id, author_id, customer_id,
			body, is_internal, source_type, external_id, ai_drafted
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, ticket_id, author_id, customer_id,
		          body, is_internal, source_type, external_id,
		          ai_drafted, ai_accepted, created_at`,
		p.TicketID, p.AuthorID, p.CustomerID,
		p.Body, p.IsInternal, p.SourceType, p.ExternalID, p.AIDrafted,
	).Scan(
		&c.ID, &c.TicketID, &c.AuthorID, &c.CustomerID,
		&c.Body, &c.IsInternal, &c.SourceType, &c.ExternalID,
		&c.AIDrafted, &c.AIAccepted, &c.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Determine event type
	eventType := "comment.added"
	if p.IsInternal {
		eventType = "comment.internal"
	}

	// Determine actor
	actorType := "user"
	var actorID *string
	if p.AuthorID != nil {
		actorID = p.AuthorID
	} else {
		actorType = "webhook" // customer comments come via external channels
	}

	err = RecordEventTx(tx, p.OrgID, p.TicketID, actorID, actorType, eventType,
		EventPayload("comment_id", c.ID, "internal", fmt.Sprintf("%t", p.IsInternal)),
	)
	if err != nil {
		return nil, err
	}

	// Update ticket's updated_at
	_, err = tx.Exec(
		"UPDATE tickets SET updated_at = now() WHERE id = $1",
		p.TicketID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &c, nil
}

// ListCommentsByTicket returns all comments for a ticket in chronological order.
// If includeInternal is false, internal notes are excluded (for customer-facing views).
func ListCommentsByTicket(db *sql.DB, ticketID string, includeInternal bool) ([]Comment, error) {
	query := `
		SELECT id, ticket_id, author_id, customer_id,
		       body, is_internal, source_type, external_id,
		       ai_drafted, ai_accepted, created_at
		FROM comments
		WHERE ticket_id = $1`

	if !includeInternal {
		query += " AND is_internal = false"
	}

	query += " ORDER BY created_at ASC"

	rows, err := db.Query(query, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(
			&c.ID, &c.TicketID, &c.AuthorID, &c.CustomerID,
			&c.Body, &c.IsInternal, &c.SourceType, &c.ExternalID,
			&c.AIDrafted, &c.AIAccepted, &c.CreatedAt,
		); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}
