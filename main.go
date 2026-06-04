package main

import (
	"database/sql"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/time/rate"
)

// ── App ──────────────────────────────────────────────────────────────────────
// App carries all shared dependencies. No globals except this struct.
// Every handler is a method on App.
type App struct {
	db         *sql.DB
	templates  *template.Template
	httpClient *http.Client
	limiterMap *LimiterMap
}

// ── LimiterMap ────────────────────────────────────────────────────────────────
// Per-IP rate limiter store. Thread-safe.
// Each IP gets: 5 requests per second, burst of 10.
type LimiterMap struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func newLimiterMap() *LimiterMap {
	return &LimiterMap{
		limiters: make(map[string]*rate.Limiter),
	}
}

func (lm *LimiterMap) get(ip string) *rate.Limiter {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	l, exists := lm.limiters[ip]
	if !exists {
		// 5 requests/second sustained, burst of 10
		l = rate.NewLimiter(rate.Every(200*time.Millisecond), 10)
		lm.limiters[ip] = l
	}
	return l
}

// ── main ──────────────────────────────────────────────────────────────────────
func main() {
	app := &App{}

	// 1. Database
	app.db = connectDB()

	// 2. Migrations
	runMigrations(app.db)

	// 3. Templates — parsed once at startup, cached
	tmpl, err := template.ParseGlob(filepath.Join("templates", "*.html"))
	if err != nil {
		log.Fatalf("❌ Template parse error: %v", err)
	}
	app.templates = tmpl

	// 4. Outbound HTTP client — strict timeout, no redirects leaked
	app.httpClient = &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	// 5. Rate limiter map
	app.limiterMap = newLimiterMap()

	// 6. Port
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 7. Router
	mux := http.NewServeMux()

	// Static files
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Routes — all handlers are methods on App
	mux.HandleFunc("/", app.handleHome)
	mux.HandleFunc("/health", app.handleHealth)
	mux.HandleFunc("/simulate/404", app.handleSimulate404)
	mux.HandleFunc("/simulate/403", app.handleSimulate403)
	mux.HandleFunc("/api/stats", app.handleStats)

	// 8. Middleware chain: logger → rate limiter → router
	handler := app.rateLimiterMiddleware(mux)
	handler = requestLoggerMiddleware(handler)

	// 9. Server
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("🚀 404not403 Engine Online. Port %s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// ── Database ──────────────────────────────────────────────────────────────────
func connectDB() *sql.DB {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Println("ℹ️  No DATABASE_URL found. Running without DB.")
		return nil
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Printf("⚠️  DB open error: %v", err)
		return nil
	}

	// Connection pool config — intentional for production
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Printf("⚠️  DB ping failed: %v", err)
		return nil
	}

	log.Println("✅ Postgres connected.")
	return db
}

// ── Migrations ────────────────────────────────────────────────────────────────
// Runs at startup. Idempotent. Safe to run every deploy.
func runMigrations(db *sql.DB) {
	if db == nil {
		log.Println("ℹ️  Skipping migrations — no DB connection.")
		return
	}

	migrations := []struct {
		name string
		sql  string
	}{
		{
			name: "create_logs_table",
			sql: `CREATE TABLE IF NOT EXISTS logs (
				id          SERIAL PRIMARY KEY,
				status_code INTEGER NOT NULL,
				message     TEXT,
				created_at  TIMESTAMP DEFAULT now()
			)`,
		},
		{
			name: "create_scans_table",
			sql: `CREATE TABLE IF NOT EXISTS scans (
				id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				url          TEXT NOT NULL,
				status_code  INTEGER,
				headers      JSONB,
				body_hash    TEXT,
				body_size    INTEGER,
				tls_issuer   TEXT,
				tls_expiry   TIMESTAMP,
				server       TEXT,
				cdn          TEXT,
				waf          TEXT,
				region       TEXT DEFAULT 'us-east',
				duration_ms  INTEGER,
				error        TEXT,
				created_at   TIMESTAMP DEFAULT now()
			)`,
		},
		{
			name: "create_index_scans_url",
			sql:  `CREATE INDEX IF NOT EXISTS idx_scans_url ON scans(url)`,
		},
		{
			name: "create_index_scans_created",
			sql:  `CREATE INDEX IF NOT EXISTS idx_scans_created ON scans(created_at)`,
		},
		{
			name: "create_index_scans_status",
			sql:  `CREATE INDEX IF NOT EXISTS idx_scans_status ON scans(status_code)`,
		},
	}

	for _, m := range migrations {
		if _, err := db.Exec(m.sql); err != nil {
			log.Fatalf("❌ Migration failed [%s]: %v", m.name, err)
		}
		log.Printf("✅ Migration OK: %s", m.name)
	}
}

// ── Middleware ────────────────────────────────────────────────────────────────
func requestLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s %dms",
			r.Method,
			r.URL.Path,
			realIP(r),
			time.Since(start).Milliseconds(),
		)
	})
}

func (app *App) rateLimiterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := realIP(r)
		limiter := app.limiterMap.get(ip)
		if !limiter.Allow() {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// realIP extracts the real client IP, respecting Railway's proxy headers.
func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		// X-Forwarded-For can be a comma-separated list — take the first
		for i := 0; i < len(ip); i++ {
			if ip[i] == ',' {
				return ip[:i]
			}
		}
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ── Handlers ──────────────────────────────────────────────────────────────────
func (app *App) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if err := app.templates.ExecuteTemplate(w, "index.html", nil); err != nil {
		log.Printf("❌ Template execute error: %v", err)
		http.Error(w, "System Error", http.StatusInternalServerError)
	}
}

func (app *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	dbStatus := "ok"
	if app.db == nil {
		dbStatus = "offline"
	} else if err := app.db.Ping(); err != nil {
		dbStatus = "error"
	}

	status := http.StatusOK
	if dbStatus != "ok" {
		status = http.StatusServiceUnavailable
	}

	w.WriteHeader(status)
	w.Write([]byte(`{"status":"` + dbStatus + `"}`))
}

func (app *App) handleSimulate404(w http.ResponseWriter, r *http.Request) {
	go app.logEvent(404, "Not Found - resource is missing.")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`{
		"status": 404,
		"error": "Not Found",
		"message": "The resource is missing.",
		"tip": "It is gone. Not forbidden. Just gone."
	}`))
}

func (app *App) handleSimulate403(w http.ResponseWriter, r *http.Request) {
	go app.logEvent(403, "Forbidden - access denied.")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(`{
		"status": 403,
		"error": "Forbidden",
		"message": "Access denied.",
		"tip": "It exists. You just are not on the list."
	}`))
}

func (app *App) logEvent(statusCode int, message string) {
	if app.db == nil {
		log.Println("⚠️  logEvent: db is nil, skipping.")
		return
	}
	_, err := app.db.Exec(
		"INSERT INTO logs (status_code, message) VALUES ($1, $2)",
		statusCode,
		message,
	)
	if err != nil {
		log.Printf("⚠️  logEvent insert error: %v", err)
	} else {
		log.Printf("✅ Logged: %d - %s", statusCode, message)
	}
}

func (app *App) handleStats(w http.ResponseWriter, r *http.Request) {
	// CORS: locked to our own origin only
	origin := r.Header.Get("Origin")
	if origin == "https://404not403.com" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Set("Content-Type", "application/json")

	if app.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"database offline"}`))
		return
	}

	var total, count404, count403 int

	if err := app.db.QueryRow("SELECT COUNT(*) FROM logs").Scan(&total); err != nil {
		log.Printf("⚠️  Stats total query error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"query failed"}`))
		return
	}

	// These are non-fatal — partial stats are acceptable
	app.db.QueryRow("SELECT COUNT(*) FROM logs WHERE status_code = 404").Scan(&count404)
	app.db.QueryRow("SELECT COUNT(*) FROM logs WHERE status_code = 403").Scan(&count403)

	w.Write([]byte(`{"total":` + itoa(total) +
		`,"404s":` + itoa(count404) +
		`,"403s":` + itoa(count403) + `}`))
}

// itoa converts int to string without importing strconv or fmt
// for simple JSON assembly only
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 20)
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte(n%10) + '0'
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
