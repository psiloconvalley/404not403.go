package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/psiloconvalley/404not403/internal/domain"
)

// ── Ticket ────────────────────────────────────────────────────────────────────

// Ticket is the core work unit of the system.
// Every ticket belongs to an organization.
// Every ticket has a lifecycle governed by the domain state machine.
type Ticket struct {
	ID           string     `json:"id"`
	OrgID        string     `json:"org_id"`
	CustomerID   *string    `json:"customer_id,omitempty"`
	AssignedTo   *string    `json:"assigned_to,omitempty"`
	Subject      string     `json:"subject"`
	Body         string     `json:"body"`
	Status       string     `json:"status"`
	Priority     string     `json:"priority"`
	Category     *string    `json:"category,omitempty"`
	SourceType   string     `json:"source_type"`
	ThreadID     *string    `json:"thread_id,omitempty"`
	IncidentID   *string    `json:"incident_id,omitempty"`
	SLADueAt     *time.Time `json:"sla_due_at,omitempty"`
	SLABreached  bool       `json:"sla_breached"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
}

// ── Create ────────────────────────────────────────────────────────────────────

// CreateTicketParams contains everything needed to create a ticket.
// Validated before reaching the store layer.
type CreateTicketParams struct {
	OrgID      string
	CustomerID *string
	Subject    string
	Body       string
	Priority   string // validated domain.Priority
	SourceType string // validated domain.SourceType
	ThreadID   *string
}

// CreateTicket inserts a new ticket and records the creation event.
// Wrapped in a transaction — ticket + event succeed together or not at all.
func CreateTicket(db *sql.DB, p CreateTicketParams) (*Ticket, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var t Ticket
	err = tx.QueryRow(`
		INSERT INTO tickets (
			org_id, customer_id, subject, body,
			status, priority, source_type, thread_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, org_id, customer_id, assigned_to,
		          subject, body, status, priority, category,
		          source_type, thread_id, incident_id,
		          sla_due_at, sla_breached,
		          created_at, updated_at, resolved_at`,
		p.OrgID, p.CustomerID, p.Subject, p.Body,
		string(domain.DefaultStatus()),
		p.Priority,
		p.SourceType, p.ThreadID,
	).Scan(
		&t.ID, &t.OrgID, &t.CustomerID, &t.AssignedTo,
		&t.Subject, &t.Body, &t.Status, &t.Priority, &t.Category,
		&t.SourceType, &t.ThreadID, &t.IncidentID,
		&t.SLADueAt, &t.SLABreached,
		&t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt,
	)
	if err != nil {
		return nil, err
	}

	// Record creation event in the same transaction
	_, err = tx.Exec(`
		INSERT INTO ticket_events (ticket_id, org_id, actor_type, event_type, payload)
		VALUES ($1, $2, $3, $4, $5)`,
		t.ID, t.OrgID,
		string(domain.ActorSystem),
		string(domain.EventTicketCreated),
		fmt.Sprintf(`{"source":"%s","priority":"%s"}`, p.SourceType, p.Priority),
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &t, nil
}

// ── Read ──────────────────────────────────────────────────────────────────────

// GetTicketByID returns a single ticket by UUID, scoped to org.
func GetTicketByID(db *sql.DB, orgID, ticketID string) (*Ticket, error) {
	var t Ticket
	err := db.QueryRow(`
		SELECT id, org_id, customer_id, assigned_to,
		       subject, body, status, priority, category,
		       source_type, thread_id, incident_id,
		       sla_due_at, sla_breached,
		       created_at, updated_at, resolved_at
		FROM tickets
		WHERE org_id = $1 AND id = $2`,
		orgID, ticketID,
	).Scan(
		&t.ID, &t.OrgID, &t.CustomerID, &t.AssignedTo,
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

// GetTicketByThreadID returns a ticket by its external thread identifier.
// Used for idempotent ingestion — prevents duplicate tickets from the same email/slack thread.
func GetTicketByThreadID(db *sql.DB, threadID string) (*Ticket, error) {
	var t Ticket
	err := db.QueryRow(`
		SELECT id, org_id, customer_id, assigned_to,
		       subject, body, status, priority, category,
		       source_type, thread_id, incident_id,
		       sla_due_at, sla_breached,
		       created_at, updated_at, resolved_at
		FROM tickets
		WHERE thread_id = $1`,
		threadID,
	).Scan(
		&t.ID, &t.OrgID, &t.CustomerID, &t.AssignedTo,
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

// ── List ──────────────────────────────────────────────────────────────────────

// ListTicketsParams allows filtering and pagination of ticket lists.
type ListTicketsParams struct {
	OrgID      string
	Status     *string // filter by status
	Priority   *string // filter by priority
	AssignedTo *string // filter by agent
	Limit      int
	Offset     int
}

// ListTickets returns tickets for an org with optional filters.
func ListTickets(db *sql.DB, p ListTicketsParams) ([]Ticket, error) {
	if p.Limit <= 0 || p.Limit > 200 {
		p.Limit = 50
	}

	query := `
		SELECT id, org_id, customer_id, assigned_to,
		       subject, body, status, priority, category,
		       source_type, thread_id, incident_id,
		       sla_due_at, sla_breached,
		       created_at, updated_at, resolved_at
		FROM tickets
		WHERE org_id = $1`

	args := []interface{}{p.OrgID}
	argIdx := 2

	if p.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, *p.Status)
		argIdx++
	}

	if p.Priority != nil {
		query += fmt.Sprintf(" AND priority = $%d", argIdx)
		args = append(args, *p.Priority)
		argIdx++
	}

	if p.AssignedTo != nil {
		query += fmt.Sprintf(" AND assigned_to = $%d", argIdx)
		args = append(args, *p.AssignedTo)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, p.Limit, p.Offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(
			&t.ID, &t.OrgID, &t.CustomerID, &t.AssignedTo,
			&t.Subject, &t.Body, &t.Status, &t.Priority, &t.Category,
			&t.SourceType, &t.ThreadID, &t.IncidentID,
			&t.SLADueAt, &t.SLABreached,
			&t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt,
		); err != nil {
			return nil, err
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

// SearchTickets performs full text search on tickets within an org.
func SearchTickets(db *sql.DB, orgID, query string, limit int) ([]Ticket, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := db.Query(`
		SELECT id, org_id, customer_id, assigned_to,
		       subject, body, status, priority, category,
		       source_type, thread_id, incident_id,
		       sla_due_at, sla_breached,
		       created_at, updated_at, resolved_at
		FROM tickets
		WHERE org_id = $1
		  AND search_vector @@ plainto_tsquery('english', $2)
		ORDER BY ts_rank(search_vector, plainto_tsquery('english', $2)) DESC
		LIMIT $3`,
		orgID, query, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(
			&t.ID, &t.OrgID, &t.CustomerID, &t.AssignedTo,
			&t.Subject, &t.Body, &t.Status, &t.Priority, &t.Category,
			&t.SourceType, &t.ThreadID, &t.IncidentID,
			&t.SLADueAt, &t.SLABreached,
			&t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt,
		); err != nil {
			return nil, err
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

// ── Update ────────────────────────────────────────────────────────────────────

// UpdateTicketStatus transitions a ticket to a new status.
// Validates the transition against the domain state machine.
// Records the status change event in the same transaction.
func UpdateTicketStatus(db *sql.DB, orgID, ticketID, actorUserID, newStatus string) error {
	// Validate new status is a known value
	to, err := domain.ParseStatus(newStatus)
	if err != nil {
		return domain.ErrInvalidStatus
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Read current status inside transaction
	var currentStatus string
	err = tx.QueryRow(
		"SELECT status FROM tickets WHERE org_id = $1 AND id = $2 FOR UPDATE",
		orgID, ticketID,
	).Scan(&currentStatus)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	from, err := domain.ParseStatus(currentStatus)
	if err != nil {
		return err
	}

	// Enforce state machine
	if !from.CanTransitionTo(to) {
		return domain.ErrInvalidTransition
	}

	// Determine resolved_at
	resolvedClause := ""
	if to.IsResolved() {
		resolvedClause = ", resolved_at = now()"
	}

	_, err = tx.Exec(
		fmt.Sprintf(`
			UPDATE tickets
			SET status = $1, updated_at = now()%s
			WHERE org_id = $2 AND id = $3`,
			resolvedClause),
		newStatus, orgID, ticketID,
	)
	if err != nil {
		return err
	}

	// Record event
	_, err = tx.Exec(`
		INSERT INTO ticket_events (ticket_id, org_id, actor_user_id, actor_type, event_type, payload)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		ticketID, orgID, actorUserID,
		string(domain.ActorUser),
		string(domain.EventTicketStatusChange),
		fmt.Sprintf(`{"from":"%s","to":"%s"}`, currentStatus, newStatus),
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateTicketPriority changes a ticket's priority.
// Records the priority change event.
func UpdateTicketPriority(db *sql.DB, orgID, ticketID, actorUserID, newPriority string) error {
	if _, err := domain.ParsePriority(newPriority); err != nil {
		return domain.ErrInvalidPriority
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentPriority string
	err = tx.QueryRow(
		"SELECT priority FROM tickets WHERE org_id = $1 AND id = $2 FOR UPDATE",
		orgID, ticketID,
	).Scan(&currentPriority)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		UPDATE tickets
		SET priority = $1, updated_at = now()
		WHERE org_id = $2 AND id = $3`,
		newPriority, orgID, ticketID,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		INSERT INTO ticket_events (ticket_id, org_id, actor_user_id, actor_type, event_type, payload)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		ticketID, orgID, actorUserID,
		string(domain.ActorUser),
		string(domain.EventTicketPriorityChange),
		fmt.Sprintf(`{"from":"%s","to":"%s"}`, currentPriority, newPriority),
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// AssignTicket assigns a ticket to an agent.
// Records the assignment event.
func AssignTicket(db *sql.DB, orgID, ticketID, actorUserID, assigneeUserID string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentStatus string
	var currentAssigned *string
	err = tx.QueryRow(
		"SELECT status, assigned_to FROM tickets WHERE org_id = $1 AND id = $2 FOR UPDATE",
		orgID, ticketID,
	).Scan(&currentStatus, &currentAssigned)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	// Determine new status — if open or reopened, auto-transition to assigned
	newStatus := currentStatus
	from, _ := domain.ParseStatus(currentStatus)
	if from == domain.StatusOpen || from == domain.StatusReopened {
		newStatus = string(domain.StatusAssigned)
	}

	_, err = tx.Exec(`
		UPDATE tickets
		SET assigned_to = $1, status = $2, updated_at = now()
		WHERE org_id = $3 AND id = $4`,
		assigneeUserID, newStatus, orgID, ticketID,
	)
	if err != nil {
		return err
	}

	// Record assignment event
	_, err = tx.Exec(`
		INSERT INTO ticket_events (ticket_id, org_id, actor_user_id, actor_type, event_type, payload)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		ticketID, orgID, actorUserID,
		string(domain.ActorUser),
		string(domain.EventTicketAssigned),
		fmt.Sprintf(`{"assigned_to":"%s","status":"%s"}`, assigneeUserID, newStatus),
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateTicketCategory sets the category on a ticket.
// Typically called by AI classification or agent manually.
func UpdateTicketCategory(db *sql.DB, orgID, ticketID, category string) error {
	_, err := db.Exec(`
		UPDATE tickets
		SET category = $1, updated_at = now()
		WHERE org_id = $2 AND id = $3`,
		category, orgID, ticketID,
	)
	return err
}

// ListTicketsForAgent returns tickets visible to an agent:
//   - tickets in queues the agent belongs to
//   - tickets assigned to the agent
// This enforces queue-based access control at the data layer.
func ListTicketsForAgent(db *sql.DB, orgID, userID string, limit int) ([]Ticket, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := db.Query(`
		SELECT DISTINCT t.id, t.org_id, t.customer_id, t.assigned_to,
		       t.subject, t.body, t.status, t.priority, t.category,
		       t.source_type, t.thread_id, t.incident_id,
		       t.sla_due_at, t.sla_breached,
		       t.created_at, t.updated_at, t.resolved_at
		FROM tickets t
		LEFT JOIN queue_members qm
		  ON qm.queue_id = t.queue_id AND qm.user_id = $2 AND qm.org_id = $1
		WHERE t.org_id = $1
		  AND (qm.user_id IS NOT NULL OR t.assigned_to = $2)
		ORDER BY t.created_at DESC
		LIMIT $3`,
		orgID, userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(
			&t.ID, &t.OrgID, &t.CustomerID, &t.AssignedTo,
			&t.Subject, &t.Body, &t.Status, &t.Priority, &t.Category,
			&t.SourceType, &t.ThreadID, &t.IncidentID,
			&t.SLADueAt, &t.SLABreached,
			&t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt,
		); err != nil {
			return nil, err
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

// NextTicketSequence atomically increments and returns the next ticket number for an org.
// Uses INSERT ON CONFLICT to initialize the counter if it doesn't exist.
func NextTicketSequence(db *sql.DB, orgID string) (int64, error) {
	var seq int64
	err := db.QueryRow(`
		INSERT INTO ticket_sequences (org_id, counter)
		VALUES ($1, 1)
		ON CONFLICT (org_id) DO UPDATE
		SET counter = ticket_sequences.counter + 1
		RETURNING counter`,
		orgID,
	).Scan(&seq)
	return seq, err
}
