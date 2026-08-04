package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/auth"
	"github.com/psiloconvalley/404not403/internal/handler"
	helphandler "github.com/psiloconvalley/404not403/internal/handler/help"
	queuehandler "github.com/psiloconvalley/404not403/internal/handler/queue"
	orghandler "github.com/psiloconvalley/404not403/internal/handler/org"
	tickethandler "github.com/psiloconvalley/404not403/internal/handler/ticket"
	inboundhandler "github.com/psiloconvalley/404not403/internal/handler/inbound"
	cataloghandler "github.com/psiloconvalley/404not403/internal/handler/catalog"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/provider/ai"
	"github.com/psiloconvalley/404not403/internal/provider/email"
	"github.com/psiloconvalley/404not403/internal/store"
	"github.com/psiloconvalley/404not403/internal/worker"
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
	a.Email = email.NewResendProvider(a.HTTPClient)
	log.Println("✅ AI provider initialized (disabled — no API key configured).")

	// 3. JWT Keys — must be set in environment
	if err := auth.InitKeys(); err != nil {
		log.Fatalf("❌ JWT init failed: %v — cannot start without auth keys", err)
	}
	log.Println("✅ JWT keys loaded.")

	// 4. Templates — with custom functions for portal rendering
	funcMap := template.FuncMap{
		"lower": strings.ToLower,
		"replace": func(s, old, new string) string {
			return strings.ReplaceAll(s, old, new)
		},
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob(filepath.Join("templates", "*.html"))
	if err != nil {
		log.Fatalf("❌ Template error: %v", err)
	}
	a.Templates = tmpl
	log.Println("✅ Templates parsed.")

	// 5. Shutdown context — cancelled on SIGTERM or SIGINT
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 6. Initialize handlers
	tickets := tickethandler.New(a)
	orgs := orghandler.New(a)
	helpPortal := helphandler.New(a)
	queues := queuehandler.New(a)
	inbound := inboundhandler.New(a)
	catalog := cataloghandler.New(a)

	// 7. Worker — background job processor (AI enrichment)
	go worker.Start(ctx, a)

	// 8. Router
	mux := http.NewServeMux()

	// Static files
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// ── Public routes ─────────────────────────────────────────────────
	mux.HandleFunc("/", handler.Home(a))
	mux.HandleFunc("/health", handler.Health(a))
	mux.HandleFunc("/login", handler.LoginPage(a))
	mux.HandleFunc("/register", handler.RegisterPage(a))
	mux.HandleFunc("/dashboard", handler.Dashboard(a))

	// ── Employee Portal (public — no auth) ────────────────────────────
	mux.HandleFunc("/help/track/comment", helpPortal.AddComment)
	mux.HandleFunc("/help/track", helpPortal.Track)
	mux.HandleFunc("/help/", helpPortal.Portal)


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
	mux.HandleFunc("/api/auth/passkey/register/begin", middleware.RequireAuth(a, handler.PasskeyRegisterBegin(a)))
	mux.HandleFunc("/api/auth/passkey/register/finish", middleware.RequireAuth(a, handler.PasskeyRegisterFinish(a)))
	mux.HandleFunc("/api/auth/passkey/login/begin", handler.PasskeyLoginBegin(a))
	mux.HandleFunc("/api/auth/passkey/login/finish", handler.PasskeyLoginFinish(a))
	mux.HandleFunc("/api/auth/passkeys", middleware.RequireAuth(a, handler.PasskeyList(a)))
	mux.HandleFunc("/api/auth/passkeys/delete", middleware.RequireAuth(a, handler.PasskeyDelete(a)))

	// ── Org routes ────────────────────────────────────────────────────
	mux.HandleFunc("/api/orgs", middleware.RequireAuth(a, orgs.Create))
	mux.HandleFunc("/api/orgs/me", middleware.RequireAuth(a, orgs.ListMine))
	mux.HandleFunc("/api/orgs/", middleware.RequireAuth(a, orgRouter(orgs, tickets, queues, catalog)))

	// ── Ticket routes ─────────────────────────────────────────────────
	// All ticket routes go through /api/orgs/{orgID}/tickets/...
	// The orgRouter delegates to the ticket handler for ticket paths.

	// ── Billing ───────────────────────────────────────────────────────
	mux.HandleFunc("/api/billing/checkout", middleware.RequireAuth(a, handler.CreateCheckoutSession(a)))
	mux.HandleFunc("/api/webhooks/stripe", handler.StripeWebhook(a))
	mux.HandleFunc("/api/webhooks/inbound-email", inbound.ResendEmail)

	// 9. Middleware chain
	wrapped := middleware.RateLimiter(a)(mux)
	wrapped = middleware.Logger(wrapped)

	// 10. Server
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

	// 11. Start server in goroutine — main goroutine waits for shutdown signal
	go func() {
		log.Printf("🚀 404NOT403 Online — port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Server error: %v", err)
		}
	}()

	// 12. Block until shutdown signal received
	<-ctx.Done()
	log.Println("⏳ Shutdown signal received — draining connections...")

	// 13. Graceful shutdown — 30 second drain window
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("⚠️  Server forced to close: %v", err)
	}

	log.Println("✅ 404NOT403 shut down cleanly.")
}

// orgRouter routes /api/orgs/{orgID}/... requests to the correct handler.
// This avoids registering dozens of individual routes.
// Standard library ServeMux matches by prefix — this function does the sub-routing.
func orgRouter(orgs *orghandler.Handler, tickets *tickethandler.Handler, qh *queuehandler.Handler, cat *cataloghandler.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		// Queue routes: /api/orgs/{orgID}/queues/...
		case contains(path, "/queues/sidebar"):
			qh.Sidebar(w, r)
		case contains(path, "/queues") && contains(path, "/assign"):
			qh.AssignTicket(w, r)
		case contains(path, "/queues") && contains(path, "/tickets"):
			qh.ListTickets(w, r)
		case hasSuffix(path, "/queues"):
			qh.Create(w, r)
		case contains(path, "/queues") && contains(path, "/settings"):
			qh.UpdateQueueSettings(w, r)
		case contains(path, "/queues") && contains(path, "/members/"):
			qh.RemoveMember(w, r)
		case contains(path, "/queues") && hasSuffix(path, "/members"):
			qh.AddMember(w, r)
		case contains(path, "/queues/") && !contains(path, "/tickets") && !contains(path, "/assign") && !contains(path, "/sidebar"):
			qh.GetQueue(w, r)

		// Ticket routes: /api/orgs/{orgID}/tickets/...
		case contains(path, "/tickets/search"):
			tickets.Search(w, r)
		case contains(path, "/tickets") && contains(path, "/status"):
			tickets.UpdateStatus(w, r)
		case contains(path, "/tickets") && contains(path, "/assign"):
			tickets.Assign(w, r)
		case contains(path, "/tickets") && contains(path, "/priority"):
			tickets.UpdatePriority(w, r)
		case contains(path, "/tickets") && contains(path, "/comments"):
			tickets.AddComment(w, r)
		case contains(path, "/tickets/"):
			tickets.Get(w, r)
		case hasSuffix(path, "/tickets"):
			if r.Method == http.MethodPost {
				tickets.Create(w, r)
			} else {
				tickets.List(w, r)
			}

		// Member routes: /api/orgs/{orgID}/members/...
		case contains(path, "/members") && contains(path, "/role"):
			orgs.UpdateRole(w, r)
		case contains(path, "/members/"):
			orgs.RemoveMember(w, r)
		case hasSuffix(path, "/members"):
			orgs.Invite(w, r)

		// Org settings: /api/orgs/{orgID}/settings
		case hasSuffix(path, "/settings"):
			orgs.UpdateSettings(w, r)


		// Catalog routes: /api/orgs/{orgID}/catalog/...
		case contains(path, "/catalog/"):
			if r.Method == http.MethodDelete {
				cat.Delete(w, r)
			} else {
				cat.Update(w, r)
			}
		case hasSuffix(path, "/catalog"):
			if r.Method == http.MethodPost {
				cat.Create(w, r)
			} else {
				cat.List(w, r)
			}
		// Org detail: /api/orgs/{orgID}
		default:
			orgs.Get(w, r)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
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
