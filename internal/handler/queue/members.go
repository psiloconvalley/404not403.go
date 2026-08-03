package queue

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/store"
)
// ── Queue Members ────────────────────────────────────────────────────────────

// AddMember handles POST /api/orgs/{orgID}/queues/{queueID}/members
func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
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
	path = strings.TrimSuffix(path, "/members")
	parts := strings.SplitN(path, "/queues/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	queueID := parts[1]

	var input struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if input.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if input.Role == "" {
		input.Role = "member"
	}

	if err := store.AddQueueMember(h.app.DB, orgID, queueID, input.UserID, input.Role); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

// RemoveMember handles DELETE /api/orgs/{orgID}/queues/{queueID}/members/{userID}
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "use DELETE")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Path: /api/orgs/{orgID}/queues/{queueID}/members/{targetUserID}
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	queueParts := strings.SplitN(path, "/queues/", 2)
	if len(queueParts) != 2 {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	memberParts := strings.SplitN(queueParts[1], "/members/", 2)
	if len(memberParts) != 2 || memberParts[0] == "" || memberParts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	queueID := memberParts[0]
	targetUserID := memberParts[1]

	if err := store.RemoveQueueMember(h.app.DB, queueID, targetUserID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}
