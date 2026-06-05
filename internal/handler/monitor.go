package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/store"
)

// CreateMonitor handles POST /api/monitor
// Accepts: {"url": "https://example.com", "interval": "1h"}
func CreateMonitor(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		var input struct {
			URL      string `json:"url"`
			Interval string `json:"interval"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}

		// Validate URL
		input.URL = strings.TrimSpace(input.URL)
		if input.URL == "" {
			http.Error(w, `{"error":"url is required"}`, http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(input.URL, "http://") && !strings.HasPrefix(input.URL, "https://") {
			input.URL = "https://" + input.URL
		}

		// Validate interval — only allow known values
		switch input.Interval {
		case "1h", "6h", "24h":
			// valid
		case "":
			input.Interval = "1h"
		default:
			http.Error(w, `{"error":"interval must be 1h, 6h, or 24h"}`, http.StatusBadRequest)
			return
		}

		m, err := store.CreateMonitor(a.DB, input.URL, input.Interval)
		if err != nil {
			if err == store.ErrMonitorLimitReached {
				http.Error(w, `{"error":"monitor limit reached"}`, http.StatusTooManyRequests)
				return
			}
			http.Error(w, `{"error":"failed to create monitor"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(m)
	}
}

// ListMonitors handles GET /api/monitors
func ListMonitors(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		monitors, err := store.ListMonitors(a.DB)
		if err != nil {
			http.Error(w, `{"error":"failed to list monitors"}`, http.StatusInternalServerError)
			return
		}

		if monitors == nil {
			monitors = []store.Monitor{}
		}

		json.NewEncoder(w).Encode(monitors)
	}
}

// ListChanges handles GET /api/changes
// Optional query param: ?url=https://example.com
func ListChanges(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		url := r.URL.Query().Get("url")

		changes, err := store.ListChanges(a.DB, url)
		if err != nil {
			http.Error(w, `{"error":"failed to list changes"}`, http.StatusInternalServerError)
			return
		}

		if changes == nil {
			changes = []store.Change{}
		}

		json.NewEncoder(w).Encode(changes)
	}
}

// DeactivateMonitor handles DELETE /api/monitor?id=...
func DeactivateMonitor(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, `{"error":"use DELETE"}`, http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
			return
		}

		if err := store.DeactivateMonitor(a.DB, id); err != nil {
			http.Error(w, `{"error":"failed to deactivate monitor"}`, http.StatusInternalServerError)
			return
		}

		w.Write([]byte(`{"status":"deactivated"}`))
	}
}
