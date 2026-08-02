// Package inbound handles incoming webhooks from email providers.
//
// Responsibilities:
//   - Receive raw webhook payloads
//   - Store immediately (never lose data)
//   - Parse and create tickets
//   - Handle duplicates gracefully via external_id uniqueness
package inbound

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/domain"
	ticketsvc "github.com/psiloconvalley/404not403/internal/service/ticket"
	"github.com/psiloconvalley/404not403/internal/store"
)

// Handler holds inbound webhook dependencies.
type Handler struct {
	app *app.App
	svc *ticketsvc.Service
}

// New returns a new inbound Handler.
func New(a *app.App) *Handler {
	return &Handler{
		app: a,
		svc: ticketsvc.New(a.DB),
	}
}

// ResendEmail handles POST /api/webhooks/inbound-email
// Receives inbound emails from Resend and creates tickets.
func (h *Handler) ResendEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}

	// Read raw body — we store this before any parsing
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("inbound: failed to read body: %v", err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Parse enough to get the Message-ID for idempotency
	var payload struct {
		Type string `json:"type"`
		Data struct {
			From    string   `json:"from"`
			To      []string `json:"to"`
			Subject string   `json:"subject"`
			Text    string   `json:"text"`
			HTML    string   `json:"html"`
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("inbound: failed to parse JSON: %v", err)
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Extract Message-ID for idempotency
	var messageID string
	for _, h := range payload.Data.Headers {
		if strings.EqualFold(h.Name, "Message-ID") {
			messageID = h.Value
			break
		}
	}
	if messageID == "" {
		log.Printf("inbound: no Message-ID header")
		http.Error(w, "missing Message-ID header", http.StatusBadRequest)
		return
	}

	// Store raw message first — never lose the original
	msg, err := store.CreateInboundMessage(h.app.DB, "email", messageID, body)
	if err != nil {
		// Duplicate Message-ID — already processed, return success
		if strings.Contains(err.Error(), "duplicate key") {
			log.Printf("inbound: duplicate message %s — ignoring", messageID)
			w.WriteHeader(http.StatusOK)
			return
		}
		log.Printf("inbound: failed to store message: %v", err)
		http.Error(w, "failed to store message", http.StatusInternalServerError)
		return
	}

	// Find the organization by the To address
	if len(payload.Data.To) == 0 {
		log.Printf("inbound: no To address in message %s", msg.ID)
		w.WriteHeader(http.StatusOK) // Accept but don't process
		return
	}

	toAddress := extractEmail(payload.Data.To[0])
	org, err := store.GetOrgByInboundEmail(h.app.DB, toAddress)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("inbound: no org for address %s — message %s stored but not processed", toAddress, msg.ID)
			w.WriteHeader(http.StatusOK) // Accept — maybe configured later
			return
		}
		log.Printf("inbound: failed to lookup org: %v", err)
		http.Error(w, "org lookup failed", http.StatusInternalServerError)
		return
	}

	// Find or create the customer
	fromAddress := extractEmail(payload.Data.From)
	fromName := extractName(payload.Data.From)
	customer, err := store.FindOrCreateCustomerByEmail(h.app.DB, org.ID, fromAddress, fromName)
	if err != nil {
		log.Printf("inbound: failed to find/create customer: %v", err)
		http.Error(w, "customer lookup failed", http.StatusInternalServerError)
		return
	}

	// Create the ticket
	ticketBody := payload.Data.Text
	if ticketBody == "" {
		ticketBody = payload.Data.HTML // Fallback to HTML if no plain text
	}
	if ticketBody == "" {
		ticketBody = "(empty message body)"
	}

	subject := payload.Data.Subject
	if subject == "" {
		subject = "(no subject)"
	}

	result, err := h.svc.Create(r.Context(), ticketsvc.CreateInput{
		OrgID:      org.ID,
		CustomerID: &customer.ID,
		Subject:    subject,
		Body:       ticketBody,
		SourceType: string(domain.SourceEmail),
		ThreadID:   &messageID,
	})
	if err != nil {
		log.Printf("inbound: failed to create ticket: %v", err)
		http.Error(w, "failed to create ticket", http.StatusInternalServerError)
		return
	}

	// Mark the inbound message as processed
	if err := store.MarkInboundProcessed(h.app.DB, msg.ID, org.ID, result.Ticket.ID); err != nil {
		log.Printf("inbound: failed to mark processed: %v", err)
		// Don't fail — ticket was created
	}

	log.Printf("inbound: created ticket %s from email %s", result.Ticket.ID, messageID)
	w.WriteHeader(http.StatusOK)
}

// extractEmail pulls the email address from "Name <email@example.com>" format
func extractEmail(s string) string {
	s = strings.TrimSpace(s)
	if start := strings.Index(s, "<"); start != -1 {
		if end := strings.Index(s, ">"); end > start {
			return strings.TrimSpace(s[start+1 : end])
		}
	}
	return s
}

// extractName pulls the name from "Name <email@example.com>" format
func extractName(s string) *string {
	s = strings.TrimSpace(s)
	if start := strings.Index(s, "<"); start > 0 {
		name := strings.TrimSpace(s[:start])
		if name != "" {
			return &name
		}
	}
	return nil
}
