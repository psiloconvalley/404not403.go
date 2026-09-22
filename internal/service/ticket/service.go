// Package ticket implements the business logic for ticket lifecycle management.
//
// Responsibilities:
//   - Validate input before it reaches the store
//   - Enforce business rules (tier limits, ownership, consent)
//   - Coordinate multi-step operations (create ticket + enqueue AI job)
//   - Never contain SQL
//   - Never contain HTTP parsing
//
// The service receives plain Go types.
// The service returns plain Go types or domain errors.
// Handlers call the service. The service calls the store.
package ticket

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/store"
)

// Service handles ticket business logic.
type Service struct {
	db *sql.DB
}

// New returns a new ticket Service.
func New(db *sql.DB) *Service {
	return &Service{db: db}
}

// ── Create ────────────────────────────────────────────────────────────────────
// CreateInput is the validated input for creating a ticket.
type CreateInput struct {
	OrgID                 string
	QueueID               *string
	CustomerID            *string
	Subject               string
	Body                  string
	Priority              string  // optional — defaults to P2
	SourceType            string  // required — domain.SourceType
	ThreadID              *string // optional — for idempotency
	CustomerEmail         *string // optional — find or create customer on submit
	CatalogItemID         *string // optional — drives TicketType, Priority, SLADueAt
	SubmittedByCustomerID *string // optional — identity of submitter
	SubmittedByUserID     *string // optional — internal agent creator
	RequesterCustomerID   *string // optional — beneficiary customer
}

// CreateResult is returned after a ticket is created.
type CreateResult struct {
	Ticket *store.Ticket
	JobID  *string // nil if job enqueue was skipped
}

// Create validates input, creates a ticket, and enqueues an AI classification job.
// The AI job is enqueued in the same transaction as the ticket creation.
// If AI enqueue fails, the ticket is still created — AI is never blocking.
func (s *Service) Create(ctx context.Context, input CreateInput) (*CreateResult, error) {
	// Validate subject and body
	input.Subject = strings.TrimSpace(input.Subject)
	input.Body = strings.TrimSpace(input.Body)

	if input.Subject == "" {
		return nil, fmt.Errorf("subject is required")
	}
	if input.Body == "" {
		return nil, fmt.Errorf("body is required")
	}
	if len(input.Subject) > 500 {
		return nil, fmt.Errorf("subject must be 500 characters or less")
	}
	if input.OrgID == "" {
		return nil, fmt.Errorf("org_id is required")
	}

	// Validate and default source type
	if _, err := domain.ParseSourceType(input.SourceType); err != nil {
		return nil, fmt.Errorf("invalid source_type: %w", err)
	}

	// Validate and default priority
	if input.Priority == "" {
		input.Priority = string(domain.DefaultPriority())
	}
	if _, err := domain.ParsePriority(input.Priority); err != nil {
		return nil, fmt.Errorf("invalid priority: %w", err)
	}

	// Resolve catalog item — drives TicketType, Priority, SLADueAt
	// If no catalog item, defaults apply: type=service_request, priority already set above
	ticketType := "service_request"
	var sladue *time.Time
	if input.CatalogItemID != nil && *input.CatalogItemID != "" {
		catalogItem, err := store.GetCatalogItem(s.db, input.OrgID, *input.CatalogItemID)
		if err != nil {
			return nil, fmt.Errorf("resolve catalog item: %w", err)
		}
		if catalogItem != nil {
			ticketType = catalogItem.TicketType
			// Catalog default priority wins only if caller did not specify one
			if input.Priority == string(domain.DefaultPriority()) && catalogItem.DefaultPriority != "" {
				input.Priority = catalogItem.DefaultPriority
			}
			// Calculate SLA deadline
			if catalogItem.SLAHours != nil && *catalogItem.SLAHours > 0 {
				due := time.Now().UTC().Add(time.Duration(*catalogItem.SLAHours) * time.Hour)
				sladue = &due
			}
		}
	}

	// Resolve customer email to customer ID if provided
	if input.CustomerEmail != nil && *input.CustomerEmail != "" && input.CustomerID == nil {
		customer, err := store.FindOrCreateCustomerByEmail(s.db, input.OrgID, *input.CustomerEmail, nil)
		if err != nil {
			return nil, fmt.Errorf("resolve customer: %w", err)
		}
		input.CustomerID = &customer.ID
	}

	// Create the ticket
	ticket, err := store.CreateTicket(s.db, store.CreateTicketParams{
		OrgID:                 input.OrgID,
		QueueID:               input.QueueID,
		CustomerID:            input.CustomerID,
		Subject:               input.Subject,
		Body:                  input.Body,
		Priority:              input.Priority,
		SourceType:            input.SourceType,
		ThreadID:              input.ThreadID,
		TicketType:            ticketType,
		SLADueAt:              sladue,
		SubmittedByCustomerID: input.SubmittedByCustomerID,
		SubmittedByUserID:     input.SubmittedByUserID,
		RequesterCustomerID:   input.RequesterCustomerID,
	})
	if err != nil {
		return nil, fmt.Errorf("create ticket: %w", err)
	}

	// Enqueue AI classification job
	// Non-fatal — ticket exists regardless of whether this succeeds
	idempotencyKey := fmt.Sprintf("ai.classify.%s", ticket.ID)
	payload, _ := json.Marshal(map[string]string{
		"ticket_id": ticket.ID,
		"org_id":    ticket.OrgID,
	})

	job, err := store.EnqueueJob(s.db, store.EnqueueJobParams{
		OrgID:          &ticket.OrgID,
		JobType:        store.JobTypeAIClassify,
		Payload:        payload,
		IdempotencyKey: &idempotencyKey,
		MaxAttempts:    3,
	})
	if err != nil {
		// Log but do not fail — ticket is already created
		fmt.Printf("⚠️  failed to enqueue AI job for ticket %s: %v\n", ticket.ID, err)
	}

	result := &CreateResult{Ticket: ticket}
	if job != nil {
		result.JobID = &job.ID
	}

	return result, nil
}

// ── Get ───────────────────────────────────────────────────────────────────────

// TicketContext is a ticket with all related data loaded.
// This is what an agent sees when they open a ticket.
type TicketContext struct {
	Ticket       *store.Ticket       `json:"ticket"`
	Requester    *store.Customer     `json:"requester,omitempty"`
	Submitter    *store.Customer     `json:"submitter,omitempty"`
	ManagerChain []store.Customer    `json:"manager_chain,omitempty"`
	Comments     []store.Comment     `json:"comments"`
	Events       []store.TicketEvent `json:"events"`
	ConfigItems  []store.ConfigItem  `json:"config_items"`
	Analysis     *store.AIAnalysis   `json:"analysis"`
}

// Get loads a ticket with full context.
// Scoped to org — agents cannot access tickets from other orgs.
func (s *Service) Get(ctx context.Context, orgID, ticketID string) (*TicketContext, error) {
	ticket, err := store.GetTicketByID(s.db, orgID, ticketID)
	if err != nil {
		return nil, fmt.Errorf("get ticket: %w", err)
	}
	if ticket == nil {
		return nil, domain.ErrUnauthorized // do not reveal existence to wrong org
	}

	// Load all related data — failures are non-fatal for read operations
	comments, err := store.ListCommentsByTicket(s.db, ticketID, true)
	if err != nil {
		return nil, fmt.Errorf("load comments: %w", err)
	}

	events, err := store.ListEventsByTicket(s.db, orgID, ticketID)
	if err != nil {
		return nil, fmt.Errorf("load events: %w", err)
	}

	configItems, err := store.GetConfigItemsForTicket(s.db, orgID, ticketID)
	if err != nil {
		return nil, fmt.Errorf("load config items: %w", err)
	}

	analysis, err := store.GetLatestAnalysis(s.db, orgID, ticketID)
	if err != nil {
		return nil, fmt.Errorf("load analysis: %w", err)
	}

	var requester *store.Customer
	var managerChain []store.Customer
	reqID := ticket.RequesterCustomerID
	if reqID == nil || *reqID == "" {
		reqID = ticket.CustomerID
	}
	if reqID != nil && *reqID != "" {
		if r, err := store.GetCustomerByID(s.db, orgID, *reqID); err == nil && r != nil {
			requester = r
			if chain, err := store.GetManagerChain(s.db, orgID, r.ID, 5); err == nil {
				managerChain = chain
			}
		}
	}

	var submitter *store.Customer
	if ticket.SubmittedByCustomerID != nil && *ticket.SubmittedByCustomerID != "" {
		if s, err := store.GetCustomerByID(s.db, orgID, *ticket.SubmittedByCustomerID); err == nil && s != nil {
			submitter = s
		}
	}

	return &TicketContext{
		Ticket:       ticket,
		Requester:    requester,
		Submitter:    submitter,
		ManagerChain: managerChain,
		Comments:     comments,
		Events:       events,
		ConfigItems:  configItems,
		Analysis:     analysis,
	}, nil
}

// ── List ──────────────────────────────────────────────────────────────────────
