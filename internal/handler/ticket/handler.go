// Package ticket implements HTTP handlers for ticket management.
//
// Responsibilities:
//   - Parse HTTP requests
//   - Extract and validate identity from JWT
//   - Derive org context from request (never trust client-provided org_id)
//   - Call the ticket service
//   - Write JSON responses
//
// No SQL. No business rules. No direct store calls.
// All logic lives in internal/service/ticket.
package ticket

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/store"
	ticketsvc "github.com/psiloconvalley/404not403/internal/service/ticket"
)


// Handler holds handler dependencies.
type Handler struct {
	app *app.App
	svc *ticketsvc.Service
}

// New returns a new ticket Handler.
func New(a *app.App) *Handler {
	return &Handler{
		app: a,
		svc: ticketsvc.New(a.DB),
	}
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

// orgIDFromPath extracts the org ID from the URL path.
// Routes are structured as /api/orgs/{orgID}/tickets/...
// This prevents clients from supplying arbitrary org IDs.
func orgIDFromPath(r *http.Request, prefix string) string {
	path := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// domainErrStatus maps domain errors to HTTP status codes.
func domainErrStatus(err error) int {
	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusForbidden
	case errors.Is(err, domain.ErrInvalidTransition):
		return http.StatusUnprocessableEntity
	case errors.Is(err, domain.ErrInvalidPriority):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrInvalidStatus):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrTicketClosed):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// ── Create ────────────────────────────────────────────────────────────────────

// Create handles POST /api/orgs/{orgID}/tickets
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	orgID := orgIDFromPath(r, "/api/orgs/")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	var input struct {
		Subject    string  `json:"subject"`
		Body       string  `json:"body"`
		Priority   string  `json:"priority"`
		SourceType string  `json:"source_type"`
		ThreadID   *string `json:"thread_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Default source type to web for agent-created tickets
	if input.SourceType == "" {
		input.SourceType = string(domain.SourceWeb)
	}

	result, err := h.svc.Create(r.Context(), ticketsvc.CreateInput{
		OrgID:      orgID,
		Subject:    input.Subject,
		Body:       input.Body,
		Priority:   input.Priority,
		SourceType: input.SourceType,
		ThreadID:   input.ThreadID,
	})
	if err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, result.Ticket)
}

// ── Get ───────────────────────────────────────────────────────────────────────

// Get handles GET /api/orgs/{orgID}/tickets/{ticketID}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Path: /api/orgs/{orgID}/tickets/{ticketID}
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	parts := strings.SplitN(path, "/tickets/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	ticketID := parts[1]

	ctx, err := h.svc.Get(r.Context(), orgID, ticketID)
	if err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ctx)
}

// ── List ──────────────────────────────────────────────────────────────────────

// List handles GET /api/orgs/{orgID}/tickets
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	orgID := orgIDFromPath(r, "/api/orgs/")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	// Optional query filters
	q := r.URL.Query()
	var status, priority, assignedTo *string
	if v := q.Get("status"); v != "" {
		status = &v
	}
	if v := q.Get("priority"); v != "" {
		priority = &v
	}
	if v := q.Get("assigned_to"); v != "" {
		assignedTo = &v
	}

	tickets, err := h.svc.List(r.Context(), ticketsvc.ListInput{
		OrgID:      orgID,
		Status:     status,
		Priority:   priority,
		AssignedTo: assignedTo,
		Limit:      50,
	})
	if err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	if tickets == nil {
		tickets = []store.Ticket{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tickets": tickets,
		"count":   len(tickets),
	})
}

// ── Update Status ─────────────────────────────────────────────────────────────

// UpdateStatus handles POST /api/orgs/{orgID}/tickets/{ticketID}/status
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Path: /api/orgs/{orgID}/tickets/{ticketID}/status
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	path = strings.TrimSuffix(path, "/status")
	parts := strings.SplitN(path, "/tickets/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	ticketID := parts[1]

	var input struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if input.Status == "" {
		writeError(w, http.StatusBadRequest, "status is required")
		return
	}

	if err := h.svc.UpdateStatus(r.Context(), orgID, ticketID, userID, input.Status); err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": input.Status})
}

// ── Assign ────────────────────────────────────────────────────────────────────

// Assign handles POST /api/orgs/{orgID}/tickets/{ticketID}/assign
func (h *Handler) Assign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Path: /api/orgs/{orgID}/tickets/{ticketID}/assign
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	path = strings.TrimSuffix(path, "/assign")
	parts := strings.SplitN(path, "/tickets/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	ticketID := parts[1]

	var input struct {
		AssigneeUserID string `json:"assignee_user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if input.AssigneeUserID == "" {
		writeError(w, http.StatusBadRequest, "assignee_user_id is required")
		return
	}

	if err := h.svc.Assign(r.Context(), orgID, ticketID, userID, input.AssigneeUserID); err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"assigned_to": input.AssigneeUserID,
	})
}

// ── Add Comment ───────────────────────────────────────────────────────────────

// AddComment handles POST /api/orgs/{orgID}/tickets/{ticketID}/comments
func (h *Handler) AddComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Path: /api/orgs/{orgID}/tickets/{ticketID}/comments
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	path = strings.TrimSuffix(path, "/comments")
	parts := strings.SplitN(path, "/tickets/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	ticketID := parts[1]

	var input struct {
		Body       string `json:"body"`
		IsInternal bool   `json:"is_internal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	comment, err := h.svc.AddComment(r.Context(), ticketsvc.AddCommentInput{
		OrgID:      orgID,
		TicketID:   ticketID,
		AuthorID:   &userID,
		Body:       input.Body,
		IsInternal: input.IsInternal,
		SourceType: string(domain.SourceApp),
	})
	if err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, comment)
}

// ── Search ────────────────────────────────────────────────────────────────────

// Search handles GET /api/orgs/{orgID}/tickets/search?q=...
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	orgID := orgIDFromPath(r, "/api/orgs/")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}

	tickets, err := h.svc.Search(r.Context(), orgID, query, 50)
	if err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	if tickets == nil {
		tickets = []store.Ticket{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tickets": tickets,
		"count":   len(tickets),
	})
}
