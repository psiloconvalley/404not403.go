package store

import (
	"database/sql"
	"time"
)

// CatalogItem is a service catalog entry — a template for ticket creation.
// Each item defines the type, queue, priority, and SLA for a specific service.
type CatalogItem struct {
	ID              string     `json:"id"`
	OrgID           string     `json:"org_id"`
	QueueID         *string    `json:"queue_id,omitempty"`
	Name            string     `json:"name"`
	Description     *string    `json:"description,omitempty"`
	Category        *string    `json:"category,omitempty"`
	TicketType      string     `json:"ticket_type"`
	DefaultPriority string     `json:"default_priority"`
	SLAHours        *int       `json:"sla_hours,omitempty"`
	FormFields      []byte     `json:"form_fields,omitempty"`
	Icon            *string    `json:"icon,omitempty"`
	Active          bool       `json:"active"`
	SortOrder       int        `json:"sort_order"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// CreateCatalogItem inserts a new service catalog entry.
func CreateCatalogItem(db *sql.DB, orgID string, queueID *string, name string, description, category *string, ticketType, defaultPriority string, slaHours *int, icon *string) (*CatalogItem, error) {
	var item CatalogItem
	err := db.QueryRow(`
		INSERT INTO catalog_items (org_id, queue_id, name, description, category, ticket_type, default_priority, sla_hours, icon)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, org_id, queue_id, name, description, category,
		          ticket_type, default_priority, sla_hours, form_fields,
		          icon, active, sort_order, created_at, updated_at`,
		orgID, queueID, name, description, category, ticketType, defaultPriority, slaHours, icon,
	).Scan(
		&item.ID, &item.OrgID, &item.QueueID, &item.Name, &item.Description, &item.Category,
		&item.TicketType, &item.DefaultPriority, &item.SLAHours, &item.FormFields,
		&item.Icon, &item.Active, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// GetCatalogItem returns a single catalog item by ID, scoped to org.
func GetCatalogItem(db *sql.DB, orgID, itemID string) (*CatalogItem, error) {
	var item CatalogItem
	err := db.QueryRow(`
		SELECT id, org_id, queue_id, name, description, category,
		       ticket_type, default_priority, sla_hours, form_fields,
		       icon, active, sort_order, created_at, updated_at
		FROM catalog_items
		WHERE org_id = $1 AND id = $2`,
		orgID, itemID,
	).Scan(
		&item.ID, &item.OrgID, &item.QueueID, &item.Name, &item.Description, &item.Category,
		&item.TicketType, &item.DefaultPriority, &item.SLAHours, &item.FormFields,
		&item.Icon, &item.Active, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// ListCatalogItems returns all active catalog items for an org, grouped by category.
func ListCatalogItems(db *sql.DB, orgID string) ([]CatalogItem, error) {
	rows, err := db.Query(`
		SELECT id, org_id, queue_id, name, description, category,
		       ticket_type, default_priority, sla_hours, form_fields,
		       icon, active, sort_order, created_at, updated_at
		FROM catalog_items
		WHERE org_id = $1 AND active = true
		ORDER BY category ASC NULLS LAST, sort_order ASC, name ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CatalogItem
	for rows.Next() {
		var item CatalogItem
		if err := rows.Scan(
			&item.ID, &item.OrgID, &item.QueueID, &item.Name, &item.Description, &item.Category,
			&item.TicketType, &item.DefaultPriority, &item.SLAHours, &item.FormFields,
			&item.Icon, &item.Active, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListCatalogItemsByQueue returns catalog items for a specific queue.
func ListCatalogItemsByQueue(db *sql.DB, orgID, queueID string) ([]CatalogItem, error) {
	rows, err := db.Query(`
		SELECT id, org_id, queue_id, name, description, category,
		       ticket_type, default_priority, sla_hours, form_fields,
		       icon, active, sort_order, created_at, updated_at
		FROM catalog_items
		WHERE org_id = $1 AND queue_id = $2 AND active = true
		ORDER BY sort_order ASC, name ASC`,
		orgID, queueID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CatalogItem
	for rows.Next() {
		var item CatalogItem
		if err := rows.Scan(
			&item.ID, &item.OrgID, &item.QueueID, &item.Name, &item.Description, &item.Category,
			&item.TicketType, &item.DefaultPriority, &item.SLAHours, &item.FormFields,
			&item.Icon, &item.Active, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpdateCatalogItem updates mutable fields on a catalog item.
func UpdateCatalogItem(db *sql.DB, orgID, itemID string, name, description, category, ticketType, defaultPriority, icon *string, queueID *string, slaHours *int) error {
	_, err := db.Exec(`
		UPDATE catalog_items
		SET name             = COALESCE($1, name),
		    description      = COALESCE($2, description),
		    category         = COALESCE($3, category),
		    ticket_type      = COALESCE($4, ticket_type),
		    default_priority = COALESCE($5, default_priority),
		    icon             = COALESCE($6, icon),
		    queue_id         = COALESCE($7, queue_id),
		    sla_hours        = COALESCE($8, sla_hours),
		    updated_at       = now()
		WHERE org_id = $9 AND id = $10`,
		name, description, category, ticketType, defaultPriority, icon, queueID, slaHours, orgID, itemID,
	)
	return err
}

// DeactivateCatalogItem soft-deletes a catalog item.
func DeactivateCatalogItem(db *sql.DB, orgID, itemID string) error {
	_, err := db.Exec(`
		UPDATE catalog_items
		SET active = false, updated_at = now()
		WHERE org_id = $1 AND id = $2`,
		orgID, itemID,
	)
	return err
}
