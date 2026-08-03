// Package help implements the employee-facing support portal.
//
// No account required. Zero friction.
// Employees submit requests via a simple form.
// They track requests via a token-based link sent to their email.
//
// Routes:
//   GET  /help/{slug}          → submit form for an org
//   POST /help/{slug}          → process submission
//   GET  /help/track           → verify token, show tickets
//   POST /help/track/comment   → add comment via portal
package help

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/store"
)

// Handler holds help portal dependencies.
type Handler struct {
	app *app.App
}

// New returns a new help Handler.
func New(a *app.App) *Handler {
	return &Handler{app: a}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func generateToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, hash, nil
}

func extractDomain(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}

// ── Portal Page ───────────────────────────────────────────────────────────────

// Portal serves the help form page (GET) or processes submission (POST).
// GET  /help/{slug} → show form
// POST /help/{slug} → process submission
func (h *Handler) Portal(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/help/")
	slug = strings.TrimSuffix(slug, "/")
	if slug == "" || slug == "track" {
		return // handled by other routes
	}

	switch r.Method {
	case http.MethodGet:
		h.showForm(w, r, slug)
	case http.MethodPost:
		h.Submit(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "use GET or POST")
	}
}

// showForm renders the help submission form with service catalog.
func (h *Handler) showForm(w http.ResponseWriter, r *http.Request, slug string) {
	org, err := store.GetOrgBySlug(h.app.DB, slug)
	if err != nil || org == nil {
		http.Error(w, "Organization not found", http.StatusNotFound)
		return
	}

	// Load service catalog for this org
	catalogItems, err := store.ListCatalogItems(h.app.DB, org.ID)
	if err != nil {
		catalogItems = []store.CatalogItem{}
	}

	// Group catalog items by queue for department display
	queues, err := store.ListQueuesWithCounts(h.app.DB, org.ID)
	if err != nil {
		queues = []store.QueueWithCounts{}
	}

	if err := h.app.Templates.ExecuteTemplate(w, "help.html", map[string]interface{}{
		"OrgName":  org.Name,
		"OrgSlug":  org.Slug,
		"OrgID":    org.ID,
		"Catalog":  catalogItems,
		"Queues":   queues,
	}); err != nil {
		http.Error(w, "System Error", http.StatusInternalServerError)
	}
}


// ── Submit Request ────────────────────────────────────────────────────────────

// Submit processes a help request submission.
// POST /help/{slug}
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	slug := strings.TrimPrefix(r.URL.Path, "/help/")
	slug = strings.TrimSuffix(slug, "/")

	// Find the org
	org, err := store.GetOrgBySlug(h.app.DB, slug)
	if err != nil || org == nil {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}

	// Parse input
	var input struct {
		Name           string `json:"name"`
		Email          string `json:"email"`
		Subject        string `json:"subject"`
		Body           string `json:"body"`
		Urgency        string `json:"urgency"`
		CatalogItemID  string `json:"catalog_item_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Validate
	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Subject = strings.TrimSpace(input.Subject)
	input.Body = strings.TrimSpace(input.Body)

	if input.Name == "" || input.Email == "" || input.Subject == "" || input.Body == "" {
		writeError(w, http.StatusBadRequest, "all fields are required")
		return
	}

	if !strings.Contains(input.Email, "@") || !strings.Contains(input.Email, ".") {
		writeError(w, http.StatusBadRequest, "invalid email")
		return
	}

	// Domain validation
	if org.Domain != nil && *org.Domain != "" {
		emailDomain := extractDomain(input.Email)
		_ = emailDomain // non-blocking for now
	}

	// Resolve catalog item → queue, type, priority, SLA
	var queueID *string
	var ticketType string = "service_request"
	priority := "P2"

	if input.CatalogItemID != "" {
		catalogItem, err := store.GetCatalogItem(h.app.DB, org.ID, input.CatalogItemID)
		if err == nil && catalogItem != nil {
			queueID = catalogItem.QueueID
			ticketType = catalogItem.TicketType
			priority = catalogItem.DefaultPriority
			// SLA will be wired when sla_due_at is implemented
		}
	} else {
		// Fallback: map urgency to priority for generic requests
		switch input.Urgency {
		case "low":
			priority = "P3"
		case "high":
			priority = "P1"
		case "critical":
			priority = "P0"
		}
	}

	// Find or create customer
	customer, err := store.FindOrCreateCustomerByEmail(h.app.DB, org.ID, input.Email, &input.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to process request")
		return
	}

	// Generate tracking token if customer doesn't have one
	if customer.TrackingTokenHash == nil {
		rawToken, tokenHash, err := generateToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate tracking token")
			return
		}

		expires := time.Now().Add(30 * 24 * time.Hour)
		if err := store.SetTrackingToken(h.app.DB, customer.ID, tokenHash, expires); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store tracking token")
			return
		}
		_ = rawToken
	}

	// Create ticket with catalog-derived routing
	createParams := store.CreateTicketParams{
		OrgID:      org.ID,
		CustomerID: &customer.ID,
		Subject:    input.Subject,
		Body:       input.Body,
		Priority:   priority,
		SourceType: string(domain.SourceWeb),
		TicketType: ticketType,
	}

	ticket, err := store.CreateTicket(h.app.DB, createParams)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}

	// Assign to queue if catalog item specified one
	if queueID != nil {
		store.AssignTicketToQueue(h.app.DB, org.ID, ticket.ID, *queueID)
	}

	// Enqueue AI classification job
	idempotencyKey := fmt.Sprintf("ai.classify.%s", ticket.ID)
	payload, _ := json.Marshal(map[string]string{
		"ticket_id": ticket.ID,
		"org_id":    org.ID,
	})
	store.EnqueueJob(h.app.DB, store.EnqueueJobParams{
		OrgID:          &org.ID,
		JobType:        store.JobTypeAIClassify,
		Payload:        payload,
		IdempotencyKey: &idempotencyKey,
		MaxAttempts:    3,
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"status":    "received",
		"ticket_id": ticket.ID,
		"message":   "Your request has been submitted. Check your email for a tracking link.",
	})
}

// ── Track Requests ────────────────────────────────────────────────────────────

// Track shows an employee their submitted tickets.
// GET /help/track?token=RAW_TOKEN
func (h *Handler) Track(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}

	rawToken := strings.TrimSpace(r.URL.Query().Get("token"))
	if rawToken == "" {
		http.Error(w, "Token required", http.StatusBadRequest)
		return
	}

	// Hash the token to look it up
	hashBytes := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hashBytes[:])

	customer, err := store.GetCustomerByTrackingToken(h.app.DB, tokenHash)
	if err != nil || customer == nil {
		http.Error(w, "Invalid or expired tracking link", http.StatusUnauthorized)
		return
	}

	// Load their tickets
	tickets, err := store.GetTicketsForCustomer(h.app.DB, customer.OrgID, customer.ID)
	if err != nil {
		http.Error(w, "Failed to load requests", http.StatusInternalServerError)
		return
	}

	// Get org name for display
	org, _ := store.GetOrgByID(h.app.DB, customer.OrgID)
	orgName := ""
	if org != nil {
		orgName = org.Name
	}

	if err := h.app.Templates.ExecuteTemplate(w, "help_track.html", map[string]interface{}{
		"Customer": customer,
		"Tickets":  tickets,
		"OrgName":  orgName,
		"Token":    rawToken,
	}); err != nil {
		http.Error(w, "System Error", http.StatusInternalServerError)
	}
}

// ── Add Comment From Portal ───────────────────────────────────────────────────

// AddComment lets an employee add a comment to their ticket.
// POST /help/track/comment
func (h *Handler) AddComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	var input struct {
		Token    string `json:"token"`
		TicketID string `json:"ticket_id"`
		Body     string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	input.Body = strings.TrimSpace(input.Body)
	if input.Token == "" || input.TicketID == "" || input.Body == "" {
		writeError(w, http.StatusBadRequest, "token, ticket_id, and body are required")
		return
	}

	// Verify token
	hashBytes := sha256.Sum256([]byte(input.Token))
	tokenHash := hex.EncodeToString(hashBytes[:])

	customer, err := store.GetCustomerByTrackingToken(h.app.DB, tokenHash)
	if err != nil || customer == nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	// Verify ticket belongs to this customer
	ticket, err := store.GetTicketByID(h.app.DB, customer.OrgID, input.TicketID)
	if err != nil || ticket == nil {
		writeError(w, http.StatusNotFound, "ticket not found")
		return
	}
	if ticket.CustomerID == nil || *ticket.CustomerID != customer.ID {
		writeError(w, http.StatusForbidden, "not your ticket")
		return
	}

	// Create comment
	_, err = store.CreateComment(h.app.DB, store.CreateCommentParams{
		OrgID:      customer.OrgID,
		TicketID:   input.TicketID,
		CustomerID: &customer.ID,
		Body:       input.Body,
		IsInternal: false,
		SourceType: string(domain.SourceWeb),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add comment")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"status": "comment added",
	})
}
