package store

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/psiloconvalley/404not403/internal/scanner"
)

// ConnectDB initializes the Postgres connection pool.
func ConnectDB() *sql.DB {
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

// RunMigrations creates tables and indexes if they do not exist.
func RunMigrations(db *sql.DB) {
	if db == nil {
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

// LogEvent is for the simulator legacy tracking.
func LogEvent(db *sql.DB, statusCode int, message string) {
	if db == nil {
		return
	}
	_, err := db.Exec(
		"INSERT INTO logs (status_code, message) VALUES ($1, $2)",
		statusCode,
		message,
	)
	if err != nil {
		log.Printf("⚠️  LogEvent error: %v", err)
	}
}

// StoreScan writes a ScanResult to the database.
func StoreScan(db *sql.DB, r scanner.ScanResult) error {
	if db == nil {
		return nil
	}

	// We pass the raw map to the DB executor — pq handles the JSONB conversion
	// but we must ensure we aren't passing nil to non-nullable fields.
	var tlsExpiry interface{}
	if !r.TLSExpiry.IsZero() {
		tlsExpiry = r.TLSExpiry
	}

	_, err := db.Exec(`
		INSERT INTO scans (
			url, status_code, headers, body_hash, body_size,
			tls_issuer, tls_expiry, server, cdn, waf,
			region, duration_ms, error, created_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14
		)`,
		r.URL,
		nullableInt(r.StatusCode),
		r.Headers, // pg driver handles map[string]string -> jsonb
		r.BodyHash,
		r.BodySize,
		r.TLSIssuer,
		tlsExpiry,
		r.Server,
		r.CDN,
		r.WAF,
		r.Region,
		r.DurationMS,
		r.Error,
		r.ScannedAt,
	)
	return err
}

func nullableInt(n int) interface{} {
	if n == 0 {
		return nil
	}
	return n
}
