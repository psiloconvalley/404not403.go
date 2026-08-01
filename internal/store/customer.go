package store

import (
	"database/sql"
	"time"
)

// ── Customer ──────────────────────────────────────────────────────────────────

// Customer represents a person who submits tickets.
// Customers are scoped to an organization.
// A customer may be identified by email, Slack user ID, or both.
// Customers do not have passwords — they are not agents.
type Customer struct {
	ID                   string     `json:"id"`
	OrgID                string     `json:"org_id"`
	Email                *string    `json:"email,omitempty"`
	SlackUserID          *string    `json:"slack_user_id,omitempty"`
	FullName             *string    `json:"full_name,omitempty"`
	Department           *string    `json:"department,omitempty"`
	TrackingTokenHash    *string    `json:"-"`
	TrackingTokenExpires *time.Time `json:"-"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}


// ── Queries ───────────────────────────────────────────────────────────────────

// CreateCustomer inserts a new customer record.
func CreateCustomer(db *sql.DB, orgID string, email, slackUserID, fullName, department *string) (*Customer, error) {
	var c Customer
	err := db.QueryRow(`
		INSERT INTO customers (org_id, email, slack_user_id, full_name, department)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, org_id, email, slack_user_id, full_name, department,
		          created_at, updated_at`,
		orgID, email, slackUserID, fullName, department,
	).Scan(
		&c.ID, &c.OrgID, &c.Email, &c.SlackUserID,
		&c.FullName, &c.Department,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetCustomerByID returns a customer by UUID.
func GetCustomerByID(db *sql.DB, orgID, customerID string) (*Customer, error) {
	var c Customer
	err := db.QueryRow(`
		SELECT id, org_id, email, slack_user_id, full_name, department,
		       created_at, updated_at
		FROM customers
		WHERE org_id = $1 AND id = $2`,
		orgID, customerID,
	).Scan(
		&c.ID, &c.OrgID, &c.Email, &c.SlackUserID,
		&c.FullName, &c.Department,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetCustomerByEmail returns a customer by email within an org.
func GetCustomerByEmail(db *sql.DB, orgID, email string) (*Customer, error) {
	var c Customer
	err := db.QueryRow(`
		SELECT id, org_id, email, slack_user_id, full_name, department,
		       created_at, updated_at
		FROM customers
		WHERE org_id = $1 AND email = $2`,
		orgID, email,
	).Scan(
		&c.ID, &c.OrgID, &c.Email, &c.SlackUserID,
		&c.FullName, &c.Department,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetCustomerBySlackUserID returns a customer by Slack user ID within an org.
func GetCustomerBySlackUserID(db *sql.DB, orgID, slackUserID string) (*Customer, error) {
	var c Customer
	err := db.QueryRow(`
		SELECT id, org_id, email, slack_user_id, full_name, department,
		       created_at, updated_at
		FROM customers
		WHERE org_id = $1 AND slack_user_id = $2`,
		orgID, slackUserID,
	).Scan(
		&c.ID, &c.OrgID, &c.Email, &c.SlackUserID,
		&c.FullName, &c.Department,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// FindOrCreateCustomerByEmail finds an existing customer by email or creates one.
// This is the idempotent entry point for email ingestion.
func FindOrCreateCustomerByEmail(db *sql.DB, orgID, email string, fullName *string) (*Customer, error) {
	// Try to find existing
	existing, err := GetCustomerByEmail(db, orgID, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	// Create new
	return CreateCustomer(db, orgID, &email, nil, fullName, nil)
}

// FindOrCreateCustomerBySlack finds an existing customer by Slack ID or creates one.
// This is the idempotent entry point for Slack ingestion.
func FindOrCreateCustomerBySlack(db *sql.DB, orgID, slackUserID string, fullName *string) (*Customer, error) {
	// Try to find existing
	existing, err := GetCustomerBySlackUserID(db, orgID, slackUserID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	// Create new
	return CreateCustomer(db, orgID, nil, &slackUserID, fullName, nil)
}

// ListCustomers returns all customers for an org.
func ListCustomers(db *sql.DB, orgID string, limit int) ([]Customer, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := db.Query(`
		SELECT id, org_id, email, slack_user_id, full_name, department,
		       created_at, updated_at
		FROM customers
		WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2`,
		orgID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var c Customer
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.Email, &c.SlackUserID,
			&c.FullName, &c.Department,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		customers = append(customers, c)
	}
	return customers, rows.Err()
}

// UpdateCustomer updates mutable customer fields.
func UpdateCustomer(db *sql.DB, orgID, customerID string, fullName, department *string) error {
	_, err := db.Exec(`
		UPDATE customers
		SET full_name = COALESCE($1, full_name),
		    department = COALESCE($2, department),
		    updated_at = now()
		WHERE org_id = $3 AND id = $4`,
		fullName, department, orgID, customerID,
	)
	return err
}

// ── Tracking Tokens ───────────────────────────────────────────────────────────

// SetTrackingToken stores a hashed tracking token for a customer.
// The raw token is sent via email. Only the hash is stored.
func SetTrackingToken(db *sql.DB, customerID, tokenHash string, expiresAt time.Time) error {
	_, err := db.Exec(`
		UPDATE customers
		SET tracking_token_hash = $1,
		    tracking_token_expires = $2,
		    updated_at = now()
		WHERE id = $3`,
		tokenHash, expiresAt, customerID,
	)
	return err
}

// GetCustomerByTrackingToken looks up a customer by their tracking token hash.
// Returns nil if the token is invalid or expired.
func GetCustomerByTrackingToken(db *sql.DB, tokenHash string) (*Customer, error) {
	var c Customer
	err := db.QueryRow(`
		SELECT id, org_id, email, slack_user_id, full_name, department,
		       tracking_token_hash, tracking_token_expires,
		       created_at, updated_at
		FROM customers
		WHERE tracking_token_hash = $1
		  AND tracking_token_expires > now()`,
		tokenHash,
	).Scan(
		&c.ID, &c.OrgID, &c.Email, &c.SlackUserID,
		&c.FullName, &c.Department,
		&c.TrackingTokenHash, &c.TrackingTokenExpires,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetTicketsForCustomer returns all tickets submitted by a customer.
// Used in the employee tracking portal.
func GetTicketsForCustomer(db *sql.DB, orgID, customerID string) ([]Ticket, error) {
	rows, err := db.Query(`
		SELECT id, org_id, customer_id, assigned_to,
		       subject, body, status, priority, category,
		       source_type, thread_id, incident_id,
		       sla_due_at, sla_breached,
		       created_at, updated_at, resolved_at
		FROM tickets
		WHERE org_id = $1 AND customer_id = $2
		ORDER BY created_at DESC
		LIMIT 50`,
		orgID, customerID,
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
