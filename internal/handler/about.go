package handler

import (
	"net/http"
	"github.com/psiloconvalley/404not403/internal/app"
)

func About(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.Templates.ExecuteTemplate(w, "about.html", nil); err != nil {
			http.Error(w, "System Error", http.StatusInternalServerError)
		}
	}
}
