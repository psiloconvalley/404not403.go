package handler

import (
	"fmt"
	"net/http"
	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/store"
)

func Simulate404(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		go store.LogEvent(a.DB, 404, "Not Found simulation")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"status": 404, "error": "Not Found", "message": "The resource is missing.", "tip": "It is gone. Not forbidden. Just gone."}`))
	}
}

func Simulate403(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		go store.LogEvent(a.DB, 403, "Forbidden simulation")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"status": 403, "error": "Forbidden", "message": "Access denied.", "tip": "It exists. You just are not on the list."}`))
	}
}

func Stats(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "https://404not403.com" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Content-Type", "application/json")

		if a.DB == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"database offline"}`))
			return
		}

		var total, count404, count403 int
		a.DB.QueryRow("SELECT COUNT(*) FROM logs").Scan(&total)
		a.DB.QueryRow("SELECT COUNT(*) FROM logs WHERE status_code = 404").Scan(&count404)
		a.DB.QueryRow("SELECT COUNT(*) FROM logs WHERE status_code = 403").Scan(&count403)

		w.Write([]byte(fmt.Sprintf(`{"total":%d,"404s":%d,"403s":%d}`, total, count404, count403)))
	}
}
