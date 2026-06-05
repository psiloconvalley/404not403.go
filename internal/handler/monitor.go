package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/store"
)

// Monitor dispatches POST and DELETE on /api/monitor.
func Monitor(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			createMonitor(a, w, r)
		case http.MethodDelete:
			deactivateMonitor(a, w, r)
		default:
			http.Error(w, `{"error":"use POST or DELETE"}`, http.StatusMethodNotAllowed)
		}
	}
}

func createMonitor(a *app.App, w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var input struct {
		URL      string `json:"url"`
		Interval string `json:"interval"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	input.URL = strings.TrimSpace(input.URL)
	if input.URL == "" {
		http.Error(w, `{"error":"url is required"}`, http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(input.URL, "http://") && !strings.HasPrefix(input.URL, "https://") {
		input.URL = "https://" + input.URL
	}

	switch input.Interval {
	case "1h", "6h", "24h":
	case "":
		input.Interval = "1h"
	default:
		http.Error(w, `{"error":"interval must be 1h, 6h, or 24h"}`, http.StatusBadRequest)
		return
	}

	m, err := store.CreateMonitor(a.DB, userID, input.URL, input.Interval)
	if err != nil {
		if err == store.ErrMonitorLimitReached {
			http.Error(w, `{"error":"monitor limit reached — max 10 per account"}`, http.StatusTooManyRequests)
			return
		}
		http.Error(w, `{"error":"failed to create monitor"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(m)
}

func deactivateMonitor(a *app.App, w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	if err := store.DeactivateMonitor(a.DB, id, userID); err != nil {
		if err == store.ErrNotOwner {
			http.Error(w, `{"error":"you do not own this monitor"}`, http.StatusForbidden)
			return
		}
		http.Error(w, `{"error":"failed to deactivate monitor"}`, http.StatusInternalServerError)
		return
	}

	w.Write([]byte(`{"status":"deactivated"}`))
}

// ListMonitors handles GET /api/monitors
func ListMonitors(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		userID := middleware.GetUserID(r)

		monitors, err := store.ListMonitors(a.DB, userID)
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
func ListChanges(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		userID := middleware.GetUserID(r)
		url := r.URL.Query().Get("url")

		changes, err := store.ListChanges(a.DB, userID, url)
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
