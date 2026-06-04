package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/handler"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/interna:l/store"
	"html/template"
)

func main() {
	a := &app.App{}

	// 1. Infrastructure
	a.DB = store.ConnectDB()
	store.RunMigrations(a.DB)
	a.HTTPClient = app.NewHTTPClient()
	a.Limiter = app.NewLimiterMap()

	// 2. Templates
	tmpl, err := template.ParseGlob(filepath.Join("templates", "*.html"))
	if err != nil {
		log.Fatalf("❌ Template error: %v", err)
	}
	a.Templates = tmpl

	// 3. Router
	mux := http.NewServeMux()
	
	// Static
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Routes
	mux.HandleFunc("/", handler.Home(a))
	mux.HandleFunc("/health", handler.Health(a))
	mux.HandleFunc("/simulate/404", handler.Simulate404(a))
	mux.HandleFunc("/simulate/403", handler.Simulate403(a))
	mux.HandleFunc("/api/stats", handler.Stats(a))
	mux.HandleFunc("/api/scan", handler.Scan(a))

	// 4. Middleware
	wrapped := middleware.RateLimiter(a)(mux)
	wrapped = middleware.Logger(wrapped)

	// 5. Server
	port := os.Getenv("PORT")
	if port == "" { port = "8080" }

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      wrapped,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("🚀 404NOT403 Engine Online on %s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
