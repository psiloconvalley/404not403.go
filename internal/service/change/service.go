// Package change implements business logic for change management.
//
// A change request is a planned modification to systems or services.
// Unlike tickets (reactive), changes are proactive and require approval.
//
// Lifecycle:
//   draft → pending_approval → approved → scheduled →
//   in_progress → completed | failed | rolled_back
package change

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/psiloconvalley/404not403/internal/domain"
	orgsvc "github.com/psiloconvalley/404not403/internal/service/org"
	"github.com/psiloconvalley/404not403/internal/store"
)

// Service handles change management business logic.
type Service struct {
	db   *sql.DB
	orgs *orgsvc.Service
}

// New returns a new change Service.
func New(db *sql.DB) *Service {
	return &Service{
		db:   db,
		orgs: orgsvc.New(db),
	}
}

// ── Create ────────────────────────────────────────────────────────────────────

type CreateInput struct {
	OrgID            string
	RequestingUserID string
	Title            string
	Description      string
	ChangeType       string
	RiskLevel        string
	AffectedSystems  []string
	RollbackPlan     string
	TestPlan         *string
	IncidentID       *string
	TicketID         *string
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*store.ChangeRequest, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.RollbackPlan = strings.TrimSpace(input.RollbackPlan)

	if input.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if input.Description == "" {
		return nil, fmt.Errorf("description is required")
	}
	if input.RollbackPlan == "" {
		return nil, fmt.Errorf("rollback plan is required — no rollback plan, no change")
	}
	if input.OrgID == "" || input.RequestingUserID == "" {
		return nil, fmt.Errorf("org_id and requesting_user_id are required")
	}

	// Must be at least agent to create a change
	_, err := s.orgs.RequireRole(ctx, input.OrgID, input.RequestingUserID, "agent")
	if err != nil {
		return nil, err
	}

	if input.ChangeType == "" {
		input.ChangeType = store.ChangeTypeStandard
	}
	if input.RiskLevel == "" {
		input.RiskLevel = "medium"
	}

	return store.CreateChangeRequest(s.db, store.CreateChangeParams{
		OrgID:           input.OrgID,
		Title:           input.Title,
		Description:     input.Description,
		ChangeType:      input.ChangeType,
		RiskLevel:       input.RiskLevel,
		AffectedSystems: input.AffectedSystems,
		RollbackPlan:    input.RollbackPlan,
		TestPlan:        input.TestPlan,
		RequestedBy:     input.RequestingUserID,
		IncidentID:      input.IncidentID,
		TicketID:        input.TicketID,
	})
}

// ── Get ───────────────────────────────────────────────────────────────────────

type ChangeContext struct {
	Change    *store.ChangeRequest  `json:"change"`
	Approvals []store.ChangeApproval `json:"approvals"`
}

func (s *Service) Get(ctx context.Context, orgID, changeID, requestingUserID string) (*ChangeContext, error) {
	_, err := s.orgs.RequireMember(ctx, orgID, requestingUserID)
	if err != nil {
		return nil, err
	}

	cr, err := store.GetChangeByID(s.db, orgID, changeID)
	if err != nil {
		return nil, fmt.Errorf("get change: %w", err)
	}
	if cr == nil {
		return nil, domain.ErrUnauthorized
	}

	approvals, err := store.ListApprovalsForChange(s.db, orgID, changeID)
	if err != nil {
		return nil, fmt.Errorf("load approvals: %w", err)
	}

	return &ChangeContext{
		Change:    cr,
		Approvals: approvals,
	}, nil
}

// ── List ──────────────────────────────────────────────────────────────────────

func (s *Service) List(ctx context.Context, orgID, requestingUserID string) ([]store.ChangeRequest, error) {
	_, err := s.orgs.RequireMember(ctx, orgID, requestingUserID)
	if err != nil {
		return nil, err
	}
	return store.ListChangeRequests(s.db, orgID, 50)
}

func (s *Service) ListPending(ctx context.Context, orgID, requestingUserID string) ([]store.ChangeRequest, error) {
	_, err := s.orgs.RequireMember(ctx, orgID, requestingUserID)
	if err != nil {
		return nil, err
	}
	return store.ListPendingApprovals(s.db, orgID)
}

// ── Submit For Approval ───────────────────────────────────────────────────────

func (s *Service) Submit(ctx context.Context, orgID, changeID, requestingUserID string) error {
	_, err := s.orgs.RequireRole(ctx, orgID, requestingUserID, "agent")
	if err != nil {
		return err
	}

	cr, err := store.GetChangeByID(s.db, orgID, changeID)
	if err != nil {
		return fmt.Errorf("get change: %w", err)
	}
	if cr == nil {
		return domain.ErrUnauthorized
	}
	if cr.RequestedBy != requestingUserID {
		// Only the requester can submit their own change
		// Unless admin/owner
		member, err := store.GetOrgMember(s.db, orgID, requestingUserID)
		if err != nil || member == nil {
			return domain.ErrUnauthorized
		}
		if member.Role != "admin" && member.Role != "owner" {
			return domain.ErrUnauthorized
		}
	}

	return store.SubmitForApproval(s.db, orgID, changeID)
}

// ── Approval ──────────────────────────────────────────────────────────────────

func (s *Service) RequestApproval(ctx context.Context, orgID, changeID, requestingUserID, approverUserID string) error {
	_, err := s.orgs.RequireRole(ctx, orgID, requestingUserID, "agent")
	if err != nil {
		return err
	}

	// Verify approver is a member
	approver, err := store.GetOrgMember(s.db, orgID, approverUserID)
	if err != nil || approver == nil {
		return domain.ErrUnauthorized
	}

	return store.RequestApproval(s.db, orgID, changeID, approverUserID)
}

func (s *Service) Approve(ctx context.Context, orgID, changeID, requestingUserID string, comment *string) error {
	_, err := s.orgs.RequireRole(ctx, orgID, requestingUserID, "agent")
	if err != nil {
		return err
	}
	return store.ApproveChange(s.db, orgID, changeID, requestingUserID, comment)
}

func (s *Service) Reject(ctx context.Context, orgID, changeID, requestingUserID string, comment *string) error {
	_, err := s.orgs.RequireRole(ctx, orgID, requestingUserID, "agent")
	if err != nil {
		return err
	}
	return store.RejectChange(s.db, orgID, changeID, requestingUserID, comment)
}

// ── Execute ───────────────────────────────────────────────────────────────────

func (s *Service) UpdateStatus(ctx context.Context, orgID, changeID, requestingUserID, newStatus string) error {
	validStatuses := map[string]bool{
		store.ChangeStatusInProgress: true,
		store.ChangeStatusCompleted:  true,
		store.ChangeStatusFailed:     true,
		store.ChangeStatusRolledBack: true,
		store.ChangeStatusCancelled:  true,
	}
	if !validStatuses[newStatus] {
		return fmt.Errorf("invalid status: %q", newStatus)
	}

	_, err := s.orgs.RequireRole(ctx, orgID, requestingUserID, "agent")
	if err != nil {
		return err
	}

	return store.UpdateChangeStatus(s.db, orgID, changeID, newStatus)
}
