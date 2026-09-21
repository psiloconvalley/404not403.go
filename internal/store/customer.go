package store

import (
	"database/sql"
	"strings"
	"time"
)

// ── Customer ──────────────────────────────────────────────────────────────────

// Customer represents an employee / customer who submits tickets.
// Scoped to an organization and enriched with enterprise directory context.
type Customer struct {
	ID                   string     `json:"id"`
	OrgID                string     `json:"org_id"`
	Email                *string    `json:"email,omitempty"`
	SlackUserID          *string    `json:"slack_user_id,omitempty"`
	FullName             *string    `json:"full_name,omitempty"`
	FirstName            *string    `json:"first_name,omitempty"`
	LastName             *string    `json:"last_name,omitempty"`
	Position             *string    `json:"position,omitempty"`
	Department           *string    `json:"department,omitempty"`
	DepartmentID         *string    `json:"department_id,omitempty"`
	CostCenter           *string    `json:"cost_center,omitempty"`
	ManagerCustomerID    *string    `json:"manager_customer_id,omitempty"`
	ManagerEmail         *string    `json:"manager_email,omitempty"`
	Location             *string    `json:"location,omitempty"`
	Phone                *string    `json:"phone,omitempty"`
	ExternalID           *string    `json:"external_id,omitempty"`
	Source               string     `json:"source"`
	TrackingTokenHash    *string    `json:"-"`
	TrackingTokenExpires *time.Time `json:"-"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

const customerSelectCols = `
	id, org_id, email, slack_user_id, full_name, first_name, last_name,
	position, department, department_id, cost_center, manager_customer_id,
	manager_email, location, phone, external_id, source,
	created_at, updated_at
`

func scanCustomer(row interface{ Scan(dest ...interface{}) error }, c *Customer) error {
	return row.Scan(
		&c.ID, &c.OrgID, &c.Email, &c.SlackUserID, &c.FullName, &c.FirstName, &c.LastName,
		&c.Position, &c.Department, &c.DepartmentID, &c.CostCenter, &c.ManagerCustomerID,
		&c.ManagerEmail, &c.Location, &c.Phone, &c.ExternalID, &c.Source,
		&c.CreatedAt, &c.UpdatedAt,
	)
}

// ── Queries ───────────────────────────────────────────────────────────────────

// CreateCustomer inserts a new customer record.
func CreateCustomer(db *sql.DB, orgID string, email, slackUserID, fullName, department *string) (*Customer, error) {
	var firstName, lastName *string
	if fullName != nil && *fullName != "" {
		parts := strings.SplitN(strings.TrimSpace(*fullName), " ", 2)
		firstName = &parts[0]
		if len(parts) > 1 {
			lastName = &parts[1]
		}
	}

	var c Customer
	query := `
		INSERT INTO customers (
			org_id, email, slack_user_id, full_name, first_name, last_name, department, source
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'email')
		RETURNING ` + customerSelectCols

	err := scanCustomer(db.QueryRow(query, orgID, email, slackUserID, fullName, firstName, lastName, department), &c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetCustomerByID returns a customer by UUID.
func GetCustomerByID(db *sql.DB, orgID, customerID string) (*Customer, error) {
	var c Customer
	query := `SELECT ` + customerSelectCols + ` FROM customers WHERE org_id = $1 AND id = $2`
	err := scanCustomer(db.QueryRow(query, orgID, customerID), &c)
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
	query := `SELECT ` + customerSelectCols + ` FROM customers WHERE org_id = $1 AND LOWER(email) = LOWER($2)`
	err := scanCustomer(db.QueryRow(query, orgID, email), &c)
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
	query := `SELECT ` + customerSelectCols + ` FROM customers WHERE org_id = $1 AND slack_user_id = $2`
	err := scanCustomer(db.QueryRow(query, orgID, slackUserID), &c)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// FindOrCreateCustomerByEmail finds an existing customer by email or creates one.
func FindOrCreateCustomerByEmail(db *sql.DB, orgID, email string, fullName *string) (*Customer, error) {
	existing, err := GetCustomerByEmail(db, orgID, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	return CreateCustomer(db, orgID, &email, nil, fullName, nil)
}

// FindOrCreateCustomerBySlack finds an existing customer by Slack ID or creates one.
func FindOrCreateCustomerBySlack(db *sql.DB, orgID, slackUserID string, fullName *string) (*Customer, error) {
	existing, err := GetCustomerBySlackUserID(db, orgID, slackUserID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	return CreateCustomer(db, orgID, nil, &slackUserID, fullName, nil)
}

// ListCustomers returns all customers for an org.
func ListCustomers(db *sql.DB, orgID string, limit int) ([]Customer, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := `SELECT ` + customerSelectCols + ` FROM customers WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2`
	rows, err := db.Query(query, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var c Customer
		if err := scanCustomer(rows, &c); err != nil {
			return nil, err
		}
		customers = append(customers, c)
	}
	return customers, rows.Err()
}

// UpdateCustomerProfile updates enriched directory attributes for a customer.
func UpdateCustomerProfile(db *sql.DB, orgID, customerID string, firstName, lastName, position, deptID, costCenter, managerCustID, managerEmail, location, phone *string) error {
	var fullName *string
	if firstName != nil || lastName != nil {
		fn := ""
		ln := ""
		if firstName != nil {
			fn = *firstName
		}
		if lastName != nil {
			ln = *lastName
		}
		combined := strings.TrimSpace(fn + " " + ln)
		if combined != "" {
			fullName = &combined
		}
	}

	query := `
		UPDATE customers
		SET first_name = COALESCE($1, first_name),
		    last_name = COALESCE($2, last_name),
		    full_name = COALESCE($3, full_name),
		    position = COALESCE($4, position),
		    department_id = COALESCE($5, department_id),
		    cost_center = COALESCE($6, cost_center),
		    manager_customer_id = COALESCE($7, manager_customer_id),
		    manager_email = COALESCE($8, manager_email),
		    location = COALESCE($9, location),
		    phone = COALESCE($10, phone),
		    updated_at = now()
		WHERE org_id = $11 AND id = $12`

	_, err := db.Exec(query, firstName, lastName, fullName, position, deptID, costCenter, managerCustID, managerEmail, location, phone, orgID, customerID)
	return err
}

// ── Org Chart Hierarchy Queries ───────────────────────────────────────────────

// GetManagerChain walks up the reporting tree up to maxDepth levels.
// e.g. Sarah -> Bob (VP) -> Alice (CFO)
func GetManagerChain(db *sql.DB, orgID, customerID string, maxDepth int) ([]Customer, error) {
	if maxDepth <= 0 || maxDepth > 10 {
		maxDepth = 5
	}

	query := `
		WITH RECURSIVE manager_tree AS (
			SELECT c.` + strings.ReplaceAll(customerSelectCols, "\n\t", " ") + `, 1 AS depth
			FROM customers c
			WHERE c.org_id = $1 AND c.id = (
				SELECT manager_customer_id FROM customers WHERE id = $2 AND org_id = $1
			)
			UNION ALL
			SELECT parent.` + strings.ReplaceAll(customerSelectCols, "\n\t", " ") + `, mt.depth + 1
			FROM customers parent
			JOIN manager_tree mt ON parent.id = mt.manager_customer_id
			WHERE parent.org_id = $1 AND mt.depth < $3
		)
		SELECT ` + customerSelectCols + ` FROM manager_tree ORDER BY depth ASC`

	rows, err := db.Query(query, orgID, customerID, maxDepth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chain []Customer
	for rows.Next() {
		var c Customer
		if err := scanCustomer(rows, &c); err != nil {
			return nil, err
		}
		chain = append(chain, c)
	}
	return chain, rows.Err()
}

// GetDirectReports returns all employees reporting to this customer.
func GetDirectReports(db *sql.DB, orgID, managerCustomerID string) ([]Customer, error) {
	query := `SELECT ` + customerSelectCols + ` FROM customers WHERE org_id = $1 AND manager_customer_id = $2 ORDER BY full_name ASC`
	rows, err := db.Query(query, orgID, managerCustomerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []Customer
	for rows.Next() {
		var c Customer
		if err := scanCustomer(rows, &c); err != nil {
			return nil, err
		}
		reports = append(reports, c)
	}
	return reports, rows.Err()
}

// ── Tracking Tokens ───────────────────────────────────────────────────────────

// SetTrackingToken stores a hashed tracking token for a customer.
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
func GetCustomerByTrackingToken(db *sql.DB, tokenHash string) (*Customer, error) {
	var c Customer
	query := `SELECT ` + customerSelectCols + ` FROM customers WHERE tracking_token_hash = $1 AND tracking_token_expires > now()`
	err := scanCustomer(db.QueryRow(query, tokenHash), &c)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetTicketsForCustomer returns all tickets submitted by a customer.
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
