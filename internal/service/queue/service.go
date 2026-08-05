// Package queue implements business logic for org queues.
//
// A queue is a department-owned container for tickets.
// Queues are how organizations like IT, HR, Facilities, and Legal
// organize, triage, and report on work.
//
// This layer:
//   - validates queue input
//   - enforces org membership before queue actions
//   - coordinates queue assignment and sidebar counts
//
// No SQL. No HTTP parsing.
package queue

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/psiloconvalley/404not403/internal/domain"
	orgsvc "github.com/psiloconvalley/404not403/internal/service/org"
	"github.com/psiloconvalley/404not403/internal/store"
)

// Service handles queue business logic.
type Service struct {
	db      *sql.DB
	orgs    *orgsvc.Service
}

// New returns a new queue Service.
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
	Name             string
	Prefix           *string
	Description      *string
	Department       *string
	Color            string
	Icon             *string
	Visibility       string
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*store.Queue, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return nil, fmt.Errorf("queue name is required")
	}
	if len(input.Name) > 100 {
		return nil, fmt.Errorf("queue name must be 100 characters or less")
	}
	if input.OrgID == "" || input.RequestingUserID == "" {
		return nil, fmt.Errorf("org_id and requesting_user_id are required")
	}

	// Only admins or owners can create queues
	_, err := s.orgs.RequireRole(ctx, input.OrgID, input.RequestingUserID, "admin")
	if err != nil {
		return nil, err
	}

	// Validate and default visibility
	visibility := input.Visibility
	if visibility == "" {
		visibility = "normal"
	}
	if visibility != "normal" && visibility != "restricted" {
		return nil, fmt.Errorf("visibility must be 'normal' or 'restricted'")
	}

	return store.CreateQueue(s.db, store.CreateQueueParams{
		OrgID:       input.OrgID,
		Name:        input.Name,
		Prefix:      input.Prefix,
		Description: input.Description,
		Department:  input.Department,
		Color:       input.Color,
		Icon:        input.Icon,
		Visibility:  visibility,
		CreatedBy:   input.RequestingUserID,
	})
}

// ── List ──────────────────────────────────────────────────────────────────────

// SidebarData is what powers the queue sidebar in the dashboard.
type SidebarData struct {
	Role      string                   `json:"role"`
	OrgCounts *store.OrgTicketCounts   `json:"org_counts"`
	Queues    []store.QueueWithCounts  `json:"queues"`
}

func (s *Service) ListForSidebar(ctx context.Context, orgID, requestingUserID string) (*SidebarData, error) {
	if orgID == "" || requestingUserID == "" {
		return nil, fmt.Errorf("org_id and requesting_user_id are required")
	}

	// Get membership and role
	member, err := s.orgs.RequireMember(ctx, orgID, requestingUserID)
	if err != nil {
		return nil, err
	}

	// Org-wide counts for "All Tickets"
	counts, err := store.GetOrgTicketCounts(s.db, orgID)
	if err != nil {
		return nil, fmt.Errorf("load org counts: %w", err)
	}

	// Role-aware queue listing
	var queues []store.QueueWithCounts
	switch member.Role {
	case "owner", "admin":
		queues, err = store.ListQueuesWithCounts(s.db, orgID)
	default:
		queues, err = store.ListQueuesForUser(s.db, orgID, requestingUserID)
	}
	if err != nil {
		return nil, fmt.Errorf("load queues: %w", err)
	}

	return &SidebarData{
		Role:      member.Role,
		OrgCounts: counts,
		Queues:    queues,
	}, nil
}

// ── Ticket Queue Assignment ───────────────────────────────────────────────────

// AssignTicket moves a ticket into a queue.
// Only agents/admins/owners can move tickets.
func (s *Service) AssignTicket(ctx context.Context, orgID, queueID, ticketID, requestingUserID string) error {
	if orgID == "" || queueID == "" || ticketID == "" || requestingUserID == "" {
		return fmt.Errorf("org_id, queue_id, ticket_id, and requesting_user_id are required")
	}

	// Must be at least agent
	_, err := s.orgs.RequireRole(ctx, orgID, requestingUserID, "agent")
	if err != nil {
		return err
	}

	// Verify queue exists
	q, err := store.GetQueueByID(s.db, orgID, queueID)
	if err != nil {
		return fmt.Errorf("verify queue: %w", err)
	}
	if q == nil {
		return domain.ErrUnauthorized
	}

	return store.AssignTicketToQueue(s.db, orgID, ticketID, queueID)
}

// UnassignTicket removes a ticket from its queue.
func (s *Service) UnassignTicket(ctx context.Context, orgID, ticketID, requestingUserID string) error {
	if orgID == "" || ticketID == "" || requestingUserID == "" {
		return fmt.Errorf("org_id, ticket_id, and requesting_user_id are required")
	}

	_, err := s.orgs.RequireRole(ctx, orgID, requestingUserID, "agent")
	if err != nil {
		return err
	}

	return store.UnassignTicketFromQueue(s.db, orgID, ticketID)
}

// ListTicketsByQueue returns tickets in a queue.
// Must be a member of the org to view queue tickets.
func (s *Service) ListTicketsByQueue(ctx context.Context, orgID, queueID, requestingUserID string) ([]store.Ticket, error) {
	if orgID == "" || queueID == "" || requestingUserID == "" {
		return nil, fmt.Errorf("org_id, queue_id, and requesting_user_id are required")
	}

	_, err := s.orgs.RequireMember(ctx, orgID, requestingUserID)
	if err != nil {
		return nil, err
	}

	return store.ListTicketsByQueue(s.db, orgID, queueID, 100)
}
