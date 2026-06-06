package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/auth"
	"github.com/psiloconvalley/404not403/internal/handler"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/monitor"
	"github.com/psiloconvalley/404not403/internal/store"
)

func main() {
	a := &app.App{}

	// 1. Infrastructure
	a.DB = store.ConnectDB()
	store.RunMigrations(a.DB)
	a.HTTPClient = app.NewHTTPClient()
	a.Limiter = app.NewLimiterMap()

	// 2. JWT Keys — must be set in environment
	if err := auth.InitKeys(); err != nil {
		log.Printf("⚠️  JWT init: %v — auth endpoints will not work", err)
	}

	// 3. Templates
	tmpl, err := template.ParseGlob(filepath.Join("templates", "*.html"))
	if err != nil {
		log.Fatalf("❌ Template error: %v", err)
	}
	a.Templates = tmpl

	// 4. Ghost Link Monitor worker
	go monitor.StartWorker(a)

	// 5. Router
	mux := http.NewServeMux()

	// Static files
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// ── Public routes ─────────────────────────────────────────────────
	mux.HandleFunc("/", handler.Home(a))
	mux.HandleFunc("/about", handler.About(a))
	mux.HandleFunc("/health", handler.Health(a))
	mux.HandleFunc("/simulate/404", handler.Simulate404(a))
	mux.HandleFunc("/simulate/403", handler.Simulate403(a))
	mux.HandleFunc("/api/stats", handler.Stats(a))
	mux.HandleFunc("/api/scan", handler.Scan(a))
	mux.HandleFunc("/api/scans", middleware.RequireAuth(a, handler.RecentScans(a)))
	mux.HandleFunc("/api/feed", handler.GlobalFeed(a))

	// ── Auth routes ───────────────────────────────────────────────────
	mux.HandleFunc("/api/auth/register", handler.Register(a))
	mux.HandleFunc("/api/auth/login", handler.Login(a))
	mux.HandleFunc("/api/auth/logout", handler.Logout)
	mux.HandleFunc("/api/auth/me", middleware.RequireAuth(a, handler.Me(a)))
	mux.HandleFunc("/api/auth/check-handle", handler.CheckHandle(a))
	mux.HandleFunc("/api/auth/forgot", handler.ForgotPassword(a))
	mux.HandleFunc("/api/auth/reset", handler.ResetPassword(a))
	mux.HandleFunc("/reset", handler.ResetPage(a))

	// ── Protected routes — require authentication ─────────────────────
	mux.HandleFunc("/api/monitor", middleware.RequireAuth(a, handler.Monitor(a)))
	mux.HandleFunc("/api/monitors", middleware.RequireAuth(a, handler.ListMonitors(a)))
	mux.HandleFunc("/api/changes", middleware.RequireAuth(a, handler.ListChanges(a)))

	// 6. Middleware chain
	wrapped := middleware.RateLimiter(a)(mux)
	wrapped = middleware.Logger(wrapped)

	// 7. Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      wrapped,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("🚀 404NOT403 Engine Online on port %s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
