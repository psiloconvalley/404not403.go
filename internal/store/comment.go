package store

import (
	"database/sql"
	"fmt"
	"time"
)

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
