package queue

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/middleware"
	queuesvc "github.com/psiloconvalley/404not403/internal/service/queue"
	"github.com/psiloconvalley/404not403/internal/store"
)
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

// UpdateQueueSettings handles POST /api/orgs/{orgID}/queues/{queueID}/settings
func (h *Handler) UpdateQueueSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	path = strings.TrimSuffix(path, "/settings")
	parts := strings.SplitN(path, "/queues/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	queueID := parts[1]

	var input struct {
		Name       *string `json:"name"`
		Prefix     *string `json:"prefix"`
		Department *string `json:"department"`
		Color      *string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := store.UpdateQueue(h.app.DB, orgID, queueID, input.Name, input.Prefix, nil, input.Department, input.Color, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update queue")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

