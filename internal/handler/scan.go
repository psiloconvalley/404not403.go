package handler

import (
	"encoding/json"
	"net/http"
	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/scanner"
	"github.com/psiloconvalley/404not403/internal/store"
)

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

		if a.DB != nil {
			store.StoreScan(a.DB, result)
		}

		json.NewEncoder(w).Encode(result)
	}
}
