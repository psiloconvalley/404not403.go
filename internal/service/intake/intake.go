package intake

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/provider/chat"
	"github.com/psiloconvalley/404not403/internal/store"
	"github.com/psiloconvalley/404not403/internal/service/ticket"
)

type Service struct {
	db        *sql.DB
	ticketSvc *ticket.Service
	provider  chat.ChatProvider
}

func New(db *sql.DB, ticketSvc *ticket.Service, provider chat.ChatProvider) *Service {
	return &Service{
		db:        db,
		ticketSvc: ticketSvc,
		provider:  provider,
	}
}

// HandleInboundMessage processes a normalized chat message, routing it to ticket or comment.
func (s *Service) HandleInboundMessage(ctx context.Context, msg *chat.ChatMessage) error {
	// 1. Resolve workspace to locate owning tenant and bot credentials
	workspace, err := store.GetChatWorkspaceByWorkspaceID(s.db, msg.Provider, msg.WorkspaceID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Printf("⚠️ Chat workspace %s/%s not registered with any tenant org", msg.Provider, msg.WorkspaceID)
			return nil // Ignore messages from unregistered workspaces
		}
		return fmt.Errorf("resolve chat workspace: %w", err)
	}

	// 2. Identify or create customer directory profile
	customer, err := s.resolveCustomer(ctx, workspace.OrgID, workspace.BotToken, msg.ProviderUserID)
	if err != nil {
		return fmt.Errorf("resolve customer identity: %w", err)
	}

	// 3. Query thread context to check for idempotency
	ticketID, _, err := store.GetTicketIDByChannelThread(s.db, msg.Provider, msg.WorkspaceID, msg.ChannelID, msg.ThreadID)
	if err == nil && ticketID != "" {
		// Ticket already exists for this thread -> append comment
		return s.appendComment(ctx, workspace.OrgID, ticketID, customer.ID, msg.Body)
	}

	if !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("lookup thread context: %w", err)
	}

	// 4. Thread is new -> create a new ticket vessel
	return s.createTicketFromThread(ctx, workspace.OrgID, customer.ID, msg)
}

func (s *Service) resolveCustomer(ctx context.Context, orgID, botToken, providerUserID string) (*store.Customer, error) {
	// 1. Check directory by direct slack ID
	c, err := store.GetCustomerBySlackUserID(s.db, orgID, providerUserID)
	if err == nil && c != nil {
		return c, nil
	}

	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	// 2. Resolve user email from Slack API
	chatUser, err := s.provider.ResolveUser(ctx, botToken, providerUserID)
	if err != nil {
		log.Printf("⚠️ Failed to resolve chat user %s: %v", providerUserID, err)
		// Fallback: create temporary customer using slack ID
		handle := "slack-" + providerUserID
		email := handle + "@unverified.404not403.com"
		fullName := handle
		return store.CreateCustomer(s.db, orgID, &email, &providerUserID, &fullName, nil)
	}

	// 3. Check directory by resolved email
	if chatUser.Email != nil {
		c, err = store.GetCustomerByEmail(s.db, orgID, *chatUser.Email)
		if err == nil && c != nil {
			// Update customer with their Slack ID so lookup is fast next time
			_, _ = s.db.Exec("UPDATE customers SET slack_user_id = $1 WHERE id = $2 AND org_id = $3", providerUserID, c.ID, orgID)
			return c, nil
		}
	}

	// 4. Create new customer record
	email := "slack-" + providerUserID + "@unverified.404not403.com"
	if chatUser.Email != nil {
		email = *chatUser.Email
	}

	return store.CreateCustomer(s.db, orgID, &email, &providerUserID, &chatUser.DisplayName, nil)
}

func (s *Service) appendComment(ctx context.Context, orgID, ticketID, customerID, body string) error {
	_, err := store.CreateComment(s.db, store.CreateCommentParams{
		OrgID:      orgID,
		TicketID:   ticketID,
		CustomerID: &customerID,
		Body:       body,
		IsInternal: false,
		SourceType: "slack",
	})
	return err
}

func (s *Service) createTicketFromThread(ctx context.Context, orgID, customerID string, msg *chat.ChatMessage) error {
	// Generate subject placeholder (first 60 characters of message body)
	bodyRunes := []rune(msg.Body)
	subject := string(bodyRunes)
	if len(bodyRunes) > 60 {
		subject = string(bodyRunes[:57]) + "..."
	}
	if subject == "" {
		subject = "Slack message in channel " + msg.ChannelID
	}

	// Wrap in a transaction to guarantee atomic ticket + context creation
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var t store.Ticket
	seqNum := 1
	_ = tx.QueryRow("SELECT COALESCE(MAX(sequence_num), 0) + 1 FROM tickets WHERE org_id = $1", orgID).Scan(&seqNum)
	displayID := fmt.Sprintf("REQ-%d", seqNum)

	err = tx.QueryRow(`
		INSERT INTO tickets (
			org_id, customer_id, subject, body, status, priority, source_type, thread_id, sequence_num, display_id, requester_customer_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, org_id, display_id, sequence_num, subject, body, status, priority, source_type, created_at, updated_at`,
		orgID, customerID, subject, msg.Body, "open", "P2", "slack", msg.ThreadID, seqNum, displayID, customerID,
	).Scan(
		&t.ID, &t.OrgID, &t.DisplayID, &t.SequenceNum, &t.Subject, &t.Body, &t.Status, &t.Priority, &t.SourceType, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return err
	}

	// Record event in same transaction
	payloadBytes, _ := json.Marshal(map[string]string{"source": "slack", "priority": "P2"})
	err = store.RecordEventTx(tx, orgID, t.ID, nil, string(domain.ActorSystem), string(domain.EventTicketCreated), payloadBytes)
	if err != nil {
		return err
	}

	// Create channel context link in same transaction
	err = store.CreateTicketChannelContextTx(tx, store.TicketChannelContext{
		TicketID:       t.ID,
		OrgID:          orgID,
		Provider:       msg.Provider,
		WorkspaceID:    msg.WorkspaceID,
		ChannelID:      msg.ChannelID,
		ThreadID:       msg.ThreadID,
		ProviderUserID: msg.ProviderUserID,
		RawContext:     msg.RawPayload,
	})
	if err != nil {
		return err
	}

	return tx.Commit()
}
