package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ── ConfigItem ────────────────────────────────────────────────────────────────

// ConfigItem is any trackable asset or service in an organization.
// Laptops, servers, SaaS subscriptions, network equipment, software licenses.
// Core fields are typed columns. Variable fields live in Metadata (JSONB).
type ConfigItem struct {
	ID           string          `json:"id"`
	OrgID        string          `json:"org_id"`
	CIType       string          `json:"ci_type"`
	Name         string          `json:"name"`
	AssetTag     *string         `json:"asset_tag,omitempty"`
	SerialNumber *string         `json:"serial_number,omitempty"`
	AssignedTo   *string         `json:"assigned_to,omitempty"` // customer UUID
	ManagedBy    *string         `json:"managed_by,omitempty"`  // agent UUID
	Status       string          `json:"status"`
	Metadata     json.RawMessage `json:"metadata"`
	PurchasedAt  *time.Time      `json:"purchased_at,omitempty"`
	WarrantyEnds *time.Time      `json:"warranty_ends,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// Known CI types. Not an enum — new types can be added without migration.
// These constants prevent string drift across handlers and services.
const (
	CITypeLaptop        = "laptop"
	CITypeDesktop       = "desktop"
	CITypeServer        = "server"
	CITypePhone         = "phone"
	CITypeTablet        = "tablet"
	CITypeMonitor       = "monitor"
	CITypeNetworkDevice = "network_device"
	CITypePrinter       = "printer"
	CITypeSaaSService   = "saas_service"
	CITypeSoftware      = "software"
	CITypeLicense       = "license"
	CITypeAccessory     = "accessory"
)

// Known CI statuses.
const (
	CIStatusProcurement    = "procurement"
	CIStatusProvisioning   = "provisioning"
	CIStatusActive         = "active"
	CIStatusMaintenance    = "maintenance"
	CIStatusDecommissioned = "decommissioned"
	CIStatusRetired        = "retired"
)

// ── Create ────────────────────────────────────────────────────────────────────

// CreateConfigItemParams contains everything needed to register a new CI.
type CreateConfigItemParams struct {
	OrgID        string
	CIType       string
	Name         string
	AssetTag     *string
	SerialNumber *string
	AssignedTo   *string         // customer UUID
	ManagedBy    *string         // agent UUID
	Metadata     json.RawMessage // optional, defaults to {}
	PurchasedAt  *time.Time
	WarrantyEnds *time.Time
}

// CreateConfigItem inserts a new configuration item.
func CreateConfigItem(db *sql.DB, p CreateConfigItemParams) (*ConfigItem, error) {
	if p.Metadata == nil {
		p.Metadata = json.RawMessage(`{}`)
	}

	var ci ConfigItem
	err := db.QueryRow(`
		INSERT INTO config_items (
			org_id, ci_type, name, asset_tag, serial_number,
			assigned_to, managed_by, metadata,
			purchased_at, warranty_ends
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, org_id, ci_type, name, asset_tag, serial_number,
		          assigned_to, managed_by, status, metadata,
		          purchased_at, warranty_ends, created_at, updated_at`,
		p.OrgID, p.CIType, p.Name, p.AssetTag, p.SerialNumber,
		p.AssignedTo, p.ManagedBy, p.Metadata,
		p.PurchasedAt, p.WarrantyEnds,
	).Scan(
		&ci.ID, &ci.OrgID, &ci.CIType, &ci.Name,
		&ci.AssetTag, &ci.SerialNumber,
		&ci.AssignedTo, &ci.ManagedBy, &ci.Status, &ci.Metadata,
		&ci.PurchasedAt, &ci.WarrantyEnds,
		&ci.CreatedAt, &ci.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &ci, nil
}

// ── Read ──────────────────────────────────────────────────────────────────────

// GetConfigItemByID returns a config item by UUID, scoped to org.
func GetConfigItemByID(db *sql.DB, orgID, ciID string) (*ConfigItem, error) {
	var ci ConfigItem
	err := db.QueryRow(`
		SELECT id, org_id, ci_type, name, asset_tag, serial_number,
		       assigned_to, managed_by, status, metadata,
		       purchased_at, warranty_ends, created_at, updated_at
		FROM config_items
		WHERE org_id = $1 AND id = $2`,
		orgID, ciID,
	).Scan(
		&ci.ID, &ci.OrgID, &ci.CIType, &ci.Name,
		&ci.AssetTag, &ci.SerialNumber,
		&ci.AssignedTo, &ci.ManagedBy, &ci.Status, &ci.Metadata,
		&ci.PurchasedAt, &ci.WarrantyEnds,
		&ci.CreatedAt, &ci.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ci, nil
}

// ── List ──────────────────────────────────────────────────────────────────────

// ListConfigItemsParams allows filtering CI lists.
type ListConfigItemsParams struct {
	OrgID      string
	CIType     *string // filter by type
	Status     *string // filter by status
	AssignedTo *string // filter by customer
	ManagedBy  *string // filter by agent
	Limit      int
	Offset     int
}

// ListConfigItems returns config items for an org with optional filters.
func ListConfigItems(db *sql.DB, p ListConfigItemsParams) ([]ConfigItem, error) {
	if p.Limit <= 0 || p.Limit > 500 {
		p.Limit = 100
	}

	query := `
		SELECT id, org_id, ci_type, name, asset_tag, serial_number,
		       assigned_to, managed_by, status, metadata,
		       purchased_at, warranty_ends, created_at, updated_at
		FROM config_items
		WHERE org_id = $1`

	args := []interface{}{p.OrgID}
	argIdx := 2

	if p.CIType != nil {
		query += fmt.Sprintf(" AND ci_type = $%d", argIdx)
		args = append(args, *p.CIType)
		argIdx++
	}

	if p.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, *p.Status)
		argIdx++
	}

	if p.AssignedTo != nil {
		query += fmt.Sprintf(" AND assigned_to = $%d", argIdx)
		args = append(args, *p.AssignedTo)
		argIdx++
	}

	if p.ManagedBy != nil {
		query += fmt.Sprintf(" AND managed_by = $%d", argIdx)
		args = append(args, *p.ManagedBy)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY name ASC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, p.Limit, p.Offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ConfigItem
	for rows.Next() {
		var ci ConfigItem
		if err := rows.Scan(
			&ci.ID, &ci.OrgID, &ci.CIType, &ci.Name,
			&ci.AssetTag, &ci.SerialNumber,
			&ci.AssignedTo, &ci.ManagedBy, &ci.Status, &ci.Metadata,
			&ci.PurchasedAt, &ci.WarrantyEnds,
			&ci.CreatedAt, &ci.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, ci)
	}
	return items, rows.Err()
}

// GetConfigItemsByCustomer returns all CIs assigned to a specific customer.
// Used to provide context when a ticket is created — what does this person own?
func GetConfigItemsByCustomer(db *sql.DB, orgID, customerID string) ([]ConfigItem, error) {
	rows, err := db.Query(`
		SELECT id, org_id, ci_type, name, asset_tag, serial_number,
		       assigned_to, managed_by, status, metadata,
		       purchased_at, warranty_ends, created_at, updated_at
		FROM config_items
		WHERE org_id = $1 AND assigned_to = $2
		ORDER BY ci_type ASC, name ASC`,
		orgID, customerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ConfigItem
	for rows.Next() {
		var ci ConfigItem
		if err := rows.Scan(
			&ci.ID, &ci.OrgID, &ci.CIType, &ci.Name,
			&ci.AssetTag, &ci.SerialNumber,
			&ci.AssignedTo, &ci.ManagedBy, &ci.Status, &ci.Metadata,
			&ci.PurchasedAt, &ci.WarrantyEnds,
			&ci.CreatedAt, &ci.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, ci)
	}
	return items, rows.Err()
}

// ── Update ────────────────────────────────────────────────────────────────────

// UpdateConfigItemStatus changes a CI's lifecycle status.
func UpdateConfigItemStatus(db *sql.DB, orgID, ciID, newStatus string) error {
	_, err := db.Exec(`
		UPDATE config_items
		SET status = $1, updated_at = now()
		WHERE org_id = $2 AND id = $3`,
		newStatus, orgID, ciID,
	)
	return err
}

// UpdateConfigItemAssignment changes who a CI is assigned to.
func UpdateConfigItemAssignment(db *sql.DB, orgID, ciID string, customerID *string) error {
	_, err := db.Exec(`
		UPDATE config_items
		SET assigned_to = $1, updated_at = now()
		WHERE org_id = $2 AND id = $3`,
		customerID, orgID, ciID,
	)
	return err
}

// UpdateConfigItemMetadata replaces the metadata JSONB on a CI.
func UpdateConfigItemMetadata(db *sql.DB, orgID, ciID string, metadata json.RawMessage) error {
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}
	_, err := db.Exec(`
		UPDATE config_items
		SET metadata = $1, updated_at = now()
		WHERE org_id = $2 AND id = $3`,
		metadata, orgID, ciID,
	)
	return err
}

// ── Ticket ↔ CI Linking ───────────────────────────────────────────────────────

// LinkTicketToCI creates a relationship between a ticket and a config item.
// Idempotent — linking the same pair twice is a no-op.
func LinkTicketToCI(db *sql.DB, ticketID, ciID string) error {
	_, err := db.Exec(`
		INSERT INTO ticket_config_items (ticket_id, ci_id)
		VALUES ($1, $2)
		ON CONFLICT (ticket_id, ci_id) DO NOTHING`,
		ticketID, ciID,
	)
	return err
}

// UnlinkTicketFromCI removes a relationship between a ticket and a config item.
func UnlinkTicketFromCI(db *sql.DB, ticketID, ciID string) error {
	_, err := db.Exec(`
		DELETE FROM ticket_config_items
		WHERE ticket_id = $1 AND ci_id = $2`,
		ticketID, ciID,
	)
	return err
}

// GetConfigItemsForTicket returns all CIs linked to a ticket.
func GetConfigItemsForTicket(db *sql.DB, orgID, ticketID string) ([]ConfigItem, error) {
	rows, err := db.Query(`
		SELECT ci.id, ci.org_id, ci.ci_type, ci.name,
		       ci.asset_tag, ci.serial_number,
		       ci.assigned_to, ci.managed_by, ci.status, ci.metadata,
		       ci.purchased_at, ci.warranty_ends,
		       ci.created_at, ci.updated_at
		FROM config_items ci
		JOIN ticket_config_items tci ON tci.ci_id = ci.id
		WHERE ci.org_id = $1 AND tci.ticket_id = $2
		ORDER BY ci.ci_type ASC, ci.name ASC`,
		orgID, ticketID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ConfigItem
	for rows.Next() {
		var ci ConfigItem
		if err := rows.Scan(
			&ci.ID, &ci.OrgID, &ci.CIType, &ci.Name,
			&ci.AssetTag, &ci.SerialNumber,
			&ci.AssignedTo, &ci.ManagedBy, &ci.Status, &ci.Metadata,
			&ci.PurchasedAt, &ci.WarrantyEnds,
			&ci.CreatedAt, &ci.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, ci)
	}
	return items, rows.Err()
}
