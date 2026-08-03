// Package org implements business logic for organization management.
//
// Responsibilities:
//   - Validate org creation input
//   - Enforce slug uniqueness and format rules
//   - Manage membership — invite, role change, removal
//   - Verify membership before any org-scoped operation
//
// Never contains SQL. Never contains HTTP parsing.
// Handlers call this. This calls store.
package org

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/store"
)

// Service handles org business logic.
type Service struct {
	db *sql.DB
}

// New returns a new org Service.
func New(db *sql.DB) *Service {
	return &Service{db: db}
}

// slugPattern enforces URL-safe org slugs.
// Lowercase letters, numbers, hyphens only. No leading/trailing hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{1,48}[a-z0-9]$`)

// validRoles defines the set of roles assignable to org members.
var validRoles = map[string]bool{
	"owner":  true,
	"admin":  true,
	"agent":  true,
	"viewer": true,
}

// ── Create ────────────────────────────────────────────────────────────────────

// CreateInput is the validated input for creating an organization.
type CreateInput struct {
	Name        string
	Slug        string
	OwnerUserID string
}

// Create validates input and creates a new organization.
// The creator is automatically added as owner.
// Slug must be unique across all organizations.
func (s *Service) Create(ctx context.Context, input CreateInput) (*store.Organization, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.TrimSpace(strings.ToLower(input.Slug))

	if input.Name == "" {
		return nil, fmt.Errorf("org name is required")
	}
	if len(input.Name) > 100 {
		return nil, fmt.Errorf("org name must be 100 characters or less")
	}
	if input.Slug == "" {
		return nil, fmt.Errorf("slug is required")
	}
	if !slugPattern.MatchString(input.Slug) {
		return nil, fmt.Errorf("slug must be 3-50 characters, lowercase letters, numbers, and hyphens only")
	}
	if input.OwnerUserID == "" {
		return nil, fmt.Errorf("owner_user_id is required")
	}

	// Check slug availability
	exists, err := store.SlugExists(s.db, input.Slug)
	if err != nil {
		return nil, fmt.Errorf("check slug: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("slug %q is already taken", input.Slug)
	}

	org, err := store.CreateOrg(s.db, input.Name, input.Slug, input.OwnerUserID)
	if err != nil {
		return nil, fmt.Errorf("create org: %w", err)
	}

	return org, nil
}

// ── Get ───────────────────────────────────────────────────────────────────────

// OrgContext is an org with its members loaded.
type OrgContext struct {
	Org     *store.Organization
	Members []store.OrgMemberDetail
}

// Get loads an org and its members.
// Returns ErrUnauthorized if the requesting user is not a member.
func (s *Service) Get(ctx context.Context, orgID, requestingUserID string) (*OrgContext, error) {
	// Verify membership first
	member, err := store.GetOrgMember(s.db, orgID, requestingUserID)
	if err != nil {
		return nil, fmt.Errorf("verify membership: %w", err)
	}
	if member == nil {
		return nil, domain.ErrUnauthorized
	}

	org, err := store.GetOrgByID(s.db, orgID)
	if err != nil {
		return nil, fmt.Errorf("get org: %w", err)
	}
	if org == nil {
		return nil, domain.ErrUnauthorized
	}

	members, err := store.ListOrgMembers(s.db, orgID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}

	return &OrgContext{
		Org:     org,
		Members: members,
	}, nil
}

// GetForUser returns all orgs a user belongs to.
func (s *Service) GetForUser(ctx context.Context, userID string) ([]store.OrgMemberDetail, error) {
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	return store.GetOrgsForUser(s.db, userID)
}

// ── Membership ────────────────────────────────────────────────────────────────

// InviteInput defines the input for adding a member to an org.
type InviteInput struct {
	OrgID           string
	RequestingUserID string // must be owner or admin
	TargetUserID    string
	Role            string
}

// Invite adds a user to an org with the given role.
// Only owners and admins can invite members.
// Cannot assign the owner role via invite — ownership is transferred separately.
func (s *Service) Invite(ctx context.Context, input InviteInput) error {
	if input.OrgID == "" || input.RequestingUserID == "" || input.TargetUserID == "" {
		return fmt.Errorf("org_id, requesting_user_id, and target_user_id are required")
	}

	// Validate role
	if !validRoles[input.Role] {
		return fmt.Errorf("invalid role: %q — must be admin, agent, or viewer", input.Role)
	}
	if input.Role == "owner" {
		return fmt.Errorf("cannot assign owner role via invite — transfer ownership separately")
	}

	// Verify requester has permission
	requester, err := store.GetOrgMember(s.db, input.OrgID, input.RequestingUserID)
	if err != nil {
		return fmt.Errorf("verify requester: %w", err)
	}
	if requester == nil {
		return domain.ErrUnauthorized
	}
	if requester.Role != "owner" && requester.Role != "admin" {
		return domain.ErrUnauthorized
	}

	return store.AddOrgMember(s.db, input.OrgID, input.TargetUserID, input.Role)
}

// UpdateRoleInput defines the input for changing a member's role.
type UpdateRoleInput struct {
	OrgID            string
	RequestingUserID string // must be owner
	TargetUserID     string
	NewRole          string
}

// UpdateRole changes a member's role within an org.
// Only owners can change roles.
// Cannot change the owner's own role.
func (s *Service) UpdateRole(ctx context.Context, input UpdateRoleInput) error {
	if input.OrgID == "" || input.RequestingUserID == "" || input.TargetUserID == "" {
		return fmt.Errorf("org_id, requesting_user_id, and target_user_id are required")
	}

	if !validRoles[input.NewRole] {
		return fmt.Errorf("invalid role: %q", input.NewRole)
	}

	// Only owners can change roles
	requester, err := store.GetOrgMember(s.db, input.OrgID, input.RequestingUserID)
	if err != nil {
		return fmt.Errorf("verify requester: %w", err)
	}
	if requester == nil || requester.Role != "owner" {
		return domain.ErrUnauthorized
	}

	// Cannot demote yourself as owner
	if input.TargetUserID == input.RequestingUserID {
		return fmt.Errorf("cannot change your own role")
	}

	return store.UpdateOrgMemberRole(s.db, input.OrgID, input.TargetUserID, input.NewRole)
}

// RemoveMember removes a user from an org.
// Only owners and admins can remove members.
// Owners cannot be removed — transfer ownership first.
func (s *Service) RemoveMember(ctx context.Context, orgID, requestingUserID, targetUserID string) error {
	if orgID == "" || requestingUserID == "" || targetUserID == "" {
		return fmt.Errorf("org_id, requesting_user_id, and target_user_id are required")
	}

	// Verify requester has permission
	requester, err := store.GetOrgMember(s.db, orgID, requestingUserID)
	if err != nil {
		return fmt.Errorf("verify requester: %w", err)
	}
	if requester == nil {
		return domain.ErrUnauthorized
	}
	if requester.Role != "owner" && requester.Role != "admin" {
		return domain.ErrUnauthorized
	}

	// Cannot remove yourself
	if targetUserID == requestingUserID {
		return fmt.Errorf("cannot remove yourself from the org")
	}

	return store.RemoveOrgMember(s.db, orgID, targetUserID)
}

// ── Membership Verification ───────────────────────────────────────────────────

// RequireMember verifies a user is a member of an org.
// Returns ErrUnauthorized if not.
// Used by handlers before any org-scoped operation.
func (s *Service) RequireMember(ctx context.Context, orgID, userID string) (*store.OrgMember, error) {
	member, err := store.GetOrgMember(s.db, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("verify membership: %w", err)
	}
	if member == nil {
		return nil, domain.ErrUnauthorized
	}
	return member, nil
}

// RequireRole verifies a user has at least the given role in an org.
// Role hierarchy: owner > admin > agent > viewer
func (s *Service) RequireRole(ctx context.Context, orgID, userID, minimumRole string) (*store.OrgMember, error) {
	member, err := s.RequireMember(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}

	if !roleAtLeast(member.Role, minimumRole) {
		return nil, domain.ErrUnauthorized
	}

	return member, nil
}

// roleAtLeast returns true if actual role meets or exceeds the required role.
// roleAtLeast returns true if actual role meets or exceeds the required role.
// Unknown roles always return false — fail closed.
func roleAtLeast(actual, required string) bool {
	order := map[string]int{
		"viewer": 1,
		"agent":  2,
		"admin":  3,
		"owner":  4,
	}
	a, aOK := order[actual]
	r, rOK := order[required]
	if !aOK || !rOK {
		return false
	}
	return a >= r
}

// ── Update Org ───────────────────────────────────────────────────────────────

// UpdateOrgInput defines the fields that can be updated on an organization.
type UpdateOrgInput struct {
	OrgID            string
	RequestingUserID string
	Name             *string
	Domain           *string
	InboundEmail     *string
}

// UpdateOrg updates an organization's settings.
// Only owners and admins can update org settings.
func (s *Service) UpdateOrg(ctx context.Context, input UpdateOrgInput) (*store.Organization, error) {
	if input.OrgID == "" || input.RequestingUserID == "" {
		return nil, fmt.Errorf("org_id and requesting_user_id are required")
	}

	// Only admins or owners can update org settings
	_, err := s.RequireRole(ctx, input.OrgID, input.RequestingUserID, "admin")
	if err != nil {
		return nil, err
	}

	return store.UpdateOrg(s.db, input.OrgID, input.Name, input.Domain, input.InboundEmail)
}
