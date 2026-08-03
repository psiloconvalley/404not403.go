// Package catalog implements HTTP handlers for service catalog management.
//
// A catalog item is a template for ticket creation.
// Each item defines the type, queue, priority, and SLA for a service.
// Departments manage their own catalog items through queue settings.
package catalog

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/store"
)

// Handler holds catalog handler dependencies.
type Handler struct {
	app *app.App
}

// New returns a new catalog Handler.
func New(a *app.App) *Handler {
	return &Handler{app: a}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Create handles POST /api/orgs/{orgID}/catalog
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

	orgID := orgIDFromPath(r)
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	var input struct {
		QueueID         *string `json:"queue_id"`
		Name            string  `json:"name"`
		Description     *string `json:"description"`
		Category        *string `json:"category"`
		TicketType      string  `json:"ticket_type"`
		DefaultPriority string  `json:"default_priority"`
		SLAHours        *int    `json:"sla_hours"`
		Icon            *string `json:"icon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if input.TicketType == "" {
		input.TicketType = "service_request"
	}
	if input.DefaultPriority == "" {
		input.DefaultPriority = "P2"
	}

	item, err := store.CreateCatalogItem(
		h.app.DB, orgID, input.QueueID, input.Name,
		input.Description, input.Category,
		input.TicketType, input.DefaultPriority,
		input.SLAHours, input.Icon,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create catalog item")
		return
	}

	writeJSON(w, http.StatusCreated, item)
}

// List handles GET /api/orgs/{orgID}/catalog
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}

	orgID := orgIDFromPath(r)
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	// Optional queue filter
	queueID := r.URL.Query().Get("queue_id")

	var items []store.CatalogItem
	var err error
	if queueID != "" {
		items, err = store.ListCatalogItemsByQueue(h.app.DB, orgID, queueID)
	} else {
		items, err = store.ListCatalogItems(h.app.DB, orgID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load catalog")
		return
	}

	if items == nil {
		items = []store.CatalogItem{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

// Update handles POST /api/orgs/{orgID}/catalog/{itemID}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
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
	parts := strings.SplitN(path, "/catalog/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	itemID := parts[1]

	var input struct {
		Name            *string `json:"name"`
		Description     *string `json:"description"`
		Category        *string `json:"category"`
		TicketType      *string `json:"ticket_type"`
		DefaultPriority *string `json:"default_priority"`
		Icon            *string `json:"icon"`
		QueueID         *string `json:"queue_id"`
		SLAHours        *int    `json:"sla_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := store.UpdateCatalogItem(
		h.app.DB, orgID, itemID,
		input.Name, input.Description, input.Category,
		input.TicketType, input.DefaultPriority, input.Icon,
		input.QueueID, input.SLAHours,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// Delete handles DELETE /api/orgs/{orgID}/catalog/{itemID}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "use DELETE")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	parts := strings.SplitN(path, "/catalog/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	itemID := parts[1]

	if err := store.DeactivateCatalogItem(h.app.DB, orgID, itemID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// orgIDFromPath extracts org ID from /api/orgs/{orgID}/catalog...
func orgIDFromPath(r *http.Request) string {
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}
