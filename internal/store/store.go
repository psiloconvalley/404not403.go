package store

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// ── Errors ────────────────────────────────────────────────────────────────────

type storeError string

func (e storeError) Error() string { return string(e) }

const ErrNotOwner storeError = "you do not own this resource"
const ErrNotFound storeError = "record not found"
const ErrLimitReached storeError = "tier limit reached"

// ── ConnectDB ─────────────────────────────────────────────────────────────────

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
// ── Helpers ───────────────────────────────────────────────────────────────────

func nullableInt(n int) interface{} {
	if n == 0 {
		return nil
	}
	return n
}
