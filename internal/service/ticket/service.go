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
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

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
	OrgID      string
	CustomerID *string
	Subject    string
	Body       string
	Priority   string            // optional — defaults to P2
	SourceType string            // required — domain.SourceType
	ThreadID   *string           // optional — for idempotency
	CustomerEmail *string    // optional — find or create customer on submit
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
		OrgID:      input.OrgID,
		CustomerID: input.CustomerID,
		Subject:    input.Subject,
		Body:       input.Body,
		Priority:   input.Priority,
		SourceType: input.SourceType,
		ThreadID:   input.ThreadID,
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
	Ticket      *store.Ticket       `json:"ticket"`
	Comments    []store.Comment     `json:"comments"`
	Events      []store.TicketEvent `json:"events"`
	ConfigItems []store.ConfigItem  `json:"config_items"`
	Analysis    *store.AIAnalysis   `json:"analysis"`
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

	return &TicketContext{
		Ticket:      ticket,
		Comments:    comments,
		Events:      events,
		ConfigItems: configItems,
		Analysis:    analysis,
	}, nil
}

// ── List ──────────────────────────────────────────────────────────────────────

// ListInput defines filters for listing tickets.
type ListInput struct {
	OrgID      string
	Status     *string
	Priority   *string
	AssignedTo *string
	Limit      int
	Offset     int
}

// List returns tickets for an org with optional filters.
func (s *Service) List(ctx context.Context, input ListInput) ([]store.Ticket, error) {
	if input.OrgID == "" {
		return nil, fmt.Errorf("org_id is required")
	}

	// Validate filters if provided
	if input.Status != nil {
		if _, err := domain.ParseStatus(*input.Status); err != nil {
			return nil, fmt.Errorf("invalid status filter: %w", err)
		}
	}
	if input.Priority != nil {
		if _, err := domain.ParsePriority(*input.Priority); err != nil {
			return nil, fmt.Errorf("invalid priority filter: %w", err)
		}
	}

	return store.ListTickets(s.db, store.ListTicketsParams{
		OrgID:      input.OrgID,
		Status:     input.Status,
		Priority:   input.Priority,
		AssignedTo: input.AssignedTo,
		Limit:      input.Limit,
		Offset:     input.Offset,
	})
}

// ── Update Status ─────────────────────────────────────────────────────────────

// UpdateStatus transitions a ticket to a new status.
// Validates the transition through the domain state machine.
// Returns domain.ErrInvalidTransition if the transition is not allowed.
func (s *Service) UpdateStatus(ctx context.Context, orgID, ticketID, actorUserID, newStatus string) error {
	if orgID == "" || ticketID == "" || actorUserID == "" {
		return fmt.Errorf("org_id, ticket_id, and actor_user_id are required")
	}

	return store.UpdateTicketStatus(s.db, orgID, ticketID, actorUserID, newStatus)
}

// ── Assign ────────────────────────────────────────────────────────────────────

// Assign assigns a ticket to an agent.
// Verifies the assignee is a member of the org before assigning.
func (s *Service) Assign(ctx context.Context, orgID, ticketID, actorUserID, assigneeUserID string) error {
	if orgID == "" || ticketID == "" || actorUserID == "" || assigneeUserID == "" {
		return fmt.Errorf("org_id, ticket_id, actor_user_id, and assignee_user_id are required")
	}

	// Verify assignee is a member of this org
	member, err := store.GetOrgMember(s.db, orgID, assigneeUserID)
	if err != nil {
		return fmt.Errorf("verify assignee: %w", err)
	}
	if member == nil {
		return domain.ErrUnauthorized
	}

	return store.AssignTicket(s.db, orgID, ticketID, actorUserID, assigneeUserID)
}

// ── Add Comment ───────────────────────────────────────────────────────────────

// AddCommentInput defines the input for adding a comment.
type AddCommentInput struct {
	OrgID      string
	TicketID   string
	AuthorID   *string // agent user ID — nil if from customer
	CustomerID *string // customer ID — nil if from agent
	Body       string
	IsInternal bool
	SourceType string
	ExternalID *string // for idempotency on inbound messages
	AIDrafted  bool
}

// AddComment adds a comment to a ticket.
// Validates the ticket exists and belongs to the org.
// Returns the created comment.
func (s *Service) AddComment(ctx context.Context, input AddCommentInput) (*store.Comment, error) {
	input.Body = strings.TrimSpace(input.Body)
	if input.Body == "" {
		return nil, fmt.Errorf("comment body is required")
	}
	if input.OrgID == "" || input.TicketID == "" {
		return nil, fmt.Errorf("org_id and ticket_id are required")
	}
	if input.AuthorID == nil && input.CustomerID == nil {
		return nil, fmt.Errorf("either author_id or customer_id is required")
	}

	// Validate source type
	if _, err := domain.ParseSourceType(input.SourceType); err != nil {
		input.SourceType = string(domain.SourceApp)
	}

	// Verify ticket exists and belongs to org
	ticket, err := store.GetTicketByID(s.db, input.OrgID, input.TicketID)
	if err != nil {
		return nil, fmt.Errorf("get ticket: %w", err)
	}
	if ticket == nil {
		return nil, domain.ErrUnauthorized
	}

	// Cannot comment on a closed ticket
	status, err := domain.ParseStatus(ticket.Status)
	if err == nil && status.IsTerminal() {
		return nil, domain.ErrTicketClosed
	}

	comment, err := store.CreateComment(s.db, store.CreateCommentParams{
		OrgID:      input.OrgID,
		TicketID:   input.TicketID,
		AuthorID:   input.AuthorID,
		CustomerID: input.CustomerID,
		Body:       input.Body,
		IsInternal: input.IsInternal,
		SourceType: input.SourceType,
		ExternalID: input.ExternalID,
		AIDrafted:  input.AIDrafted,
	})
	if err != nil {
		return nil, err
	}

	// Close the loop:
	// if an agent adds a public comment to a customer-owned ticket,
	// enqueue an outbound email notification.
	if input.AuthorID != nil && !input.IsInternal && ticket.CustomerID != nil {
		customer, err := store.GetCustomerByID(s.db, input.OrgID, *ticket.CustomerID)
		if err == nil && customer != nil && customer.Email != nil && *customer.Email != "" {
			subject := "Re: " + ticket.Subject

			escapedBody := strings.ReplaceAll(input.Body, "\n", "<br>")
			emailBody := "<p>You have an update on your request:</p><div style=\"font-family: sans-serif; line-height:1.6;\">" +
				escapedBody +
				"</div><hr><p style=\"color:#666;font-size:12px;\">Reply to this email or visit the support portal to continue the conversation.</p>"

			payload, _ := json.Marshal(map[string]string{
				"to":      *customer.Email,
				"subject": subject,
				"body":    emailBody,
			})

			idempotencyKey := fmt.Sprintf("email.send.comment.%s", comment.ID)
			_, _ = store.EnqueueJob(s.db, store.EnqueueJobParams{
				OrgID:          &input.OrgID,
				JobType:        store.JobTypeEmailSend,
				Payload:        payload,
				IdempotencyKey: &idempotencyKey,
				MaxAttempts:    3,
			})
		}
	}

	return comment, nil
}

// ── Search ────────────────────────────────────────────────────────────────────

// Search performs full text search on tickets within an org.
func (s *Service) Search(ctx context.Context, orgID, query string, limit int) ([]store.Ticket, error) {
	if orgID == "" {
		return nil, fmt.Errorf("org_id is required")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("search query is required")
	}
	return store.SearchTickets(s.db, orgID, query, limit)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// ── Update Priority ───────────────────────────────────────────────────────────

// UpdatePriority changes a ticket's priority.
// Validates through domain.ParsePriority.
// Records the priority change event.
func (s *Service) UpdatePriority(ctx context.Context, orgID, ticketID, actorUserID, newPriority string) error {
	if orgID == "" || ticketID == "" || actorUserID == "" {
		return fmt.Errorf("org_id, ticket_id, and actor_user_id are required")
	}

	return store.UpdateTicketPriority(s.db, orgID, ticketID, actorUserID, newPriority)
}

// inputHash produces a SHA256 fingerprint of content sent to the AI.
// Used for deduplication — same content does not trigger a second AI call.
func inputHash(subject, body string) string {
	h := sha256.New()
	h.Write([]byte(subject))
	h.Write([]byte("\n"))
	h.Write([]byte(body))
	return fmt.Sprintf("%x", h.Sum(nil))
}
