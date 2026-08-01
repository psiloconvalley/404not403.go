// Package queue implements HTTP handlers for queue management.
//
// Queues are department-owned ticket containers.
// This handler exposes CRUD for queues, sidebar data,
// and ticket-to-queue assignment.
//
// No SQL. No business rules. Delegates to service/queue.
package queue

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/store"
	queuesvc "github.com/psiloconvalley/404not403/internal/service/queue"
)

// Handler holds queue handler dependencies.
type Handler struct {
	app *app.App
	svc *queuesvc.Service
}

// New returns a new queue Handler.
func New(a *app.App) *Handler {
	return &Handler{
		app: a,
		svc: queuesvc.New(a.DB),
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

func domainErrStatus(err error) int {
	if errors.Is(err, domain.ErrUnauthorized) {
		return http.StatusForbidden
	}
	return http.StatusInternalServerError
}

// ── Create Queue ──────────────────────────────────────────────────────────────

// Create handles POST /api/orgs/{orgID}/queues
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

	// Path: /api/orgs/{orgID}/queues
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	path = strings.TrimSuffix(path, "/queues")
	orgID := path
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	var input struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Department  *string `json:"department"`
		Color       string  `json:"color"`
		Icon        *string `json:"icon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	q, err := h.svc.Create(r.Context(), queuesvc.CreateInput{
		OrgID:            orgID,
		RequestingUserID: userID,
		Name:             input.Name,
		Description:      input.Description,
		Department:       input.Department,
		Color:            input.Color,
		Icon:             input.Icon,
	})
	if err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, q)
}

// ── Sidebar ───────────────────────────────────────────────────────────────────

// Sidebar handles GET /api/orgs/{orgID}/queues/sidebar
// Returns all queues with ticket counts for the dashboard sidebar.
func (h *Handler) Sidebar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Path: /api/orgs/{orgID}/queues/sidebar
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	path = strings.TrimSuffix(path, "/queues/sidebar")
	orgID := path
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	data, err := h.svc.ListForSidebar(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, data)
}

// ── List Queue Tickets ────────────────────────────────────────────────────────

// ListTickets handles GET /api/orgs/{orgID}/queues/{queueID}/tickets
func (h *Handler) ListTickets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Path: /api/orgs/{orgID}/queues/{queueID}/tickets
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	parts := strings.SplitN(path, "/queues/", 2)
	if len(parts) != 2 {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	queueID := strings.TrimSuffix(parts[1], "/tickets")

	if orgID == "" || queueID == "" {
		writeError(w, http.StatusBadRequest, "org_id and queue_id are required")
		return
	}

	tickets, err := h.svc.ListTicketsByQueue(r.Context(), orgID, queueID, userID)
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

// ── Assign Ticket To Queue ────────────────────────────────────────────────────

// AssignTicket handles POST /api/orgs/{orgID}/queues/{queueID}/assign
func (h *Handler) AssignTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Path: /api/orgs/{orgID}/queues/{queueID}/assign
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	parts := strings.SplitN(path, "/queues/", 2)
	if len(parts) != 2 {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	queueID := strings.TrimSuffix(parts[1], "/assign")

	var input struct {
		TicketID string `json:"ticket_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if input.TicketID == "" {
		writeError(w, http.StatusBadRequest, "ticket_id is required")
		return
	}

	if err := h.svc.AssignTicket(r.Context(), orgID, queueID, input.TicketID, userID); err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "assigned",
		"queue_id": queueID,
	})
}
