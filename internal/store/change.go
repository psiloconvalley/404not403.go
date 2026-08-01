package store

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// ── Change Request ────────────────────────────────────────────────────────────

type ChangeRequest struct {
	ID              string     `json:"id"`
	OrgID           string     `json:"org_id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	ChangeType      string     `json:"change_type"`
	RiskLevel       string     `json:"risk_level"`
	AffectedSystems []string   `json:"affected_systems"`
	RollbackPlan    string     `json:"rollback_plan"`
	TestPlan        *string    `json:"test_plan,omitempty"`
	RequestedBy     string     `json:"requested_by"`
	Status          string     `json:"status"`
	ScheduledStart  *time.Time `json:"scheduled_start,omitempty"`
	ScheduledEnd    *time.Time `json:"scheduled_end,omitempty"`
	ActualStart     *time.Time `json:"actual_start,omitempty"`
	ActualEnd       *time.Time `json:"actual_end,omitempty"`
	IncidentID      *string    `json:"incident_id,omitempty"`
	TicketID        *string    `json:"ticket_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ChangeApproval struct {
	ID         string     `json:"id"`
	ChangeID   string     `json:"change_id"`
	OrgID      string     `json:"org_id"`
	ApproverID string     `json:"approver_id"`
	Status     string     `json:"status"`
	Comment    *string    `json:"comment,omitempty"`
	DecidedAt  *time.Time `json:"decided_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Change types
const (
	ChangeTypeStandard  = "standard"
	ChangeTypeEmergency = "emergency"
	ChangeTypeMajor     = "major"
)

// Change statuses
const (
	ChangeStatusDraft           = "draft"
	ChangeStatusPendingApproval = "pending_approval"
	ChangeStatusApproved        = "approved"
	ChangeStatusScheduled       = "scheduled"
	ChangeStatusInProgress      = "in_progress"
	ChangeStatusCompleted       = "completed"
	ChangeStatusFailed          = "failed"
	ChangeStatusRolledBack      = "rolled_back"
	ChangeStatusCancelled       = "cancelled"
)

// ── Create ────────────────────────────────────────────────────────────────────

type CreateChangeParams struct {
	OrgID           string
	Title           string
	Description     string
	ChangeType      string
	RiskLevel       string
	AffectedSystems []string
	RollbackPlan    string
	TestPlan        *string
	RequestedBy     string
	IncidentID      *string
	TicketID        *string
}

func CreateChangeRequest(db *sql.DB, p CreateChangeParams) (*ChangeRequest, error) {
	if p.ChangeType == "" {
		p.ChangeType = ChangeTypeStandard
	}
	if p.RiskLevel == "" {
		p.RiskLevel = "medium"
	}

	var cr ChangeRequest
	err := db.QueryRow(`
		INSERT INTO change_requests (
			org_id, title, description, change_type, risk_level,
			affected_systems, rollback_plan, test_plan,
			requested_by, incident_id, ticket_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, org_id, title, description, change_type, risk_level,
		          affected_systems, rollback_plan, test_plan, requested_by,
		          status, scheduled_start, scheduled_end,
		          actual_start, actual_end, incident_id, ticket_id,
		          created_at, updated_at`,
		p.OrgID, p.Title, p.Description, p.ChangeType, p.RiskLevel,
		pq.Array(p.AffectedSystems), p.RollbackPlan, p.TestPlan,
		p.RequestedBy, p.IncidentID, p.TicketID,
	).Scan(
		&cr.ID, &cr.OrgID, &cr.Title, &cr.Description,
		&cr.ChangeType, &cr.RiskLevel,
		pq.Array(&cr.AffectedSystems), &cr.RollbackPlan, &cr.TestPlan,
		&cr.RequestedBy, &cr.Status,
		&cr.ScheduledStart, &cr.ScheduledEnd,
		&cr.ActualStart, &cr.ActualEnd,
		&cr.IncidentID, &cr.TicketID,
		&cr.CreatedAt, &cr.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &cr, nil
}

// ── Read ──────────────────────────────────────────────────────────────────────

func GetChangeByID(db *sql.DB, orgID, changeID string) (*ChangeRequest, error) {
	var cr ChangeRequest
	err := db.QueryRow(`
		SELECT id, org_id, title, description, change_type, risk_level,
		       affected_systems, rollback_plan, test_plan, requested_by,
		       status, scheduled_start, scheduled_end,
		       actual_start, actual_end, incident_id, ticket_id,
		       created_at, updated_at
		FROM change_requests
		WHERE org_id = $1 AND id = $2`,
		orgID, changeID,
	).Scan(
		&cr.ID, &cr.OrgID, &cr.Title, &cr.Description,
		&cr.ChangeType, &cr.RiskLevel,
		pq.Array(&cr.AffectedSystems), &cr.RollbackPlan, &cr.TestPlan,
		&cr.RequestedBy, &cr.Status,
		&cr.ScheduledStart, &cr.ScheduledEnd,
		&cr.ActualStart, &cr.ActualEnd,
		&cr.IncidentID, &cr.TicketID,
		&cr.CreatedAt, &cr.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cr, nil
}

func ListChangeRequests(db *sql.DB, orgID string, limit int) ([]ChangeRequest, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.Query(`
		SELECT id, org_id, title, description, change_type, risk_level,
		       affected_systems, rollback_plan, test_plan, requested_by,
		       status, scheduled_start, scheduled_end,
		       actual_start, actual_end, incident_id, ticket_id,
		       created_at, updated_at
		FROM change_requests
		WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2`,
		orgID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var changes []ChangeRequest
	for rows.Next() {
		var cr ChangeRequest
		if err := rows.Scan(
			&cr.ID, &cr.OrgID, &cr.Title, &cr.Description,
			&cr.ChangeType, &cr.RiskLevel,
			pq.Array(&cr.AffectedSystems), &cr.RollbackPlan, &cr.TestPlan,
			&cr.RequestedBy, &cr.Status,
			&cr.ScheduledStart, &cr.ScheduledEnd,
			&cr.ActualStart, &cr.ActualEnd,
			&cr.IncidentID, &cr.TicketID,
			&cr.CreatedAt, &cr.UpdatedAt,
		); err != nil {
			return nil, err
		}
		changes = append(changes, cr)
	}
	return changes, rows.Err()
}

func ListPendingApprovals(db *sql.DB, orgID string) ([]ChangeRequest, error) {
	rows, err := db.Query(`
		SELECT id, org_id, title, description, change_type, risk_level,
		       affected_systems, rollback_plan, test_plan, requested_by,
		       status, scheduled_start, scheduled_end,
		       actual_start, actual_end, incident_id, ticket_id,
		       created_at, updated_at
		FROM change_requests
		WHERE org_id = $1 AND status = 'pending_approval'
		ORDER BY created_at ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var changes []ChangeRequest
	for rows.Next() {
		var cr ChangeRequest
		if err := rows.Scan(
			&cr.ID, &cr.OrgID, &cr.Title, &cr.Description,
			&cr.ChangeType, &cr.RiskLevel,
			pq.Array(&cr.AffectedSystems), &cr.RollbackPlan, &cr.TestPlan,
			&cr.RequestedBy, &cr.Status,
			&cr.ScheduledStart, &cr.ScheduledEnd,
			&cr.ActualStart, &cr.ActualEnd,
			&cr.IncidentID, &cr.TicketID,
			&cr.CreatedAt, &cr.UpdatedAt,
		); err != nil {
			return nil, err
		}
		changes = append(changes, cr)
	}
	return changes, rows.Err()
}

// ── Update Status ─────────────────────────────────────────────────────────────

func UpdateChangeStatus(db *sql.DB, orgID, changeID, newStatus string) error {
	query := `UPDATE change_requests SET status = $1, updated_at = now() WHERE org_id = $2 AND id = $3`

	switch newStatus {
	case ChangeStatusInProgress:
		query = `UPDATE change_requests SET status = $1, actual_start = now(), updated_at = now() WHERE org_id = $2 AND id = $3`
	case ChangeStatusCompleted:
		query = `UPDATE change_requests SET status = $1, actual_end = now(), updated_at = now() WHERE org_id = $2 AND id = $3`
	}

	_, err := db.Exec(query, newStatus, orgID, changeID)
	return err
}

func SubmitForApproval(db *sql.DB, orgID, changeID string) error {
	_, err := db.Exec(`
		UPDATE change_requests
		SET status = 'pending_approval', updated_at = now()
		WHERE org_id = $1 AND id = $2 AND status = 'draft'`,
		orgID, changeID,
	)
	return err
}

// ── Approvals ─────────────────────────────────────────────────────────────────

func RequestApproval(db *sql.DB, orgID, changeID, approverID string) error {
	_, err := db.Exec(`
		INSERT INTO change_approvals (change_id, org_id, approver_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`,
		changeID, orgID, approverID,
	)
	return err
}

func ApproveChange(db *sql.DB, orgID, changeID, approverID string, comment *string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		UPDATE change_approvals
		SET status = 'approved', comment = $1, decided_at = now()
		WHERE change_id = $2 AND org_id = $3 AND approver_id = $4`,
		comment, changeID, orgID, approverID,
	)
	if err != nil {
		return err
	}

	// Check if all approvals are approved
	var pending int
	err = tx.QueryRow(`
		SELECT COUNT(*) FROM change_approvals
		WHERE change_id = $1 AND org_id = $2 AND status = 'pending'`,
		changeID, orgID,
	).Scan(&pending)
	if err != nil {
		return err
	}

	// If all approved, update change status
	if pending == 0 {
		_, err = tx.Exec(`
			UPDATE change_requests
			SET status = 'approved', updated_at = now()
			WHERE org_id = $1 AND id = $2`,
			orgID, changeID,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func RejectChange(db *sql.DB, orgID, changeID, approverID string, comment *string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		UPDATE change_approvals
		SET status = 'rejected', comment = $1, decided_at = now()
		WHERE change_id = $2 AND org_id = $3 AND approver_id = $4`,
		comment, changeID, orgID, approverID,
	)
	if err != nil {
		return err
	}

	// Any rejection cancels the change
	_, err = tx.Exec(`
		UPDATE change_requests
		SET status = 'cancelled', updated_at = now()
		WHERE org_id = $1 AND id = $2`,
		orgID, changeID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func ListApprovalsForChange(db *sql.DB, orgID, changeID string) ([]ChangeApproval, error) {
	rows, err := db.Query(`
		SELECT id, change_id, org_id, approver_id, status,
		       comment, decided_at, created_at
		FROM change_approvals
		WHERE org_id = $1 AND change_id = $2
		ORDER BY created_at ASC`,
		orgID, changeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []ChangeApproval
	for rows.Next() {
		var a ChangeApproval
		if err := rows.Scan(
			&a.ID, &a.ChangeID, &a.OrgID, &a.ApproverID,
			&a.Status, &a.Comment, &a.DecidedAt, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		approvals = append(approvals, a)
	}
	return approvals, rows.Err()
}
