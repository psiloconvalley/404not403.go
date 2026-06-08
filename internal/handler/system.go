package handler

import (
	"net/http"
	"github.com/psiloconvalley/404not403/internal/app"
)

func Home(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			if err := a.Templates.ExecuteTemplate(w, "404.html", nil); err != nil {
				http.Error(w, "Not Found", http.StatusNotFound)
			}
			return
		}
		if err := a.Templates.ExecuteTemplate(w, "index.html", nil); err != nil {
			http.Error(w, "System Error", http.StatusInternalServerError)
		}
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
func Status(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbStatus := "ok"
		if a.DB == nil {
			dbStatus = "offline"
		} else if err := a.DB.Ping(); err != nil {
			dbStatus = "error"
		}

		data := map[string]string{
			"DBStatus": dbStatus,
		}

		if err := a.Templates.ExecuteTemplate(w, "status.html", data); err != nil {
			http.Error(w, "System Error", http.StatusInternalServerError)
		}
	}
}
