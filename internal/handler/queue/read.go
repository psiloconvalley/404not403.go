package queue

import (
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/handler/shared"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/store"
)
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
		writeError(w, shared.DomainErrStatus(err), err.Error())
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
		writeError(w, shared.DomainErrStatus(err), err.Error())
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

// ── Get Queue ────────────────────────────────────────────────────────────────

// GetQueue handles GET /api/orgs/{orgID}/queues/{queueID}
func (h *Handler) GetQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Path: /api/orgs/{orgID}/queues/{queueID}
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	parts := strings.SplitN(path, "/queues/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	queueID := parts[1]

	queue, err := store.GetQueueByID(h.app.DB, orgID, queueID)
	if err != nil {
		writeError(w, http.StatusNotFound, "queue not found")
		return
	}

	members, err := store.ListQueueMembers(h.app.DB, orgID, queueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load members")
		return
	}
	if members == nil {
		members = []store.QueueMember{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"queue":   queue,
		"members": members,
	})
}

// ── Update Queue ─────────────────────────────────────────────────────────────

