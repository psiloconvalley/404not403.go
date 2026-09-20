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
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/provider/email"
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

// sendAutoReply sends a confirmation email with a tracking link and clarifying questions.
func (h *Handler) sendAutoReply(ctx context.Context, org *store.Organization, customer *store.Customer, toEmail string, name *string, displayID, subject string) {
	if h.app.Email == nil {
		return
	}

	displayName := "there"
	if name != nil && *name != "" {
		displayName = *name
	}

	// Ensure customer has a tracking token
	rawToken, tokenHash, err := generateTrackingToken()
	if err == nil {
		expires := time.Now().Add(30 * 24 * time.Hour)
		_ = store.SetTrackingToken(h.app.DB, customer.ID, tokenHash, expires)
	}

	trackingURL := "https://404not403.com"
	if rawToken != "" {
		trackingURL = fmt.Sprintf("https://404not403.com/help/track?token=%s", rawToken)
	}

	emailSubject := fmt.Sprintf("Re: %s [%s]", subject, displayID)

	emailBody := fmt.Sprintf(`
<div style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;max-width:600px;margin:0 auto;padding:2rem;color:#292524;background:#ffffff;border:1px solid #e7e5e4;border-radius:8px;">
	<div style="margin-bottom:1.5rem;border-bottom:1px solid #f5f5f4;padding-bottom:1rem;">
		<span style="display:inline-block;padding:0.25rem 0.6rem;background:#fef3c7;color:#92400e;font-size:0.8rem;font-weight:600;border-radius:4px;letter-spacing:0.05em;">%s</span>
	</div>

	<h2 style="font-size:1.25rem;font-weight:700;margin:0 0 1rem 0;color:#1c1917;">We've received your request</h2>

	<p style="color:#57534e;line-height:1.6;margin:0 0 1rem 0;">
		Hi %s, your request has been logged under reference <strong>%s</strong>. Our IT support team is actively reviewing it.
	</p>

	<div style="background:#fafaf9;border-left:3px solid #ea580c;padding:1rem;margin:1.5rem 0;border-radius:0 4px 4px 0;">
		<p style="font-size:0.875rem;font-weight:600;color:#1c1917;margin:0 0 0.5rem 0;">To help us resolve this as fast as possible, please reply with:</p>
		<ol style="margin:0;padding-left:1.25rem;font-size:0.875rem;color:#57534e;line-height:1.5;">
			<li>When did this issue first start?</li>
			<li>Is this only affecting you, or multiple people on your team?</li>
			<li>What device model or operating system are you using?</li>
		</ol>
		<p style="font-size:0.8rem;color:#78716c;margin:0.5rem 0 0 0;">(You can simply reply directly to this email)</p>
	</div>

	<div style="margin:1.5rem 0;">
		<a href="%s" style="display:inline-block;padding:0.65rem 1.25rem;background:#ea580c;color:#ffffff;text-decoration:none;font-weight:600;font-size:0.875rem;border-radius:6px;">View & Track Request</a>
	</div>

	<div style="border-top:1px solid #f5f5f4;padding-top:1rem;margin-top:2rem;font-size:0.75rem;color:#a8a29e;">
		<p style="margin:0;">%s Support · Powered by 404NOT403</p>
	</div>
</div>`,
		displayID,
		displayName,
		displayID,
		trackingURL,
		org.Name,
	)

	_ = h.app.Email.Send(ctx, email.SendInput{
		To:      toEmail,
		Subject: emailSubject,
		Body:    emailBody,
	})
}

func generateTrackingToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, hash, nil
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
