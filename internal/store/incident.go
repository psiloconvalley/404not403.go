package store

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// ── Incident ──────────────────────────────────────────────────────────────────

type Incident struct {
	ID                 string     `json:"id"`
	OrgID              string     `json:"org_id"`
	Title              string     `json:"title"`
	Description        *string    `json:"description,omitempty"`
	Severity           string     `json:"severity"`
	Status             string     `json:"status"`
	CommanderID        *string    `json:"commander_id,omitempty"`
	AffectedServices   []string   `json:"affected_services"`
	AffectedUsersCount *int       `json:"affected_users_count,omitempty"`
	BusinessImpact     *string    `json:"business_impact,omitempty"`
	DetectedAt         time.Time  `json:"detected_at"`
	AcknowledgedAt     *time.Time `json:"acknowledged_at,omitempty"`
	IdentifiedAt       *time.Time `json:"identified_at,omitempty"`
	ResolvedAt         *time.Time `json:"resolved_at,omitempty"`
	PostmortemWritten  bool       `json:"postmortem_written"`
	PostmortemDue      *time.Time `json:"postmortem_due,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type TimelineEntry struct {
	ID         string    `json:"id"`
	IncidentID string    `json:"incident_id"`
	OrgID      string    `json:"org_id"`
	AuthorID   *string   `json:"author_id,omitempty"`
	EntryType  string    `json:"entry_type"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

type Postmortem struct {
	ID              string     `json:"id"`
	IncidentID      string     `json:"incident_id"`
	OrgID           string     `json:"org_id"`
	WhatHappened    *string    `json:"what_happened,omitempty"`
	RootCause       *string    `json:"root_cause,omitempty"`
	Detection       *string    `json:"detection,omitempty"`
	Response        *string    `json:"response,omitempty"`
	Prevention      *string    `json:"prevention,omitempty"`
	TimelineSummary *string    `json:"timeline_summary,omitempty"`
	WrittenBy       *string    `json:"written_by,omitempty"`
	ReviewedBy      *string    `json:"reviewed_by,omitempty"`
	Approved        bool       `json:"approved"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Incident statuses
const (
	IncidentInvestigating = "investigating"
	IncidentIdentified    = "identified"
	IncidentMonitoring    = "monitoring"
	IncidentResolved      = "resolved"
	IncidentPostmortem    = "postmortem"
)

// Timeline entry types
const (
	TimelineUpdate       = "update"
	TimelineStatusChange = "status_change"
	TimelineNote         = "note"
	TimelineEscalation   = "escalation"
	TimelineAction       = "action_taken"
	TimelineResolution   = "resolution"
)

// ── Create Incident ───────────────────────────────────────────────────────────

type CreateIncidentParams struct {
	OrgID            string
	Title            string
	Description      *string
	Severity         string
	CommanderID      *string
	AffectedServices []string
	BusinessImpact   *string
}

func CreateIncident(db *sql.DB, p CreateIncidentParams) (*Incident, error) {
	if p.Severity == "" {
		p.Severity = "P1"
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var inc Incident
	err = tx.QueryRow(`
		INSERT INTO incidents (
			org_id, title, description, severity,
			commander_id, affected_services, business_impact
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, org_id, title, description, severity, status,
		          commander_id, affected_services, affected_users_count,
		          business_impact, detected_at, acknowledged_at,
		          identified_at, resolved_at, postmortem_written,
		          postmortem_due, created_at, updated_at`,
		p.OrgID, p.Title, p.Description, p.Severity,
		p.CommanderID, pq.Array(p.AffectedServices), p.BusinessImpact,
	).Scan(
		&inc.ID, &inc.OrgID, &inc.Title, &inc.Description,
		&inc.Severity, &inc.Status,
		&inc.CommanderID, pq.Array(&inc.AffectedServices),
		&inc.AffectedUsersCount, &inc.BusinessImpact,
		&inc.DetectedAt, &inc.AcknowledgedAt,
		&inc.IdentifiedAt, &inc.ResolvedAt,
		&inc.PostmortemWritten, &inc.PostmortemDue,
		&inc.CreatedAt, &inc.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Add initial timeline entry
	_, err = tx.Exec(`
		INSERT INTO incident_timeline (incident_id, org_id, author_id, entry_type, body)
		VALUES ($1, $2, $3, $4, $5)`,
		inc.ID, inc.OrgID, p.CommanderID,
		TimelineUpdate, "Incident declared: "+p.Title,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &inc, nil
}

// ── Read ──────────────────────────────────────────────────────────────────────

func GetIncidentByID(db *sql.DB, orgID, incidentID string) (*Incident, error) {
	var inc Incident
	err := db.QueryRow(`
		SELECT id, org_id, title, description, severity, status,
		       commander_id, affected_services, affected_users_count,
		       business_impact, detected_at, acknowledged_at,
		       identified_at, resolved_at, postmortem_written,
		       postmortem_due, created_at, updated_at
		FROM incidents
		WHERE org_id = $1 AND id = $2`,
		orgID, incidentID,
	).Scan(
		&inc.ID, &inc.OrgID, &inc.Title, &inc.Description,
		&inc.Severity, &inc.Status,
		&inc.CommanderID, pq.Array(&inc.AffectedServices),
		&inc.AffectedUsersCount, &inc.BusinessImpact,
		&inc.DetectedAt, &inc.AcknowledgedAt,
		&inc.IdentifiedAt, &inc.ResolvedAt,
		&inc.PostmortemWritten, &inc.PostmortemDue,
		&inc.CreatedAt, &inc.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inc, nil
}

// ListActiveIncidents returns all non-resolved incidents for an org.
func ListActiveIncidents(db *sql.DB, orgID string) ([]Incident, error) {
	rows, err := db.Query(`
		SELECT id, org_id, title, description, severity, status,
		       commander_id, affected_services, affected_users_count,
		       business_impact, detected_at, acknowledged_at,
		       identified_at, resolved_at, postmortem_written,
		       postmortem_due, created_at, updated_at
		FROM incidents
		WHERE org_id = $1 AND status NOT IN ('resolved', 'postmortem')
		ORDER BY
			CASE severity WHEN 'P0' THEN 0 WHEN 'P1' THEN 1
			WHEN 'P2' THEN 2 ELSE 3 END ASC,
			detected_at ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var incidents []Incident
	for rows.Next() {
		var inc Incident
		if err := rows.Scan(
			&inc.ID, &inc.OrgID, &inc.Title, &inc.Description,
			&inc.Severity, &inc.Status,
			&inc.CommanderID, pq.Array(&inc.AffectedServices),
			&inc.AffectedUsersCount, &inc.BusinessImpact,
			&inc.DetectedAt, &inc.AcknowledgedAt,
			&inc.IdentifiedAt, &inc.ResolvedAt,
			&inc.PostmortemWritten, &inc.PostmortemDue,
			&inc.CreatedAt, &inc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		incidents = append(incidents, inc)
	}
	return incidents, rows.Err()
}

// ListAllIncidents returns all incidents for an org.
func ListAllIncidents(db *sql.DB, orgID string, limit int) ([]Incident, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.Query(`
		SELECT id, org_id, title, description, severity, status,
		       commander_id, affected_services, affected_users_count,
		       business_impact, detected_at, acknowledged_at,
		       identified_at, resolved_at, postmortem_written,
		       postmortem_due, created_at, updated_at
		FROM incidents
		WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2`,
		orgID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var incidents []Incident
	for rows.Next() {
		var inc Incident
		if err := rows.Scan(
			&inc.ID, &inc.OrgID, &inc.Title, &inc.Description,
			&inc.Severity, &inc.Status,
			&inc.CommanderID, pq.Array(&inc.AffectedServices),
			&inc.AffectedUsersCount, &inc.BusinessImpact,
			&inc.DetectedAt, &inc.AcknowledgedAt,
			&inc.IdentifiedAt, &inc.ResolvedAt,
			&inc.PostmortemWritten, &inc.PostmortemDue,
			&inc.CreatedAt, &inc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		incidents = append(incidents, inc)
	}
	return incidents, rows.Err()
}

// ── Update Status ─────────────────────────────────────────────────────────────

func UpdateIncidentStatus(db *sql.DB, orgID, incidentID, authorID, newStatus, note string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update status + relevant timestamp
	switch newStatus {
	case IncidentIdentified:
		_, err = tx.Exec(`
			UPDATE incidents SET status = $1, identified_at = now(), updated_at = now()
			WHERE org_id = $2 AND id = $3`, newStatus, orgID, incidentID)
	case IncidentResolved:
		_, err = tx.Exec(`
			UPDATE incidents SET status = $1, resolved_at = now(),
			       postmortem_due = now() + interval '48 hours', updated_at = now()
			WHERE org_id = $2 AND id = $3`, newStatus, orgID, incidentID)
	default:
		_, err = tx.Exec(`
			UPDATE incidents SET status = $1, updated_at = now()
			WHERE org_id = $2 AND id = $3`, newStatus, orgID, incidentID)
	}
	if err != nil {
		return err
	}

	// Timeline entry
	body := "Status changed to " + newStatus
	if note != "" {
		body = body + ": " + note
	}
	_, err = tx.Exec(`
		INSERT INTO incident_timeline (incident_id, org_id, author_id, entry_type, body)
		VALUES ($1, $2, $3, $4, $5)`,
		incidentID, orgID, authorID, TimelineStatusChange, body,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ── Timeline ──────────────────────────────────────────────────────────────────

func AddTimelineEntry(db *sql.DB, orgID, incidentID, authorID, entryType, body string) error {
	_, err := db.Exec(`
		INSERT INTO incident_timeline (incident_id, org_id, author_id, entry_type, body)
		VALUES ($1, $2, $3, $4, $5)`,
		incidentID, orgID, authorID, entryType, body,
	)
	return err
}

func ListTimeline(db *sql.DB, orgID, incidentID string) ([]TimelineEntry, error) {
	rows, err := db.Query(`
		SELECT id, incident_id, org_id, author_id, entry_type, body, created_at
		FROM incident_timeline
		WHERE org_id = $1 AND incident_id = $2
		ORDER BY created_at ASC`,
		orgID, incidentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []TimelineEntry
	for rows.Next() {
		var e TimelineEntry
		if err := rows.Scan(
			&e.ID, &e.IncidentID, &e.OrgID, &e.AuthorID,
			&e.EntryType, &e.Body, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ── Link Ticket ───────────────────────────────────────────────────────────────

func LinkTicketToIncident(db *sql.DB, orgID, ticketID, incidentID string) error {
	_, err := db.Exec(`
		UPDATE tickets SET incident_id = $1, updated_at = now()
		WHERE org_id = $2 AND id = $3`,
		incidentID, orgID, ticketID,
	)
	return err
}

func GetTicketsForIncident(db *sql.DB, orgID, incidentID string) ([]Ticket, error) {
	rows, err := db.Query(`
		SELECT id, org_id, customer_id, assigned_to,
		       subject, body, status, priority, category,
		       source_type, thread_id, incident_id,
		       sla_due_at, sla_breached,
		       created_at, updated_at, resolved_at
		FROM tickets
		WHERE org_id = $1 AND incident_id = $2
		ORDER BY created_at ASC`,
		orgID, incidentID,
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

// ── Postmortem ────────────────────────────────────────────────────────────────

type CreatePostmortemParams struct {
	IncidentID      string
	OrgID           string
	WhatHappened    *string
	RootCause       *string
	Detection       *string
	Response        *string
	Prevention      *string
	TimelineSummary *string
	WrittenBy       string
}

func CreatePostmortem(db *sql.DB, p CreatePostmortemParams) (*Postmortem, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var pm Postmortem
	err = tx.QueryRow(`
		INSERT INTO postmortems (
			incident_id, org_id, what_happened, root_cause,
			detection, response, prevention, timeline_summary, written_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, incident_id, org_id, what_happened, root_cause,
		          detection, response, prevention, timeline_summary,
		          written_by, reviewed_by, approved, created_at, updated_at`,
		p.IncidentID, p.OrgID, p.WhatHappened, p.RootCause,
		p.Detection, p.Response, p.Prevention, p.TimelineSummary,
		p.WrittenBy,
	).Scan(
		&pm.ID, &pm.IncidentID, &pm.OrgID, &pm.WhatHappened,
		&pm.RootCause, &pm.Detection, &pm.Response, &pm.Prevention,
		&pm.TimelineSummary, &pm.WrittenBy, &pm.ReviewedBy,
		&pm.Approved, &pm.CreatedAt, &pm.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Mark incident as having postmortem
	_, err = tx.Exec(`
		UPDATE incidents SET postmortem_written = true, status = 'postmortem', updated_at = now()
		WHERE org_id = $1 AND id = $2`,
		p.OrgID, p.IncidentID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &pm, nil
}

func GetPostmortemByIncident(db *sql.DB, orgID, incidentID string) (*Postmortem, error) {
	var pm Postmortem
	err := db.QueryRow(`
		SELECT id, incident_id, org_id, what_happened, root_cause,
		       detection, response, prevention, timeline_summary,
		       written_by, reviewed_by, approved, created_at, updated_at
		FROM postmortems
		WHERE org_id = $1 AND incident_id = $2`,
		orgID, incidentID,
	).Scan(
		&pm.ID, &pm.IncidentID, &pm.OrgID, &pm.WhatHappened,
		&pm.RootCause, &pm.Detection, &pm.Response, &pm.Prevention,
		&pm.TimelineSummary, &pm.WrittenBy, &pm.ReviewedBy,
		&pm.Approved, &pm.CreatedAt, &pm.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pm, nil
}
