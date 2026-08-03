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
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/app"
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


// ── Create ────────────────────────────────────────────────────────────────────

