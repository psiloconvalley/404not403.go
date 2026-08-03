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
	"net/http"

	"github.com/psiloconvalley/404not403/internal/app"
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

