package store

import (
	"database/sql"
	"time"
)

// ── TicketWatcher ─────────────────────────────────────────────────────────────

// TicketWatcher represents an observer subscribed to updates on a ticket.
type TicketWatcher struct {
	ID         string    `json:"id"`
	TicketID   string    `json:"ticket_id"`
	UserID     *string   `json:"user_id,omitempty"`
	CustomerID *string   `json:"customer_id,omitempty"`
	Reason     string    `json:"reason"` // 'requester', 'manager', 'stakeholder', 'auto_added'
	CreatedAt  time.Time `json:"created_at"`
}

// AddTicketWatcher adds an agent or customer as a watcher on a ticket.
func AddTicketWatcher(db *sql.DB, ticketID string, userID, customerID *string, reason string) (*TicketWatcher, error) {
	if reason == "" {
		reason = "stakeholder"
	}
	var w TicketWatcher
	err := db.QueryRow(`
		INSERT INTO ticket_watchers (ticket_id, user_id, customer_id, reason)
		VALUES ($1, $2, $3, $4)
		RETURNING id, ticket_id, user_id, customer_id, reason, created_at`,
		ticketID, userID, customerID, reason,
	).Scan(
		&w.ID, &w.TicketID, &w.UserID, &w.CustomerID, &w.Reason, &w.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// RemoveTicketWatcher deletes a watcher subscription.
func RemoveTicketWatcher(db *sql.DB, id string) error {
	_, err := db.Exec(`
		DELETE FROM ticket_watchers
		WHERE id = $1`,
		id,
	)
	return err
}

// GetTicketWatchers retrieves all watchers for a given ticket.
func GetTicketWatchers(db *sql.DB, ticketID string) ([]*TicketWatcher, error) {
	rows, err := db.Query(`
		SELECT id, ticket_id, user_id, customer_id, reason, created_at
		FROM ticket_watchers
		WHERE ticket_id = $1
		ORDER BY created_at ASC`,
		ticketID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var watchers []*TicketWatcher
	for rows.Next() {
		var w TicketWatcher
		err := rows.Scan(&w.ID, &w.TicketID, &w.UserID, &w.CustomerID, &w.Reason, &w.CreatedAt)
		if err != nil {
			return nil, err
		}
		watchers = append(watchers, &w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return watchers, nil
}

// ── TicketTypePrefix ──────────────────────────────────────────────────────────

// TicketTypePrefix holds custom human-readable identifier overrides per tenant.
type TicketTypePrefix struct {
	OrgID     string    `json:"org_id"`
	Type      string    `json:"type"` // 'request', 'incident', 'task', 'change'
	Prefix    string    `json:"prefix"`
	CreatedAt time.Time `json:"created_at"`
}

// SetTypePrefix configures a custom prefix override for a specific ticket type.
func SetTypePrefix(db *sql.DB, orgID, ticketType, prefix string) error {
	_, err := db.Exec(`
		INSERT INTO ticket_type_prefixes (org_id, type, prefix)
		VALUES ($1, $2, $3)
		ON CONFLICT (org_id, type)
		DO UPDATE SET prefix = EXCLUDED.prefix`,
		orgID, ticketType, prefix,
	)
	return err
}

// GetTypePrefix resolves the configured prefix for an organization and ticket type.
// If no custom prefix is set, returns the standard uppercase code.
func GetTypePrefix(db *sql.DB, orgID, ticketType string) (string, error) {
	var prefix string
	err := db.QueryRow(`
		SELECT prefix
		FROM ticket_type_prefixes
		WHERE org_id = $1 AND type = $2`,
		orgID, ticketType,
	).Scan(&prefix)
	if err == sql.ErrNoRows {
		// Fallback defaults
		switch ticketType {
		case "request":
			return "REQ", nil
		case "incident":
			return "INC", nil
		case "task":
			return "TASK", nil
		case "change":
			return "CHG", nil
		default:
			return "TKT", nil
		}
	}
	if err != nil {
		return "", err
	}
	return prefix, nil
}

// ── Parent-Child Orchestration Helpers ────────────────────────────────────────

// IncrementChildCount atomically increments the child_count counter on a parent ticket.
func IncrementChildCount(db *sql.DB, parentID string) error {
	_, err := db.Exec(`
		UPDATE tickets
		SET child_count = child_count + 1, is_parent = true, updated_at = now()
		WHERE id = $1`,
		parentID,
	)
	return err
}

// UpdateChildResolvedCount recalculates and updates the resolved children counter for a parent.
func UpdateChildResolvedCount(db *sql.DB, parentID string) error {
	_, err := db.Exec(`
		UPDATE tickets
		SET child_resolved_count = (
			SELECT COALESCE(COUNT(*), 0)
			FROM tickets
			WHERE parent_ticket_id = $1 AND status = 'resolved'
		), updated_at = now()
		WHERE id = $1`,
		parentID,
	)
	return err
}
