// Package incident implements business logic for incident management.
//
// An incident is a declared major event affecting multiple users or services.
// Different from a ticket: a ticket is one person's problem.
// An incident is a systemic event requiring coordinated response.
//
// Lifecycle:
//   investigating → identified → monitoring → resolved → postmortem
package incident

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/psiloconvalley/404not403/internal/domain"
	orgsvc "github.com/psiloconvalley/404not403/internal/service/org"
	"github.com/psiloconvalley/404not403/internal/store"
)

// Service handles incident business logic.
type Service struct {
	db   *sql.DB
	orgs *orgsvc.Service
}

// New returns a new incident Service.
func New(db *sql.DB) *Service {
	return &Service{
		db:   db,
		orgs: orgsvc.New(db),
	}
}

// ── Declare ───────────────────────────────────────────────────────────────────

type DeclareInput struct {
	OrgID            string
	RequestingUserID string
	Title            string
	Description      *string
	Severity         string
	AffectedServices []string
	BusinessImpact   *string
	CommanderID      *string
	LinkedTicketID   *string
}

func (s *Service) Declare(ctx context.Context, input DeclareInput) (*store.Incident, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return nil, fmt.Errorf("incident title is required")
	}
	if input.OrgID == "" || input.RequestingUserID == "" {
		return nil, fmt.Errorf("org_id and requesting_user_id are required")
	}

	// Must be at least agent to declare an incident
	_, err := s.orgs.RequireRole(ctx, input.OrgID, input.RequestingUserID, "agent")
	if err != nil {
		return nil, err
	}

	// Validate severity
	if input.Severity == "" {
		input.Severity = "P1"
	}
	if _, err := domain.ParsePriority(input.Severity); err != nil {
		return nil, fmt.Errorf("invalid severity: %w", err)
	}

	// Default commander to requester
	if input.CommanderID == nil {
		input.CommanderID = &input.RequestingUserID
	}

	inc, err := store.CreateIncident(s.db, store.CreateIncidentParams{
		OrgID:            input.OrgID,
		Title:            input.Title,
		Description:      input.Description,
		Severity:         input.Severity,
		CommanderID:      input.CommanderID,
		AffectedServices: input.AffectedServices,
		BusinessImpact:   input.BusinessImpact,
	})
	if err != nil {
		return nil, fmt.Errorf("declare incident: %w", err)
	}

	// Link ticket if provided
	if input.LinkedTicketID != nil {
		_ = store.LinkTicketToIncident(s.db, input.OrgID, *input.LinkedTicketID, inc.ID)
	}

	return inc, nil
}

// ── Get ───────────────────────────────────────────────────────────────────────

type IncidentContext struct {
	Incident  *store.Incident       `json:"incident"`
	Timeline  []store.TimelineEntry `json:"timeline"`
	Tickets   []store.Ticket        `json:"tickets"`
	Postmortem *store.Postmortem    `json:"postmortem"`
}

func (s *Service) Get(ctx context.Context, orgID, incidentID, requestingUserID string) (*IncidentContext, error) {
	_, err := s.orgs.RequireMember(ctx, orgID, requestingUserID)
	if err != nil {
		return nil, err
	}

	inc, err := store.GetIncidentByID(s.db, orgID, incidentID)
	if err != nil {
		return nil, fmt.Errorf("get incident: %w", err)
	}
	if inc == nil {
		return nil, domain.ErrUnauthorized
	}

	timeline, err := store.ListTimeline(s.db, orgID, incidentID)
	if err != nil {
		return nil, fmt.Errorf("load timeline: %w", err)
	}

	tickets, err := store.GetTicketsForIncident(s.db, orgID, incidentID)
	if err != nil {
		return nil, fmt.Errorf("load tickets: %w", err)
	}

	postmortem, err := store.GetPostmortemByIncident(s.db, orgID, incidentID)
	if err != nil {
		return nil, fmt.Errorf("load postmortem: %w", err)
	}

	return &IncidentContext{
		Incident:   inc,
		Timeline:   timeline,
		Tickets:    tickets,
		Postmortem: postmortem,
	}, nil
}

// ── List ──────────────────────────────────────────────────────────────────────

func (s *Service) ListActive(ctx context.Context, orgID, requestingUserID string) ([]store.Incident, error) {
	_, err := s.orgs.RequireMember(ctx, orgID, requestingUserID)
	if err != nil {
		return nil, err
	}
	return store.ListActiveIncidents(s.db, orgID)
}

func (s *Service) ListAll(ctx context.Context, orgID, requestingUserID string) ([]store.Incident, error) {
	_, err := s.orgs.RequireMember(ctx, orgID, requestingUserID)
	if err != nil {
		return nil, err
	}
	return store.ListAllIncidents(s.db, orgID, 50)
}

// ── Update Status ─────────────────────────────────────────────────────────────

func (s *Service) UpdateStatus(ctx context.Context, orgID, incidentID, requestingUserID, newStatus, note string) error {
	if orgID == "" || incidentID == "" || requestingUserID == "" {
		return fmt.Errorf("org_id, incident_id, and requesting_user_id are required")
	}

	validStatuses := map[string]bool{
		"investigating": true,
		"identified":    true,
		"monitoring":    true,
		"resolved":      true,
		"postmortem":    true,
	}
	if !validStatuses[newStatus] {
		return fmt.Errorf("invalid status: %q", newStatus)
	}

	_, err := s.orgs.RequireRole(ctx, orgID, requestingUserID, "agent")
	if err != nil {
		return err
	}

	return store.UpdateIncidentStatus(s.db, orgID, incidentID, requestingUserID, newStatus, note)
}

// ── Timeline ──────────────────────────────────────────────────────────────────

func (s *Service) AddUpdate(ctx context.Context, orgID, incidentID, requestingUserID, body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Errorf("update body is required")
	}

	_, err := s.orgs.RequireRole(ctx, orgID, requestingUserID, "agent")
	if err != nil {
		return err
	}

	return store.AddTimelineEntry(s.db, orgID, incidentID, requestingUserID, store.TimelineUpdate, body)
}

// ── Link Ticket ───────────────────────────────────────────────────────────────

func (s *Service) LinkTicket(ctx context.Context, orgID, incidentID, ticketID, requestingUserID string) error {
	_, err := s.orgs.RequireRole(ctx, orgID, requestingUserID, "agent")
	if err != nil {
		return err
	}
	return store.LinkTicketToIncident(s.db, orgID, ticketID, incidentID)
}

// ── Postmortem ────────────────────────────────────────────────────────────────

type WritePostmortemInput struct {
	OrgID           string
	IncidentID      string
	RequestingUserID string
	WhatHappened    *string
	RootCause       *string
	Detection       *string
	Response        *string
	Prevention      *string
	TimelineSummary *string
}

func (s *Service) WritePostmortem(ctx context.Context, input WritePostmortemInput) (*store.Postmortem, error) {
	if input.OrgID == "" || input.IncidentID == "" || input.RequestingUserID == "" {
		return nil, fmt.Errorf("org_id, incident_id, and requesting_user_id are required")
	}

	_, err := s.orgs.RequireRole(ctx, input.OrgID, input.RequestingUserID, "agent")
	if err != nil {
		return nil, err
	}

	return store.CreatePostmortem(s.db, store.CreatePostmortemParams{
		IncidentID:      input.IncidentID,
		OrgID:           input.OrgID,
		WhatHappened:    input.WhatHappened,
		RootCause:       input.RootCause,
		Detection:       input.Detection,
		Response:        input.Response,
		Prevention:      input.Prevention,
		TimelineSummary: input.TimelineSummary,
		WrittenBy:       input.RequestingUserID,
	})
}
