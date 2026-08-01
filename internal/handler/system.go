package handler

import (
	"net/http"

	"github.com/psiloconvalley/404not403/internal/app"
)

func Home(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"error":"not found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"service":"404not403","status":"operational"}`))
	}
}

func Health(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		dbStatus := "ok"
		if a.DB == nil {
			dbStatus = "offline"
		} else if err := a.DB.Ping(); err != nil {
			dbStatus = "error"
		}
		status := http.StatusOK
		if dbStatus != "ok" {
			status = http.StatusServiceUnavailable
		}
		w.WriteHeader(status)
		w.Write([]byte(`{"status":"` + dbStatus + `"}`))
	}
}
