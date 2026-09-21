package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/psiloconvalley/404not403/internal/domain"
)

// ── Ticket ────────────────────────────────────────────────────────────────────

// Ticket is the core work unit of the system.
// Scoped to an organization and governed by the domain state machine.
type Ticket struct {
	ID                    string     `json:"id"`
	OrgID                 string     `json:"org_id"`
	DisplayID             string     `json:"display_id"`
	TicketType            string     `json:"ticket_type"`
	SequenceNum           int        `json:"sequence_num"`
	CustomerID            *string    `json:"customer_id,omitempty"`
	AssignedTo            *string    `json:"assigned_to,omitempty"`
	Subject               string     `json:"subject"`
	Body                  string     `json:"body"`
	Status                string     `json:"status"`
	Priority              string     `json:"priority"`
	Category              *string    `json:"category,omitempty"`
	SourceType            string     `json:"source_type"`
	ThreadID              *string    `json:"thread_id,omitempty"`
	IncidentID            *string    `json:"incident_id,omitempty"`
	SLADueAt              *time.Time `json:"sla_due_at,omitempty"`
	SLABreached           bool       `json:"sla_breached"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	ResolvedAt            *time.Time `json:"resolved_at,omitempty"`
	ParentTicketID        *string    `json:"parent_ticket_id,omitempty"`
	IsParent              bool       `json:"is_parent"`
	ChildCount            int        `json:"child_count"`
	ChildResolvedCount    int        `json:"child_resolved_count"`
	SubmittedByCustomerID *string    `json:"submitted_by_customer_id,omitempty"`
	SubmittedByUserID     *string    `json:"submitted_by_user_id,omitempty"`
	RequesterCustomerID   *string    `json:"requester_customer_id,omitempty"`
}

const ticketSelectCols = `
	id, org_id, display_id, ticket_type, sequence_num,
	customer_id, assigned_to,
	subject, body, status, priority, category,
	source_type, thread_id, incident_id,
	sla_due_at, sla_breached,
	created_at, updated_at, resolved_at,
	parent_ticket_id, is_parent, child_count, child_resolved_count,
	submitted_by_customer_id, submitted_by_user_id, requester_customer_id
`

func scanTicket(row interface{ Scan(dest ...interface{}) error }, t *Ticket) error {
	return row.Scan(
		&t.ID, &t.OrgID, &t.DisplayID, &t.TicketType, &t.SequenceNum,
		&t.CustomerID, &t.AssignedTo,
		&t.Subject, &t.Body, &t.Status, &t.Priority, &t.Category,
		&t.SourceType, &t.ThreadID, &t.IncidentID,
		&t.SLADueAt, &t.SLABreached,
		&t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt,
		&t.ParentTicketID, &t.IsParent, &t.ChildCount, &t.ChildResolvedCount,
		&t.SubmittedByCustomerID, &t.SubmittedByUserID, &t.RequesterCustomerID,
	)
}

// ── Create ────────────────────────────────────────────────────────────────────

// CreateTicketParams contains everything needed to create a ticket.
type CreateTicketParams struct {
	OrgID                 string
	QueueID               *string
	CustomerID            *string
	Subject               string
	Body                  string
	Priority              string     // validated domain.Priority
	SourceType            string     // validated domain.SourceType
	ThreadID              *string
	TicketType            string     // service_request, incident, change, problem, task
	SLADueAt              *time.Time // calculated from catalog item sla_hours
	ParentTicketID        *string
	SubmittedByCustomerID *string
	SubmittedByUserID     *string
	RequesterCustomerID   *string
}

// CreateTicket inserts a new ticket and records the creation event.
func CreateTicket(db *sql.DB, p CreateTicketParams) (*Ticket, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if p.TicketType == "" {
		p.TicketType = "service_request"
	}

	// Default submitter and requester if not explicitly provided
	if p.SubmittedByCustomerID == nil && p.SubmittedByUserID == nil && p.CustomerID != nil {
		p.SubmittedByCustomerID = p.CustomerID
	}
	if p.RequesterCustomerID == nil && p.CustomerID != nil {
		p.RequesterCustomerID = p.CustomerID
	}

	// Get next sequence number for this org
	var seqNum int64
	err = tx.QueryRow(`
		INSERT INTO ticket_sequences (org_id, counter)
		VALUES ($1, 1)
		ON CONFLICT (org_id) DO UPDATE
		SET counter = ticket_sequences.counter + 1
		RETURNING counter`,
		p.OrgID,
	).Scan(&seqNum)
	if err != nil {
		return nil, fmt.Errorf("failed to get sequence: %w", err)
	}

	// Resolve queue prefix for display ID
	queuePrefix := "REQ"
	if p.QueueID != nil && *p.QueueID != "" {
		var prefix *string
		_ = tx.QueryRow("SELECT prefix FROM queues WHERE id = $1 AND org_id = $2", *p.QueueID, p.OrgID).Scan(&prefix)
		if prefix != nil && *prefix != "" {
			queuePrefix = *prefix
		}
	}

	// Build display ID: {prefix}-{type_code}-{sequence}
	typeCode := map[string]string{
		"service_request": "SR",
		"incident":        "INC",
		"change":          "CHG",
		"problem":         "PRB",
		"task":            "TSK",
	}[p.TicketType]
	if typeCode == "" {
		typeCode = "SR"
	}
	displayID := fmt.Sprintf("%s-%s-%04d", queuePrefix, typeCode, seqNum)

	var t Ticket
	query := `
		INSERT INTO tickets (
			org_id, customer_id, subject, body,
			status, priority, source_type, thread_id, ticket_type,
			sequence_num, display_id, queue_id, sla_due_at,
			parent_ticket_id, submitted_by_customer_id, submitted_by_user_id, requester_customer_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING ` + ticketSelectCols

	err = scanTicket(tx.QueryRow(
		query,
		p.OrgID, p.CustomerID, p.Subject, p.Body,
		string(domain.DefaultStatus()),
		p.Priority,
		p.SourceType, p.ThreadID, p.TicketType,
		seqNum, displayID, p.QueueID, p.SLADueAt,
		p.ParentTicketID, p.SubmittedByCustomerID, p.SubmittedByUserID, p.RequesterCustomerID,
	), &t)
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
	query := `SELECT ` + ticketSelectCols + ` FROM tickets WHERE org_id = $1 AND id = $2`
	err := scanTicket(db.QueryRow(query, orgID, ticketID), &t)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTicketByThreadID returns a ticket by its external thread identifier.
func GetTicketByThreadID(db *sql.DB, threadID string) (*Ticket, error) {
	var t Ticket
	query := `SELECT ` + ticketSelectCols + ` FROM tickets WHERE thread_id = $1`
	err := scanTicket(db.QueryRow(query, threadID), &t)
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

	query := `SELECT ` + ticketSelectCols + ` FROM tickets WHERE org_id = $1`
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
		if err := scanTicket(rows, &t); err != nil {
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

	sqlQuery := `
		SELECT ` + ticketSelectCols + `
		FROM tickets
		WHERE org_id = $1
		  AND search_vector @@ plainto_tsquery('english', $2)
		ORDER BY ts_rank(search_vector, plainto_tsquery('english', $2)) DESC
		LIMIT $3`

	rows, err := db.Query(sqlQuery, orgID, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		if err := scanTicket(rows, &t); err != nil {
			return nil, err
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

// ── Update ────────────────────────────────────────────────────────────────────

// UpdateTicketStatus transitions a ticket to a new status.
func UpdateTicketStatus(db *sql.DB, orgID, ticketID, actorUserID, newStatus string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentStatus string
	err = tx.QueryRow(`
		SELECT status FROM tickets
		WHERE org_id = $1 AND id = $2
		FOR UPDATE`,
		orgID, ticketID,
	).Scan(&currentStatus)
	if err != nil {
		return err
	}

	var resolvedAt *time.Time
	if newStatus == string(domain.StatusResolved) || newStatus == string(domain.StatusClosed) {
		now := time.Now().UTC()
		resolvedAt = &now
	}

	_, err = tx.Exec(`
		UPDATE tickets
		SET status = $1, resolved_at = $2, updated_at = now()
		WHERE org_id = $3 AND id = $4`,
		newStatus, resolvedAt, orgID, ticketID,
	)
	if err != nil {
		return err
	}

	var actorType domain.ActorType
	if actorUserID == "system" {
		actorType = domain.ActorSystem
	} else {
		actorType = domain.ActorUser
	}

	_, err = tx.Exec(`
		INSERT INTO ticket_events (ticket_id, org_id, actor_user_id, actor_type, event_type, payload)
		VALUES ($1, $2, NULLIF($3, 'system')::uuid, $4, $5, $6)`,
		ticketID, orgID, actorUserID,
		string(actorType),
		string(domain.EventTicketStatusChange),
		fmt.Sprintf(`{"from":"%s","to":"%s"}`, currentStatus, newStatus),
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateTicketPriority changes the priority of a ticket.
func UpdateTicketPriority(db *sql.DB, orgID, ticketID, actorUserID, newPriority string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentPriority string
	err = tx.QueryRow(`
		SELECT priority FROM tickets
		WHERE org_id = $1 AND id = $2
		FOR UPDATE`,
		orgID, ticketID,
	).Scan(&currentPriority)
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

	var actorType domain.ActorType
	if actorUserID == "system" {
		actorType = domain.ActorSystem
	} else {
		actorType = domain.ActorUser
	}

	_, err = tx.Exec(`
		INSERT INTO ticket_events (ticket_id, org_id, actor_user_id, actor_type, event_type, payload)
		VALUES ($1, $2, NULLIF($3, 'system')::uuid, $4, $5, $6)`,
		ticketID, orgID, actorUserID,
		string(actorType),
		string(domain.EventTicketPriorityChange),
		fmt.Sprintf(`{"from":"%s","to":"%s"}`, currentPriority, newPriority),
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// AssignTicket assigns a ticket to an agent.
func AssignTicket(db *sql.DB, orgID, ticketID, actorUserID, assigneeUserID string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentStatus string
	var currentAssigned *string
	err = tx.QueryRow(`
		SELECT status, assigned_to FROM tickets
		WHERE org_id = $1 AND id = $2
		FOR UPDATE`,
		orgID, ticketID,
	).Scan(&currentStatus, &currentAssigned)
	if err != nil {
		return err
	}

	newStatus := currentStatus
	if currentStatus == string(domain.StatusOpen) {
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
func UpdateTicketCategory(db *sql.DB, orgID, ticketID, category string) error {
	_, err := db.Exec(`
		UPDATE tickets
		SET category = $1, updated_at = now()
		WHERE org_id = $2 AND id = $3`,
		category, orgID, ticketID,
	)
	return err
}

// ListTicketsForAgent returns tickets visible to an agent.
func ListTicketsForAgent(db *sql.DB, orgID, userID string, limit int) ([]Ticket, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := `
		SELECT DISTINCT ` + ticketSelectCols + `
		FROM tickets
		WHERE org_id = $1
		  AND (queue_id IN (
		        SELECT queue_id FROM queue_members WHERE user_id = $2 AND org_id = $1
		        UNION
		        SELECT dq.queue_id FROM department_queues dq 
		        JOIN department_members dm ON dq.department_id = dm.department_id 
		        WHERE dm.user_id = $2
		      ) OR assigned_to = $2)
		ORDER BY created_at DESC
		LIMIT $3`

	rows, err := db.Query(query, orgID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		if err := scanTicket(rows, &t); err != nil {
			return nil, err
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

// NextTicketSequence atomically increments and returns the next ticket number for an org.
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
