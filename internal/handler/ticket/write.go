package ticket

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/handler/shared"
	"github.com/psiloconvalley/404not403/internal/middleware"
	ticketsvc "github.com/psiloconvalley/404not403/internal/service/ticket"
)

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
		Subject       string  `json:"subject"`
		Body          string  `json:"body"`
		Priority      string  `json:"priority"`
		SourceType    string  `json:"source_type"`
		ThreadID      *string `json:"thread_id"`
		CustomerEmail string  `json:"customer_email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	customerEmail := strings.TrimSpace(input.CustomerEmail)
	var customerEmailPtr *string
	if customerEmail != "" {
		customerEmailPtr = &customerEmail
	}

	// Default source type to web for agent-created tickets
	if input.SourceType == "" {
		input.SourceType = string(domain.SourceWeb)
	}

	result, err := h.svc.Create(r.Context(), ticketsvc.CreateInput{
		OrgID:         orgID,
		Subject:       input.Subject,
		Body:          input.Body,
		Priority:      input.Priority,
		SourceType:    input.SourceType,
		ThreadID:      input.ThreadID,
		CustomerEmail: customerEmailPtr,
	})
	if err != nil {
		writeError(w, shared.DomainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, result.Ticket)
}

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
		writeError(w, shared.DomainErrStatus(err), err.Error())
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
		writeError(w, shared.DomainErrStatus(err), err.Error())
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
		writeError(w, shared.DomainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, comment)
}

// ── Update Priority ───────────────────────────────────────────────────────────

// UpdatePriority handles POST /api/orgs/{orgID}/tickets/{ticketID}/priority
func (h *Handler) UpdatePriority(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Path: /api/orgs/{orgID}/tickets/{ticketID}/priority
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	path = strings.TrimSuffix(path, "/priority")
	parts := strings.SplitN(path, "/tickets/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	ticketID := parts[1]

	var input struct {
		Priority string `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if input.Priority == "" {
		writeError(w, http.StatusBadRequest, "priority is required")
		return
	}

	if err := h.svc.UpdatePriority(r.Context(), orgID, ticketID, userID, input.Priority); err != nil {
		writeError(w, shared.DomainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"priority": input.Priority})
}
