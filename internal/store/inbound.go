package store

import (
	"database/sql"
	"time"
)

// InboundMessage stores the raw payload from an email/webhook before processing.
// We store first, parse second — never lose the original data.
type InboundMessage struct {
	ID          string     `json:"id"`
	OrgID       *string    `json:"org_id,omitempty"`
	SourceType  string     `json:"source_type"`
	ExternalID  string     `json:"external_id"`
	RawPayload  []byte     `json:"raw_payload"`
	Processed   bool       `json:"processed"`
	TicketID    *string    `json:"ticket_id,omitempty"`
	ReceivedAt  time.Time  `json:"received_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

// CreateInboundMessage stores a raw inbound message before processing.
// external_id is UNIQUE — duplicate deliveries are rejected safely.
func CreateInboundMessage(db *sql.DB, sourceType, externalID string, rawPayload []byte) (*InboundMessage, error) {
	var msg InboundMessage
	err := db.QueryRow(`
		INSERT INTO inbound_messages (source_type, external_id, raw_payload)
		VALUES ($1, $2, $3)
		RETURNING id, org_id, source_type, external_id, raw_payload,
		          processed, ticket_id, received_at, processed_at`,
		sourceType, externalID, rawPayload,
	).Scan(
		&msg.ID, &msg.OrgID, &msg.SourceType, &msg.ExternalID, &msg.RawPayload,
		&msg.Processed, &msg.TicketID, &msg.ReceivedAt, &msg.ProcessedAt,
	)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// MarkInboundProcessed links the inbound message to the created ticket
// and marks it as processed.
func MarkInboundProcessed(db *sql.DB, messageID, orgID, ticketID string) error {
	_, err := db.Exec(`
		UPDATE inbound_messages
		SET processed = true, org_id = $1, ticket_id = $2, processed_at = now()
		WHERE id = $3`,
		orgID, ticketID, messageID,
	)
	return err
}

// GetInboundMessageByExternalID finds an inbound message by its external Message-ID.
// Used for email threading to find the ticket associated with a parent email.
func GetInboundMessageByExternalID(db *sql.DB, externalID string) (*InboundMessage, error) {
	var msg InboundMessage
	err := db.QueryRow(`
		SELECT id, org_id, source_type, external_id, raw_payload,
		       processed, ticket_id, received_at, processed_at
		FROM inbound_messages
		WHERE external_id = $1`,
		externalID,
	).Scan(
		&msg.ID, &msg.OrgID, &msg.SourceType, &msg.ExternalID, &msg.RawPayload,
		&msg.Processed, &msg.TicketID, &msg.ReceivedAt, &msg.ProcessedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// GetTicketByDisplayID returns a ticket by its human-readable display ID (e.g. IT-OPS-SR-0042).
func GetTicketByDisplayID(db *sql.DB, orgID, displayID string) (*Ticket, error) {
	var t Ticket
	err := db.QueryRow(`
		SELECT id, org_id, display_id, ticket_type, sequence_num,
		       customer_id, assigned_to,
		       subject, body, status, priority, category,
		       source_type, thread_id, incident_id,
		       sla_due_at, sla_breached,
		       created_at, updated_at, resolved_at
		FROM tickets
		WHERE org_id = $1 AND display_id = $2`,
		orgID, displayID,
	).Scan(
		&t.ID, &t.OrgID, &t.DisplayID, &t.TicketType, &t.SequenceNum,
		&t.CustomerID, &t.AssignedTo,
		&t.Subject, &t.Body, &t.Status, &t.Priority, &t.Category,
		&t.SourceType, &t.ThreadID, &t.IncidentID,
		&t.SLADueAt, &t.SLABreached,
		&t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}
