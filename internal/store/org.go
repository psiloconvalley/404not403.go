package store

import (
	"database/sql"
	"time"
)

// ── Organization ──────────────────────────────────────────────────────────────

// Organization is a tenant. Every ticket, customer, agent, and config item
// belongs to exactly one organization. Nothing crosses org boundaries.
type Organization struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Slug            string     `json:"slug"`
	Domain          *string    `json:"domain,omitempty"`
	Plan            string     `json:"plan"`
	InboundEmail    *string    `json:"inbound_email,omitempty"`
	SlackTeamID     *string    `json:"slack_team_id,omitempty"`
	SlackChannelID  *string    `json:"slack_channel_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// OrgMember represents a user's membership and role within an organization.
type OrgMember struct {
	OrgID     string    `json:"org_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// OrgMemberDetail joins org_members with users for display purposes.
type OrgMemberDetail struct {
	OrgID     string    `json:"org_id"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Handle    string    `json:"handle"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// ── Queries ───────────────────────────────────────────────────────────────────

// CreateOrg inserts a new organization and adds the creator as owner.
// Both operations are wrapped in a transaction — either both succeed or neither does.
func CreateOrg(db *sql.DB, name, slug, ownerUserID string) (*Organization, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var org Organization
	err = tx.QueryRow(`
		INSERT INTO organizations (name, slug)
		VALUES ($1, $2)
		RETURNING id, name, slug, domain, plan,
		          inbound_email, slack_team_id, slack_channel_id,
		          created_at, updated_at`,
		name, slug,
	).Scan(
		&org.ID, &org.Name, &org.Slug, &org.Domain, &org.Plan,
		&org.InboundEmail, &org.SlackTeamID, &org.SlackChannelID,
		&org.CreatedAt, &org.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`
		INSERT INTO org_members (org_id, user_id, role)
		VALUES ($1, $2, 'owner')`,
		org.ID, ownerUserID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &org, nil
}

// GetOrgByID returns an organization by its UUID.
func GetOrgByID(db *sql.DB, orgID string) (*Organization, error) {
	var org Organization
	err := db.QueryRow(`
		SELECT id, name, slug, domain, plan,
		       inbound_email, slack_team_id, slack_channel_id,
		       created_at, updated_at
		FROM organizations
		WHERE id = $1`,
		orgID,
	).Scan(
		&org.ID, &org.Name, &org.Slug, &org.Domain, &org.Plan,
		&org.InboundEmail, &org.SlackTeamID, &org.SlackChannelID,
		&org.CreatedAt, &org.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// GetOrgBySlug returns an organization by its slug.
func GetOrgBySlug(db *sql.DB, slug string) (*Organization, error) {
	var org Organization
	err := db.QueryRow(`
		SELECT id, name, slug, domain, plan,
		       inbound_email, slack_team_id, slack_channel_id,
		       created_at, updated_at
		FROM organizations
		WHERE slug = $1`,
		slug,
	).Scan(
		&org.ID, &org.Name, &org.Slug, &org.Domain, &org.Plan,
		&org.InboundEmail, &org.SlackTeamID, &org.SlackChannelID,
		&org.CreatedAt, &org.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// GetOrgByInboundEmail returns an organization by its inbound email address.
// Used during email ingestion to route inbound messages to the correct org.
func GetOrgByInboundEmail(db *sql.DB, email string) (*Organization, error) {
	var org Organization
	err := db.QueryRow(`
		SELECT id, name, slug, domain, plan,
		       inbound_email, slack_team_id, slack_channel_id,
		       created_at, updated_at
		FROM organizations
		WHERE inbound_email = $1`,
		email,
	).Scan(
		&org.ID, &org.Name, &org.Slug, &org.Domain, &org.Plan,
		&org.InboundEmail, &org.SlackTeamID, &org.SlackChannelID,
		&org.CreatedAt, &org.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// GetOrgsForUser returns all organizations a user belongs to,
// including their role in each org.
func GetOrgsForUser(db *sql.DB, userID string) ([]OrgMemberDetail, error) {
	rows, err := db.Query(`
		SELECT o.id, u.id, u.email, u.handle, m.role, m.created_at
		FROM org_members m
		JOIN organizations o ON o.id = m.org_id
		JOIN users u ON u.id = m.user_id
		WHERE m.user_id = $1
		ORDER BY m.created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []OrgMemberDetail
	for rows.Next() {
		var d OrgMemberDetail
		if err := rows.Scan(
			&d.OrgID, &d.UserID, &d.Email, &d.Handle,
			&d.Role, &d.CreatedAt,
		); err != nil {
			return nil, err
		}
		members = append(members, d)
	}
	return members, rows.Err()
}

// GetOrgMember returns a user's membership in a specific org.
// Returns nil if the user is not a member.
func GetOrgMember(db *sql.DB, orgID, userID string) (*OrgMember, error) {
	var m OrgMember
	err := db.QueryRow(`
		SELECT org_id, user_id, role, created_at
		FROM org_members
		WHERE org_id = $1 AND user_id = $2`,
		orgID, userID,
	).Scan(&m.OrgID, &m.UserID, &m.Role, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListOrgMembers returns all members of an organization.
func ListOrgMembers(db *sql.DB, orgID string) ([]OrgMemberDetail, error) {
	rows, err := db.Query(`
		SELECT o.id, u.id, u.email, u.handle, m.role, m.created_at
		FROM org_members m
		JOIN organizations o ON o.id = m.org_id
		JOIN users u ON u.id = m.user_id
		WHERE m.org_id = $1
		ORDER BY m.role ASC, m.created_at ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []OrgMemberDetail
	for rows.Next() {
		var d OrgMemberDetail
		if err := rows.Scan(
			&d.OrgID, &d.UserID, &d.Email, &d.Handle,
			&d.Role, &d.CreatedAt,
		); err != nil {
			return nil, err
		}
		members = append(members, d)
	}
	return members, rows.Err()
}

// AddOrgMember adds a user to an org with the given role.
// If the user is already a member, this is a no-op (idempotent).
func AddOrgMember(db *sql.DB, orgID, userID, role string) error {
	_, err := db.Exec(`
		INSERT INTO org_members (org_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (org_id, user_id) DO NOTHING`,
		orgID, userID, role,
	)
	return err
}

// UpdateOrgMemberRole changes a member's role within an org.
func UpdateOrgMemberRole(db *sql.DB, orgID, userID, newRole string) error {
	_, err := db.Exec(`
		UPDATE org_members
		SET role = $1
		WHERE org_id = $2 AND user_id = $3`,
		newRole, orgID, userID,
	)
	return err
}

// RemoveOrgMember removes a user from an org.
// Owners cannot be removed — transfer ownership first.
func RemoveOrgMember(db *sql.DB, orgID, userID string) error {
	_, err := db.Exec(`
		DELETE FROM org_members
		WHERE org_id = $1 AND user_id = $2 AND role != 'owner'`,
		orgID, userID,
	)
	return err
}

// SlugExists returns true if the slug is already taken.
func SlugExists(db *sql.DB, slug string) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM organizations WHERE slug = $1", slug,
	).Scan(&count)
	return count > 0, err
}
