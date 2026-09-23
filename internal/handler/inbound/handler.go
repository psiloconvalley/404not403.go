// Package inbound handles incoming webhooks from email providers.
//
// Responsibilities:
//   - Receive raw webhook payloads
//   - Store immediately (never lose data)
//   - Parse and route: thread replies to existing tickets OR create new tickets
//   - Send auto-reply confirmation with clarifying questions and tracking link
//   - Handle duplicates gracefully via external_id uniqueness
package inbound

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/domain"
	ticketsvc "github.com/psiloconvalley/404not403/internal/service/ticket"
	"github.com/psiloconvalley/404not403/internal/store"
)

var displayIDRegex = regexp.MustCompile(`\[([A-Z0-9]+-[A-Z]+-[0-9]+)\]`)

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
// Receives inbound emails from Resend/webhook providers and creates or threads tickets.
func (h *Handler) ResendEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}

	// Verify webhook secret if configured
	webhookSecret := os.Getenv("INBOUND_WEBHOOK_SECRET")
	if webhookSecret != "" {
		providedSecret := r.Header.Get("X-Webhook-Secret")
		if providedSecret != webhookSecret {
			log.Printf("inbound: invalid webhook secret")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Read raw body — we store this before any parsing
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("inbound: failed to read body: %v", err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Parse enough to extract headers and message details
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

	// Extract Message-ID, In-Reply-To, and References for idempotency and threading
	var messageID, inReplyTo string
	var references []string
	for _, hdr := range payload.Data.Headers {
		if strings.EqualFold(hdr.Name, "Message-ID") {
			messageID = strings.TrimSpace(hdr.Value)
		} else if strings.EqualFold(hdr.Name, "In-Reply-To") {
			inReplyTo = strings.TrimSpace(hdr.Value)
		} else if strings.EqualFold(hdr.Name, "References") {
			for _, ref := range strings.Fields(hdr.Value) {
				cleanRef := strings.TrimSpace(ref)
				if cleanRef != "" {
					references = append(references, cleanRef)
				}
			}
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

	ticketBody := payload.Data.Text
	if ticketBody == "" {
		ticketBody = payload.Data.HTML
	}
	if ticketBody == "" {
		ticketBody = "(empty message body)"
	}

	subject := strings.TrimSpace(payload.Data.Subject)
	if subject == "" {
		subject = "(no subject)"
	}

	// ── Check For Threading (Reply to existing ticket) ─────────────────────────
	existingTicketID := h.findThreadedTicket(org.ID, inReplyTo, references, subject)

	if existingTicketID != "" {
		// Threading path: append comment to existing ticket
		_, err := h.svc.AddComment(r.Context(), ticketsvc.AddCommentInput{
			OrgID:      org.ID,
			TicketID:   existingTicketID,
			CustomerID: &customer.ID,
			Body:       ticketBody,
			SourceType: string(domain.SourceEmail),
			ExternalID: &messageID,
		})
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
				log.Printf("inbound: comment %s already added to ticket %s", messageID, existingTicketID)
				w.WriteHeader(http.StatusOK)
				return
			}
			log.Printf("inbound: failed to add comment to ticket %s: %v", existingTicketID, err)
			http.Error(w, "failed to thread comment", http.StatusInternalServerError)
			return
		}

		// Reopen ticket if resolved or closed
		existingTicket, err := store.GetTicketByID(h.app.DB, org.ID, existingTicketID)
		if err == nil && existingTicket != nil {
			if existingTicket.Status == "resolved" || existingTicket.Status == "closed" {
				_ = store.UpdateTicketStatus(h.app.DB, org.ID, existingTicketID, "system", "reopened")
			}
		}

		_ = store.MarkInboundProcessed(h.app.DB, msg.ID, org.ID, existingTicketID)
		log.Printf("inbound: threaded reply %s to ticket %s", messageID, existingTicketID)
		w.WriteHeader(http.StatusOK)
		return
	}

	// ── New Ticket Creation Path ──────────────────────────────────────────────
	// 1. Evaluate deterministic routing rules (Layer 1 of Routing Waterfall)
	targetQueueID, matchedRuleID, err := store.MatchTicketToQueue(h.app.DB, org.ID, subject, ticketBody)
	if err != nil {
		log.Printf("inbound: error matching routing rules: %v", err)
	}

	if matchedRuleID != "" {
		_ = store.IncrementRuleHitCount(h.app.DB, matchedRuleID)
		log.Printf("inbound: ticket matched routing rule %s -> queue %s", matchedRuleID, targetQueueID)
	} else {
		// Fallback: Ensure ticket always lands in the Triage queue (Layer 3 fallback)
		triageID, err := store.EnsureTriageQueue(h.app.DB, org.ID)
		if err != nil {
			log.Printf("inbound: failed to resolve triage queue: %v", err)
		} else {
			targetQueueID = triageID
			log.Printf("inbound: ticket routed to default triage queue %s", targetQueueID)
		}
	}

	var queueIDPtr *string
	if targetQueueID != "" {
		queueIDPtr = &targetQueueID
	}

	result, err := h.svc.Create(r.Context(), ticketsvc.CreateInput{
		OrgID:                 org.ID,
		QueueID:               queueIDPtr,
		CustomerID:            &customer.ID,
		Subject:               subject,
		Body:                  ticketBody,
		SourceType:            string(domain.SourceEmail),
		ThreadID:              &messageID,
		SubmittedByCustomerID: &customer.ID,
		RequesterCustomerID:   &customer.ID,
	})
	if err != nil {
		log.Printf("inbound: failed to create ticket: %v", err)
		http.Error(w, "failed to create ticket", http.StatusInternalServerError)
		return
	}

	_ = store.MarkInboundProcessed(h.app.DB, msg.ID, org.ID, result.Ticket.ID)
	log.Printf("inbound: created ticket %s (%s) from email %s", result.Ticket.ID, result.Ticket.DisplayID, messageID)

	// Send auto-reply with tracking token and clarifying questions
	h.sendAutoReply(r.Context(), org, customer, fromAddress, fromName, result.Ticket.DisplayID, subject)

	w.WriteHeader(http.StatusOK)
}

// findThreadedTicket attempts to resolve an existing ticket using email headers and subject patterns.
func (h *Handler) findThreadedTicket(orgID, inReplyTo string, references []string, subject string) string {
	// 1. Check In-Reply-To against stored inbound messages
	if inReplyTo != "" {
		if parentMsg, err := store.GetInboundMessageByExternalID(h.app.DB, inReplyTo); err == nil && parentMsg != nil && parentMsg.TicketID != nil {
			return *parentMsg.TicketID
		}
		if t, err := store.GetTicketByThreadID(h.app.DB, inReplyTo); err == nil && t != nil {
			return t.ID
		}
	}

	// 2. Check References list against stored inbound messages
	for _, ref := range references {
		if parentMsg, err := store.GetInboundMessageByExternalID(h.app.DB, ref); err == nil && parentMsg != nil && parentMsg.TicketID != nil {
			return *parentMsg.TicketID
		}
	}

	// 3. Extract [PREFIX-TYPE-NUM] from Subject
	matches := displayIDRegex.FindStringSubmatch(subject)
	if len(matches) > 1 {
		displayID := matches[1]
		if t, err := store.GetTicketByDisplayID(h.app.DB, orgID, displayID); err == nil && t != nil {
			return t.ID
		}
	}

	return ""
}
