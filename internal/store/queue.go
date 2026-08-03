package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// ── Queue ─────────────────────────────────────────────────────────────────────

// Queue is a named, filterable container for tickets.
// Departments create queues to organize their work.
// Each queue has its own team, SLA rules, and custom fields.
type Queue struct {
	ID          string          `json:"id"`
	OrgID       string          `json:"org_id"`
	Name        string          `json:"name"`
	Prefix      *string         `json:"prefix,omitempty"`
	Description *string         `json:"description,omitempty"`
	Department  *string         `json:"department,omitempty"`
	Color       string          `json:"color"`
	Icon        *string         `json:"icon,omitempty"`
	Filters     json.RawMessage `json:"filters"`
	SLAConfig   json.RawMessage `json:"sla_config"`
	Active      bool            `json:"active"`
	SortOrder   int             `json:"sort_order"`
	CreatedBy   *string         `json:"created_by,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// QueueWithCounts is a queue with ticket count metadata.
// Used for rendering the sidebar.
type QueueWithCounts struct {
	Queue
	TotalCount  int `json:"total_count"`
	OpenCount   int `json:"open_count"`
	UrgentCount int `json:"urgent_count"`
}

// QueueMember represents an agent assigned to a queue.
type QueueMember struct {
	QueueID     string    `json:"queue_id"`
	UserID      string    `json:"user_id"`
	OrgID       string    `json:"org_id"`
	Role        string    `json:"role"`
	NotifyOnNew bool      `json:"notify_on_new"`
	CreatedAt   time.Time `json:"created_at"`
}

// ── Create ────────────────────────────────────────────────────────────────────

// CreateQueueParams contains everything needed to create a queue.
type CreateQueueParams struct {
	OrgID       string
	Name        string
	Prefix      *string
	Description *string
	Department  *string
	Color       string
	Icon        *string
	CreatedBy   string
}

// CreateQueue inserts a new queue and adds the creator as lead.
func CreateQueue(db *sql.DB, p CreateQueueParams) (*Queue, error) {
	if p.Color == "" {
		p.Color = "#6366f1"
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var q Queue
	err = tx.QueryRow(`
		INSERT INTO queues (org_id, name, prefix, description, department, color, icon, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, org_id, name, prefix, description, department, color, icon,
		          filters, sla_config, active, sort_order, created_by,
		          created_at, updated_at`,
		p.OrgID, p.Name, p.Prefix, p.Description, p.Department, p.Color, p.Icon, p.CreatedBy,
	).Scan(
		&q.ID, &q.OrgID, &q.Name, &q.Prefix, &q.Description, &q.Department, &q.Color, &q.Icon,
		&q.Filters, &q.SLAConfig, &q.Active, &q.SortOrder, &q.CreatedBy,
		&q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Add creator as queue lead
	_, err = tx.Exec(`
		INSERT INTO queue_members (queue_id, user_id, org_id, role)
		VALUES ($1, $2, $3, 'lead')`,
		q.ID, p.CreatedBy, p.OrgID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &q, nil
}

// ── Read ──────────────────────────────────────────────────────────────────────

// GetQueueByID returns a queue by UUID, scoped to org.
func GetQueueByID(db *sql.DB, orgID, queueID string) (*Queue, error) {
	var q Queue
	err := db.QueryRow(`
		SELECT id, org_id, name, prefix, description, department, color, icon,
		       filters, sla_config, active, sort_order, created_by,
		       created_at, updated_at
		FROM queues
		WHERE org_id = $1 AND id = $2`,
		orgID, queueID,
	).Scan(
		&q.ID, &q.OrgID, &q.Name, &q.Prefix, &q.Description, &q.Department, &q.Color, &q.Icon,
		&q.Filters, &q.SLAConfig, &q.Active, &q.SortOrder, &q.CreatedBy,
		&q.CreatedAt, &q.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// ── List With Counts ──────────────────────────────────────────────────────────

// ListQueuesWithCounts returns all active queues for an org with ticket counts.
// This is the single query that powers the sidebar.
func ListQueuesWithCounts(db *sql.DB, orgID string) ([]QueueWithCounts, error) {
	rows, err := db.Query(`
		SELECT
			q.id, q.org_id, q.name, q.prefix, q.description, q.department,
			q.color, q.icon, q.filters, q.sla_config,
			q.active, q.sort_order, q.created_by,
			q.created_at, q.updated_at,
			COUNT(t.id) FILTER (WHERE t.status NOT IN ('closed')) AS total,
			COUNT(t.id) FILTER (WHERE t.status = 'open') AS open_count,
			COUNT(t.id) FILTER (
				WHERE t.priority IN ('P0','P1')
				AND t.status NOT IN ('closed','resolved')
			) AS urgent_count
		FROM queues q
		LEFT JOIN tickets t ON t.queue_id = q.id AND t.org_id = q.org_id
		WHERE q.org_id = $1 AND q.active = true
		GROUP BY q.id
		ORDER BY q.department ASC NULLS LAST, q.sort_order ASC, q.name ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queues []QueueWithCounts
	for rows.Next() {
		var qc QueueWithCounts
		if err := rows.Scan(
			&qc.ID, &qc.OrgID, &qc.Name, &qc.Prefix, &qc.Description, &qc.Department,
			&qc.Color, &qc.Icon, &qc.Filters, &qc.SLAConfig,
			&qc.Active, &qc.SortOrder, &qc.CreatedBy,
			&qc.CreatedAt, &qc.UpdatedAt,
			&qc.TotalCount, &qc.OpenCount, &qc.UrgentCount,
		); err != nil {
			return nil, err
		}
		queues = append(queues, qc)
	}
	return queues, rows.Err()
}

// ListQueuesForUser returns queues that a specific user is a member of.
func ListQueuesForUser(db *sql.DB, orgID, userID string) ([]QueueWithCounts, error) {
	rows, err := db.Query(`
		SELECT
			q.id, q.org_id, q.name, q.prefix, q.description, q.department,
			q.color, q.icon, q.filters, q.sla_config,
			q.active, q.sort_order, q.created_by,
			q.created_at, q.updated_at,
			COUNT(t.id) FILTER (WHERE t.status NOT IN ('closed')) AS total,
			COUNT(t.id) FILTER (WHERE t.status = 'open') AS open_count,
			COUNT(t.id) FILTER (
				WHERE t.priority IN ('P0','P1')
				AND t.status NOT IN ('closed','resolved')
			) AS urgent_count
		FROM queues q
		JOIN queue_members qm ON qm.queue_id = q.id AND qm.user_id = $2
		LEFT JOIN tickets t ON t.queue_id = q.id AND t.org_id = q.org_id
		WHERE q.org_id = $1 AND q.active = true
		GROUP BY q.id
		ORDER BY q.department ASC NULLS LAST, q.sort_order ASC, q.name ASC`,
		orgID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queues []QueueWithCounts
	for rows.Next() {
		var qc QueueWithCounts
		if err := rows.Scan(
			&qc.ID, &qc.OrgID, &qc.Name, &qc.Prefix, &qc.Description, &qc.Department,
			&qc.Color, &qc.Icon, &qc.Filters, &qc.SLAConfig,
			&qc.Active, &qc.SortOrder, &qc.CreatedBy,
			&qc.CreatedAt, &qc.UpdatedAt,
			&qc.TotalCount, &qc.OpenCount, &qc.UrgentCount,
		); err != nil {
			return nil, err
		}
		queues = append(queues, qc)
	}
	return queues, rows.Err()
}

// ── Membership ────────────────────────────────────────────────────────────────

// AddQueueMember adds an agent to a queue. Idempotent.
func AddQueueMember(db *sql.DB, orgID, queueID, userID, role string) error {
	if role == "" {
		role = "member"
	}
	_, err := db.Exec(`
		INSERT INTO queue_members (queue_id, user_id, org_id, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (queue_id, user_id) DO NOTHING`,
		queueID, userID, orgID, role,
	)
	return err
}

// RemoveQueueMember removes an agent from a queue.
func RemoveQueueMember(db *sql.DB, queueID, userID string) error {
	_, err := db.Exec(`
		DELETE FROM queue_members
		WHERE queue_id = $1 AND user_id = $2`,
		queueID, userID,
	)
	return err
}

// ListQueueMembers returns all members of a queue.
func ListQueueMembers(db *sql.DB, orgID, queueID string) ([]QueueMember, error) {
	rows, err := db.Query(`
		SELECT queue_id, user_id, org_id, role, notify_on_new, created_at
		FROM queue_members
		WHERE org_id = $1 AND queue_id = $2
		ORDER BY role ASC, created_at ASC`,
		orgID, queueID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []QueueMember
	for rows.Next() {
		var m QueueMember
		if err := rows.Scan(
			&m.QueueID, &m.UserID, &m.OrgID, &m.Role,
			&m.NotifyOnNew, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// ── Update ────────────────────────────────────────────────────────────────────

// UpdateQueue updates mutable queue fields.
func UpdateQueue(db *sql.DB, orgID, queueID string, name, prefix, description, department, color, icon *string) error {
	_, err := db.Exec(`
		UPDATE queues
		SET name        = COALESCE($1, name),
		    prefix      = COALESCE($2, prefix),
		    description = COALESCE($3, description),
		    department  = COALESCE($4, department),
		    color       = COALESCE($5, color),
		    icon        = COALESCE($6, icon),
		    updated_at  = now()
		WHERE org_id = $7 AND id = $8`,
		name, prefix, description, department, color, icon, orgID, queueID,
	)
	return err
}

// DeactivateQueue soft-deletes a queue. Tickets remain but are unqueued.
func DeactivateQueue(db *sql.DB, orgID, queueID string) error {
	_, err := db.Exec(`
		UPDATE queues
		SET active = false, updated_at = now()
		WHERE org_id = $1 AND id = $2`,
		orgID, queueID,
	)
	return err
}

// ── Ticket Assignment ─────────────────────────────────────────────────────────

// AssignTicketToQueue moves a ticket into a queue.
func AssignTicketToQueue(db *sql.DB, orgID, ticketID, queueID string) error {
	_, err := db.Exec(`
		UPDATE tickets
		SET queue_id = $1, updated_at = now()
		WHERE org_id = $2 AND id = $3`,
		queueID, orgID, ticketID,
	)
	return err
}

// UnassignTicketFromQueue removes a ticket from its queue.
func UnassignTicketFromQueue(db *sql.DB, orgID, ticketID string) error {
	_, err := db.Exec(`
		UPDATE tickets
		SET queue_id = NULL, updated_at = now()
		WHERE org_id = $1 AND id = $2`,
		orgID, ticketID,
	)
	return err
}

// ListTicketsByQueue returns tickets in a specific queue.
func ListTicketsByQueue(db *sql.DB, orgID, queueID string, limit int) ([]Ticket, error) {
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
		WHERE org_id = $1 AND queue_id = $2
		ORDER BY
			CASE priority
				WHEN 'P0' THEN 0
				WHEN 'P1' THEN 1
				WHEN 'P2' THEN 2
				WHEN 'P3' THEN 3
				ELSE 4
			END ASC,
			created_at ASC
		LIMIT $3`,
		orgID, queueID, limit,
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

// ── Counts ────────────────────────────────────────────────────────────────────

// OrgTicketCounts returns aggregate ticket counts for the entire org.
// Used for the "All Tickets" view.
type OrgTicketCounts struct {
	Total           int `json:"total"`
	Open            int `json:"open"`
	Assigned        int `json:"assigned"`
	InProgress      int `json:"in_progress"`
	PendingCustomer int `json:"pending_customer"`
	Resolved        int `json:"resolved"`
	Unassigned      int `json:"unassigned"`
	Urgent          int `json:"urgent"`
	Unqueued        int `json:"unqueued"`
}

// GetOrgTicketCounts returns aggregate counts for all tickets in an org.
func GetOrgTicketCounts(db *sql.DB, orgID string) (*OrgTicketCounts, error) {
	var c OrgTicketCounts
	err := db.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE status NOT IN ('closed')) AS total,
			COUNT(*) FILTER (WHERE status = 'open') AS open,
			COUNT(*) FILTER (WHERE status = 'assigned') AS assigned,
			COUNT(*) FILTER (WHERE status = 'in_progress') AS in_progress,
			COUNT(*) FILTER (WHERE status = 'pending_customer') AS pending_customer,
			COUNT(*) FILTER (WHERE status = 'resolved') AS resolved,
			COUNT(*) FILTER (WHERE assigned_to IS NULL AND status NOT IN ('closed','resolved')) AS unassigned,
			COUNT(*) FILTER (WHERE priority IN ('P0','P1') AND status NOT IN ('closed','resolved')) AS urgent,
			COUNT(*) FILTER (WHERE queue_id IS NULL AND status NOT IN ('closed')) AS unqueued
		FROM tickets
		WHERE org_id = $1`,
		orgID,
	).Scan(
		&c.Total, &c.Open, &c.Assigned, &c.InProgress,
		&c.PendingCustomer, &c.Resolved, &c.Unassigned,
		&c.Urgent, &c.Unqueued,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}
