package store

import (
	"database/sql"
	"time"
)

// ── Department ────────────────────────────────────────────────────────────────

// Department represents a top-level corporate division in an organization.
// Groups queues and agents into logical boundaries (e.g., HR, IT, Finance).
type Department struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	HeadUserID  *string   `json:"head_user_id,omitempty"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DepartmentQueue represents a junction mapping connecting a queue to a department.
type DepartmentQueue struct {
	DepartmentID string    `json:"department_id"`
	QueueID      string    `json:"queue_id"`
	Relationship string    `json:"relationship"` // 'owner', 'consumer', 'observer'
	CreatedAt    time.Time `json:"created_at"`
}

// CreateDepartmentParams has options for creating a department.
type CreateDepartmentParams struct {
	OrgID       string
	Name        string
	Description *string
	HeadUserID  *string
}

// CreateDepartment inserts a new department record.
func CreateDepartment(db *sql.DB, p CreateDepartmentParams) (*Department, error) {
	var d Department
	err := db.QueryRow(`
		INSERT INTO departments (org_id, name, description, head_user_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, org_id, name, description, head_user_id, active, created_at, updated_at`,
		p.OrgID, p.Name, p.Description, p.HeadUserID,
	).Scan(
		&d.ID, &d.OrgID, &d.Name, &d.Description, &d.HeadUserID, &d.Active, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// GetDepartment retrieves a single department by its ID.
func GetDepartment(db *sql.DB, id string) (*Department, error) {
	var d Department
	err := db.QueryRow(`
		SELECT id, org_id, name, description, head_user_id, active, created_at, updated_at
		FROM departments
		WHERE id = $1`,
		id,
	).Scan(
		&d.ID, &d.OrgID, &d.Name, &d.Description, &d.HeadUserID, &d.Active, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ListDepartments retrieves all active departments in an organization.
func ListDepartments(db *sql.DB, orgID string) ([]*Department, error) {
	rows, err := db.Query(`
		SELECT id, org_id, name, description, head_user_id, active, created_at, updated_at
		FROM departments
		WHERE org_id = $1 AND active = true
		ORDER BY name ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var depts []*Department
	for rows.Next() {
		var d Department
		err := rows.Scan(
			&d.ID, &d.OrgID, &d.Name, &d.Description, &d.HeadUserID, &d.Active, &d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		depts = append(depts, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return depts, nil
}

// UpdateDepartmentParams has options for updating a department.
type UpdateDepartmentParams struct {
	ID          string
	Name        string
	Description *string
	HeadUserID  *string
	Active      bool
}

// UpdateDepartment updates a department's details.
func UpdateDepartment(db *sql.DB, p UpdateDepartmentParams) (*Department, error) {
	var d Department
	err := db.QueryRow(`
		UPDATE departments
		SET name = $1, description = $2, head_user_id = $3, active = $4, updated_at = now()
		WHERE id = $5
		RETURNING id, org_id, name, description, head_user_id, active, created_at, updated_at`,
		p.Name, p.Description, p.HeadUserID, p.Active, p.ID,
	).Scan(
		&d.ID, &d.OrgID, &d.Name, &d.Description, &d.HeadUserID, &d.Active, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ── Department Queue Junctions ────────────────────────────────────────────────

// AddQueueToDepartment maps a queue to a department with a specified role.
func AddQueueToDepartment(db *sql.DB, departmentID, queueID, relationship string) error {
	if relationship == "" {
		relationship = "owner"
	}
	_, err := db.Exec(`
		INSERT INTO department_queues (department_id, queue_id, relationship)
		VALUES ($1, $2, $3)
		ON CONFLICT (department_id, queue_id) 
		DO UPDATE SET relationship = EXCLUDED.relationship`,
		departmentID, queueID, relationship,
	)
	return err
}

// RemoveQueueFromDepartment breaks the connection between a queue and a department.
func RemoveQueueFromDepartment(db *sql.DB, departmentID, queueID string) error {
	_, err := db.Exec(`
		DELETE FROM department_queues
		WHERE department_id = $1 AND queue_id = $2`,
		departmentID, queueID,
	)
	return err
}

// GetQueuesForDepartment lists all queues mapped to a department.
func GetQueuesForDepartment(db *sql.DB, departmentID string) ([]*Queue, error) {
	rows, err := db.Query(`
		SELECT q.id, q.org_id, q.name, q.prefix, q.description, q.department, q.color, q.icon,
		       q.filters, q.sla_config, q.active, q.sort_order, q.visibility, q.created_by,
		       q.created_at, q.updated_at
		FROM queues q
		JOIN department_queues dq ON q.id = dq.queue_id
		WHERE dq.department_id = $1 AND q.active = true
		ORDER BY q.sort_order ASC, q.name ASC`,
		departmentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queues []*Queue
	for rows.Next() {
		var q Queue
		err := rows.Scan(
			&q.ID, &q.OrgID, &q.Name, &q.Prefix, &q.Description, &q.Department, &q.Color, &q.Icon,
			&q.Filters, &q.SLAConfig, &q.Active, &q.SortOrder, &q.Visibility, &q.CreatedBy,
			&q.CreatedAt, &q.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		queues = append(queues, &q)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return queues, nil
}

// GetDepartmentsForQueue lists all departments mapped to a specific queue.
func GetDepartmentsForQueue(db *sql.DB, queueID string) ([]*Department, error) {
	rows, err := db.Query(`
		SELECT d.id, d.org_id, d.name, d.description, d.head_user_id, d.active, d.created_at, d.updated_at
		FROM departments d
		JOIN department_queues dq ON d.id = dq.department_id
		WHERE dq.queue_id = $1 AND d.active = true
		ORDER BY d.name ASC`,
		queueID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var depts []*Department
	for rows.Next() {
		var d Department
		err := rows.Scan(
			&d.ID, &d.OrgID, &d.Name, &d.Description, &d.HeadUserID, &d.Active, &d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		depts = append(depts, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return depts, nil
}
