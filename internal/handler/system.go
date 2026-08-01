package handler

import (
	"net/http"

	"github.com/psiloconvalley/404not403/internal/app"
)

func Home(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			if err := a.Templates.ExecuteTemplate(w, "index.html", nil); err != nil {
				http.Error(w, "Not Found", http.StatusNotFound)
			}
			return
		}
		if err := a.Templates.ExecuteTemplate(w, "index.html", nil); err != nil {
			http.Error(w, "System Error", http.StatusInternalServerError)
		}
	}
}

func LoginPage(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.Templates.ExecuteTemplate(w, "login.html", nil); err != nil {
			http.Error(w, "System Error", http.StatusInternalServerError)
		}
	}
}

func RegisterPage(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.Templates.ExecuteTemplate(w, "register.html", nil); err != nil {
			http.Error(w, "System Error", http.StatusInternalServerError)
		}
	}
}

func Dashboard(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.Templates.ExecuteTemplate(w, "dashboard.html", nil); err != nil {
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
