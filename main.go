package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/auth"
	"github.com/psiloconvalley/404not403/internal/handler"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/provider/ai"
	"github.com/psiloconvalley/404not403/internal/store"
)

func main() {
	// 0. Validate required environment variables — fail fast, not fail later
	validateEnv(
		"DATABASE_URL",
		"JWT_PRIVATE_KEY",
		"JWT_PUBLIC_KEY",
	)

	a := &app.App{}

	// 1. Infrastructure
	a.DB = store.ConnectDB()
	if err := a.DB.Ping(); err != nil {
		log.Fatalf("❌ Database unreachable: %v", err)
	}
	log.Println("✅ Database connected and reachable.")

	store.RunMigrations(a.DB)
	a.HTTPClient = app.NewHTTPClient()
	a.Limiter = app.NewLimiterMap()

	// 2. AI Provider — disabled until OPENAI_API_KEY is configured
	a.AI = ai.NewDisabled()
	log.Println("✅ AI provider initialized (disabled — no API key configured).")

	// 3. JWT Keys — must be set in environment
	if err := auth.InitKeys(); err != nil {
		log.Fatalf("❌ JWT init failed: %v — cannot start without auth keys", err)
	}
	log.Println("✅ JWT keys loaded.")

	// 4. Templates
	tmpl, err := template.ParseGlob(filepath.Join("templates", "*.html"))
	if err != nil {
		log.Fatalf("❌ Template error: %v", err)
	}
	a.Templates = tmpl
	log.Println("✅ Templates parsed.")

	// 5. Shutdown context — cancelled on SIGTERM or SIGINT
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 6. Router
	mux := http.NewServeMux()

	// Static files
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// ── Public routes ─────────────────────────────────────────────────
	mux.HandleFunc("/", handler.Home(a))
	mux.HandleFunc("/health", handler.Health(a))

	// ── Auth routes ───────────────────────────────────────────────────
	mux.HandleFunc("/api/auth/register", handler.Register(a))
	mux.HandleFunc("/api/auth/login", handler.Login(a))
	mux.HandleFunc("/api/auth/logout", handler.Logout)
	mux.HandleFunc("/api/auth/me", middleware.RequireAuth(a, handler.Me(a)))
	mux.HandleFunc("/api/auth/check-handle", handler.CheckHandle(a))
	mux.HandleFunc("/api/auth/forgot", handler.ForgotPassword(a))
	mux.HandleFunc("/api/auth/reset", handler.ResetPassword(a))
	mux.HandleFunc("/reset", handler.ResetPage(a))
	mux.HandleFunc("/api/auth/mfa/setup", middleware.RequireAuth(a, handler.MFASetup(a)))
	mux.HandleFunc("/api/auth/mfa/verify", middleware.RequireAuth(a, handler.MFAVerify(a)))
	mux.HandleFunc("/api/auth/mfa/disable", middleware.RequireAuth(a, handler.MFADisable(a)))

	// ── Billing ───────────────────────────────────────────────────────
	mux.HandleFunc("/api/billing/checkout", middleware.RequireAuth(a, handler.CreateCheckoutSession(a)))
	mux.HandleFunc("/api/webhooks/stripe", handler.StripeWebhook(a))

	// 7. Middleware chain
	wrapped := middleware.RateLimiter(a)(mux)
	wrapped = middleware.Logger(wrapped)

	// 8. Server
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

	// 9. Start server in goroutine — main goroutine waits for shutdown signal
	go func() {
		log.Printf("🚀 404NOT403 Online — port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Server error: %v", err)
		}
	}()

	// 10. Block until shutdown signal received
	<-ctx.Done()
	log.Println("⏳ Shutdown signal received — draining connections...")

	// 11. Graceful shutdown — 30 second drain window
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("⚠️  Server forced to close: %v", err)
	}

	log.Println("✅ 404NOT403 shut down cleanly.")
}

// validateEnv checks that all required environment variables are set.
// Logs all missing vars before fatally exiting — fail fast, not fail later.
func validateEnv(keys ...string) {
	missing := []string{}
	for _, k := range keys {
		if os.Getenv(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		for _, k := range missing {
			log.Printf("❌ Missing required environment variable: %s", k)
		}
		log.Fatalf("❌ Cannot start — %d required environment variable(s) missing.", len(missing))
	}
}
