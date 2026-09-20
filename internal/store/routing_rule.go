package store

import (
	"database/sql"
	"strings"
	"time"
)

// ── RoutingRule ───────────────────────────────────────────────────────────────

// RoutingRule is a deterministic filter that matches inbound tickets to queues.
// Checked before invoking AI models to save on cost and speed up response times.
type RoutingRule struct {
	ID            string     `json:"id"`
	OrgID         string     `json:"org_id"`
	QueueID       string     `json:"queue_id"`
	Name          string     `json:"name"`
	Priority      int        `json:"priority"`
	MatchType     string     `json:"match_type"`      // 'keyword', 'regex', 'domain'
	MatchField    string     `json:"match_field"`     // 'subject', 'body', 'subject_or_body', 'sender_domain'
	MatchValue    string     `json:"match_value"`     // e.g., "okta" or "loaner"
	CaseSensitive bool       `json:"case_sensitive"`
	Active        bool       `json:"active"`
	HitCount      int        `json:"hit_count"`
	LastMatchedAt *time.Time `json:"last_matched_at,omitempty"`
	CreatedBy     *string    `json:"created_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// CreateRoutingRuleParams contains options for creating a routing rule.
type CreateRoutingRuleParams struct {
	OrgID         string
	QueueID       string
	Name          string
	Priority      int
	MatchType     string // 'keyword', 'regex', 'domain'
	MatchField    string // 'subject', 'body', 'subject_or_body', 'sender_domain'
	MatchValue    string
	CaseSensitive bool
	CreatedBy     *string
}

// CreateRoutingRule inserts a new routing rule.
func CreateRoutingRule(db *sql.DB, p CreateRoutingRuleParams) (*RoutingRule, error) {
	if p.Priority == 0 {
		p.Priority = 100
	}
	var r RoutingRule
	err := db.QueryRow(`
		INSERT INTO routing_rules (org_id, queue_id, name, priority, match_type, match_field, match_value, case_sensitive, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, org_id, queue_id, name, priority, match_type, match_field, match_value, case_sensitive, active, hit_count, last_matched_at, created_by, created_at, updated_at`,
		p.OrgID, p.QueueID, p.Name, p.Priority, p.MatchType, p.MatchField, p.MatchValue, p.CaseSensitive, p.CreatedBy,
	).Scan(
		&r.ID, &r.OrgID, &r.QueueID, &r.Name, &r.Priority, &r.MatchType, &r.MatchField, &r.MatchValue, &r.CaseSensitive, &r.Active, &r.HitCount, &r.LastMatchedAt, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetRoutingRule retrieves a single routing rule by ID.
func GetRoutingRule(db *sql.DB, id string) (*RoutingRule, error) {
	var r RoutingRule
	err := db.QueryRow(`
		SELECT id, org_id, queue_id, name, priority, match_type, match_field, match_value, case_sensitive, active, hit_count, last_matched_at, created_by, created_at, updated_at
		FROM routing_rules
		WHERE id = $1`,
		id,
	).Scan(
		&r.ID, &r.OrgID, &r.QueueID, &r.Name, &r.Priority, &r.MatchType, &r.MatchField, &r.MatchValue, &r.CaseSensitive, &r.Active, &r.HitCount, &r.LastMatchedAt, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRoutingRules retrieves all routing rules for an organization.
func ListRoutingRules(db *sql.DB, orgID string) ([]*RoutingRule, error) {
	rows, err := db.Query(`
		SELECT id, org_id, queue_id, name, priority, match_type, match_field, match_value, case_sensitive, active, hit_count, last_matched_at, created_by, created_at, updated_at
		FROM routing_rules
		WHERE org_id = $1
		ORDER BY priority ASC, name ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*RoutingRule
	for rows.Next() {
		var r RoutingRule
		err := rows.Scan(
			&r.ID, &r.OrgID, &r.QueueID, &r.Name, &r.Priority, &r.MatchType, &r.MatchField, &r.MatchValue, &r.CaseSensitive, &r.Active, &r.HitCount, &r.LastMatchedAt, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		rules = append(rules, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

// UpdateRoutingRuleParams holds options for updating a routing rule.
type UpdateRoutingRuleParams struct {
	ID            string
	QueueID       string
	Name          string
	Priority      int
	MatchType     string
	MatchField    string
	MatchValue    string
	CaseSensitive bool
	Active        bool
}

// UpdateRoutingRule updates a routing rule's details.
func UpdateRoutingRule(db *sql.DB, p UpdateRoutingRuleParams) (*RoutingRule, error) {
	var r RoutingRule
	err := db.QueryRow(`
		UPDATE routing_rules
		SET queue_id = $1, name = $2, priority = $3, match_type = $4, match_field = $5, match_value = $6, case_sensitive = $7, active = $8, updated_at = now()
		WHERE id = $9
		RETURNING id, org_id, queue_id, name, priority, match_type, match_field, match_value, case_sensitive, active, hit_count, last_matched_at, created_by, created_at, updated_at`,
		p.QueueID, p.Name, p.Priority, p.MatchType, p.MatchField, p.MatchValue, p.CaseSensitive, p.Active, p.ID,
	).Scan(
		&r.ID, &r.OrgID, &r.QueueID, &r.Name, &r.Priority, &r.MatchType, &r.MatchField, &r.MatchValue, &r.CaseSensitive, &r.Active, &r.HitCount, &r.LastMatchedAt, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// IncrementRuleHitCount increments a rule's matches metric and updates last_matched_at.
func IncrementRuleHitCount(db *sql.DB, id string) error {
	_, err := db.Exec(`
		UPDATE routing_rules
		SET hit_count = hit_count + 1, last_matched_at = now()
		WHERE id = $1`,
		id,
	)
	return err
}

// ── Deterministic Matching Engine ─────────────────────────────────────────────

// MatchTicketToQueue runs a ticket's subject and body against all active routing rules
// for an organization. Returns the matching queue_id and routing_rule_id if a match is found.
// If no rules match, returns ("", "", nil).
func MatchTicketToQueue(db *sql.DB, orgID, subject, body string) (string, string, error) {
	rows, err := db.Query(`
		SELECT id, queue_id, match_type, match_field, match_value, case_sensitive
		FROM routing_rules
		WHERE org_id = $1 AND active = true
		ORDER BY priority ASC, id ASC`,
		orgID,
	)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	for rows.Next() {
		var ruleID, queueID, mType, mField, mValue string
		var caseSensitive bool

		err := rows.Scan(&ruleID, &queueID, &mType, &mField, &mValue, &caseSensitive)
		if err != nil {
			return "", "", err
		}

		// v1 currently only evaluates 'keyword' type. Match rules and ignore others.
		if mType != "keyword" {
			continue
		}

		// Select target fields for string checking
		var searchContent []string
		switch mField {
		case "subject":
			searchContent = []string{subject}
		case "body":
			searchContent = []string{body}
		case "subject_or_body":
			searchContent = []string{subject, body}
		default:
			continue
		}

		// Perform keyword check
		matched := false
		for _, content := range searchContent {
			if caseSensitive {
				if strings.Contains(content, mValue) {
					matched = true
					break
				}
			} else {
				if strings.Contains(strings.ToLower(content), strings.ToLower(mValue)) {
					matched = true
					break
				}
			}
		}

		if matched {
			return queueID, ruleID, nil
		}
	}

	if err := rows.Err(); err != nil {
		return "", "", err
	}

	return "", "", nil
}
