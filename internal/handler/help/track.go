package help

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/store"
)

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
