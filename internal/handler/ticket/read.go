package ticket

import (
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/store"
	ticketsvc "github.com/psiloconvalley/404not403/internal/service/ticket"
)

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

	tickets, err := h.svc.List(r.Context(), ticketsvc.ListInput{RequestingUserID: userID,
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
