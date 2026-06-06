package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/scanner"
	"github.com/psiloconvalley/404not403/internal/store"
)

// Scan handles POST /api/scan — public endpoint, optionally linked to user
func Scan(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
			return
		}

		origin := r.Header.Get("Origin")
		if origin == "https://404not403.com" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Content-Type", "application/json")

		var input struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}

		result := scanner.Scan(a, input.URL)

		// Link scan to user if authenticated, otherwise empty string
		userID := middleware.GetUserID(r)

		if a.DB != nil {
			store.StoreScan(a.DB, userID, result)
		}

		json.NewEncoder(w).Encode(result)
	}
}

// RecentScans handles GET /api/scans — returns user's private scan history
func RecentScans(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		userID := middleware.GetUserID(r)

		limitStr := r.URL.Query().Get("limit")
		limit := 10
		if limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 50 {
				limit = n
			}
		}

		scans, err := store.RecentScans(a.DB, userID, limit)
		if err != nil {
			http.Error(w, `{"error":"failed to load scans"}`, http.StatusInternalServerError)
			return
		}

		if scans == nil {
			scans = []store.ScanRecord{}
		}

		json.NewEncoder(w).Encode(scans)
	}
}

// GlobalFeed handles GET /api/feed — public global scan activity
func GlobalFeed(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		limitStr := r.URL.Query().Get("limit")
		limit := 20
		if limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 50 {
				limit = n
			}
		}

		scans, err := store.GlobalFeed(a.DB, limit)
		if err != nil {
			http.Error(w, `{"error":"failed to load feed"}`, http.StatusInternalServerError)
			return
		}

		if scans == nil {
			scans = []store.ScanRecord{}
		}

		json.NewEncoder(w).Encode(scans)
	}
}
