package store

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/psiloconvalley/404not403/internal/scanner"
)

// ── Monitor represents a tracked URL ─────────────────────────────────────────
type Monitor struct {
	ID            string     `json:"id"`
	URL           string     `json:"url"`
	CheckInterval string     `json:"check_interval"`
	LastStatus    *int       `json:"last_status"`
	LastHash      *string    `json:"last_hash"`
	ChangeCount   int        `json:"change_count"`
	Active        bool       `json:"active"`
	LastChecked   *time.Time `json:"last_checked"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ── Change represents a detected state change on a monitored URL ──────────────
type Change struct {
	ID         string    `json:"id"`
	MonitorID  string    `json:"monitor_id"`
	URL        string    `json:"url"`
	OldStatus  int       `json:"old_status"`
	NewStatus  int       `json:"new_status"`
	OldHash    string    `json:"old_hash"`
	NewHash    string    `json:"new_hash"`
	DetectedAt time.Time `json:"detected_at"`
}

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

// ── RunMigrations ─────────────────────────────────────────────────────────────
// Idempotent. Safe to run on every deploy.
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
		{
			name: "create_monitors_table",
			sql: `CREATE TABLE IF NOT EXISTS monitors (
				id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				url            TEXT NOT NULL,
				check_interval TEXT NOT NULL DEFAULT '1h',
				last_status    INTEGER,
				last_hash      TEXT,
				change_count   INTEGER NOT NULL DEFAULT 0,
				active         BOOLEAN NOT NULL DEFAULT true,
				last_checked   TIMESTAMP,
				created_at     TIMESTAMP NOT NULL DEFAULT now()
			)`,
		},
		{
			name: "create_index_monitors_active",
			sql:  `CREATE INDEX IF NOT EXISTS idx_monitors_active ON monitors(active)`,
		},
		{
			name: "create_changes_table",
			sql: `CREATE TABLE IF NOT EXISTS changes (
				id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				monitor_id   UUID NOT NULL REFERENCES monitors(id),
				url          TEXT NOT NULL,
				old_status   INTEGER,
				new_status   INTEGER,
				old_hash     TEXT,
				new_hash     TEXT,
				detected_at  TIMESTAMP NOT NULL DEFAULT now()
			)`,
		},
		{
			name: "create_index_changes_monitor",
			sql:  `CREATE INDEX IF NOT EXISTS idx_changes_monitor ON changes(monitor_id)`,
		},
		{
			name: "create_index_changes_detected",
			sql:  `CREATE INDEX IF NOT EXISTS idx_changes_detected ON changes(detected_at)`,
		},
				{
			name: "create_users_table",
			sql: `CREATE TABLE IF NOT EXISTS users (
				id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				email          TEXT NOT NULL UNIQUE,
				handle         TEXT NOT NULL UNIQUE,
				password_hash  TEXT NOT NULL,
				role           TEXT NOT NULL DEFAULT 'observer',
				mfa_secret     TEXT,
				mfa_enabled    BOOLEAN NOT NULL DEFAULT false,
				email_verified BOOLEAN NOT NULL DEFAULT false,
				last_login     TIMESTAMP,
				created_at     TIMESTAMP NOT NULL DEFAULT now(),
				updated_at     TIMESTAMP NOT NULL DEFAULT now()
			)`,
		},
		{
			name: "create_index_users_email",
			sql:  `CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
		},
		{
			name: "create_index_users_handle",
			sql:  `CREATE INDEX IF NOT EXISTS idx_users_handle ON users(handle)`,
		},
		{
			name: "create_api_keys_table",
			sql: `CREATE TABLE IF NOT EXISTS api_keys (
				id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				user_id    UUID NOT NULL REFERENCES users(id),
				name       TEXT NOT NULL,
				key_hash   TEXT NOT NULL UNIQUE,
				last_used  TIMESTAMP,
				expires_at TIMESTAMP,
				active     BOOLEAN NOT NULL DEFAULT true,
				created_at TIMESTAMP NOT NULL DEFAULT now()
			)`,
		},
		{
			name: "create_index_api_keys_user",
			sql:  `CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id)`,
		},
		{
			name: "create_index_api_keys_hash",
			sql:  `CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash)`,
		},
		{
			name: "add_user_id_to_monitors",
			sql:  `ALTER TABLE monitors ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id)`,
		},
		{
			name: "add_user_id_to_scans",
			sql:  `ALTER TABLE scans ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id)`,
		},
	}

	for _, m := range migrations {
		if _, err := db.Exec(m.sql); err != nil {
			log.Fatalf("❌ Migration failed [%s]: %v", m.name, err)
		}
		log.Printf("✅ Migration OK: %s", m.name)
	}
}

// ── LogEvent ──────────────────────────────────────────────────────────────────
func LogEvent(db *sql.DB, statusCode int, message string) {
	if db == nil {
		return
	}
	_, err := db.Exec(
		"INSERT INTO logs (status_code, message) VALUES ($1, $2)",
		statusCode, message,
	)
	if err != nil {
		log.Printf("⚠️  LogEvent error: %v", err)
	}
}

// ── StoreScan ─────────────────────────────────────────────────────────────────
func StoreScan(db *sql.DB, r scanner.ScanResult) error {
	if db == nil {
		return nil
	}
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
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
		)`,
		r.URL, nullableInt(r.StatusCode), r.Headers,
		r.BodyHash, r.BodySize, r.TLSIssuer, tlsExpiry,
		r.Server, r.CDN, r.WAF, r.Region,
		r.DurationMS, r.Error, r.ScannedAt,
	)
	return err
}

// ── Monitor queries ───────────────────────────────────────────────────────────

// CreateMonitor inserts a new monitor. Returns error if limit reached.
func CreateMonitor(db *sql.DB, url, interval string) (*Monitor, error) {
	// Global limit — 50 active monitors until auth is implemented
	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM monitors WHERE active = true",
	).Scan(&count); err != nil {
		return nil, err
	}
	if count >= 50 {
		return nil, ErrMonitorLimitReached
	}

	var m Monitor
	err := db.QueryRow(`
		INSERT INTO monitors (url, check_interval)
		VALUES ($1, $2)
		RETURNING id, url, check_interval, last_status,
		          last_hash, change_count, active,
		          last_checked, created_at`,
		url, interval,
	).Scan(
		&m.ID, &m.URL, &m.CheckInterval,
		&m.LastStatus, &m.LastHash, &m.ChangeCount,
		&m.Active, &m.LastChecked, &m.CreatedAt,
	)
	return &m, err
}

// ListMonitors returns all active monitors ordered by creation date.
func ListMonitors(db *sql.DB) ([]Monitor, error) {
	rows, err := db.Query(`
		SELECT id, url, check_interval, last_status,
		       last_hash, change_count, active,
		       last_checked, created_at
		FROM monitors
		WHERE active = true
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var monitors []Monitor
	for rows.Next() {
		var m Monitor
		if err := rows.Scan(
			&m.ID, &m.URL, &m.CheckInterval,
			&m.LastStatus, &m.LastHash, &m.ChangeCount,
			&m.Active, &m.LastChecked, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		monitors = append(monitors, m)
	}
	return monitors, rows.Err()
}

// DueMonitors returns monitors that are due for a check.
func DueMonitors(db *sql.DB) ([]Monitor, error) {
	rows, err := db.Query(`
		SELECT id, url, check_interval, last_status,
		       last_hash, change_count, active,
		       last_checked, created_at
		FROM monitors
		WHERE active = true
		AND (
			last_checked IS NULL
			OR (
				check_interval = '1h'  AND last_checked < now() - INTERVAL '1 hour'
				OR check_interval = '6h'  AND last_checked < now() - INTERVAL '6 hours'
				OR check_interval = '24h' AND last_checked < now() - INTERVAL '24 hours'
			)
		)
		ORDER BY last_checked ASC NULLS FIRST
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var monitors []Monitor
	for rows.Next() {
		var m Monitor
		if err := rows.Scan(
			&m.ID, &m.URL, &m.CheckInterval,
			&m.LastStatus, &m.LastHash, &m.ChangeCount,
			&m.Active, &m.LastChecked, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		monitors = append(monitors, m)
	}
	return monitors, rows.Err()
}

// UpdateMonitorState updates the last known state after a check.
func UpdateMonitorState(db *sql.DB, id string, status int, hash string) error {
	_, err := db.Exec(`
		UPDATE monitors
		SET last_status  = $1,
		    last_hash    = $2,
		    last_checked = now()
		WHERE id = $3`,
		nullableInt(status), hash, id,
	)
	return err
}

// IncrementChangeCount bumps the change counter on a monitor.
func IncrementChangeCount(db *sql.DB, id string) error {
	_, err := db.Exec(
		"UPDATE monitors SET change_count = change_count + 1 WHERE id = $1",
		id,
	)
	return err
}

// RecordChange writes a detected change to the evidence log.
func RecordChange(db *sql.DB, c Change) error {
	_, err := db.Exec(`
		INSERT INTO changes (monitor_id, url, old_status, new_status, old_hash, new_hash)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		c.MonitorID, c.URL,
		nullableInt(c.OldStatus), nullableInt(c.NewStatus),
		c.OldHash, c.NewHash,
	)
	return err
}

// ListChanges returns the most recent changes, optionally filtered by URL.
func ListChanges(db *sql.DB, url string) ([]Change, error) {
	query := `
		SELECT c.id, c.monitor_id, c.url,
		       c.old_status, c.new_status,
		       c.old_hash, c.new_hash, c.detected_at
		FROM changes c
		ORDER BY c.detected_at DESC
		LIMIT 100
	`
	args := []interface{}{}

	if url != "" {
		query = `
			SELECT c.id, c.monitor_id, c.url,
			       c.old_status, c.new_status,
			       c.old_hash, c.new_hash, c.detected_at
			FROM changes c
			WHERE c.url = $1
			ORDER BY c.detected_at DESC
			LIMIT 100
		`
		args = append(args, url)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var changes []Change
	for rows.Next() {
		var c Change
		if err := rows.Scan(
			&c.ID, &c.MonitorID, &c.URL,
			&c.OldStatus, &c.NewStatus,
			&c.OldHash, &c.NewHash, &c.DetectedAt,
		); err != nil {
			return nil, err
		}
		changes = append(changes, c)
	}
	return changes, rows.Err()
}

// DeactivateMonitor soft-deletes a monitor.
func DeactivateMonitor(db *sql.DB, id string) error {
	_, err := db.Exec(
		"UPDATE monitors SET active = false WHERE id = $1", id,
	)
	return err
}

// ── Errors ────────────────────────────────────────────────────────────────────
type storeError string

func (e storeError) Error() string { return string(e) }

const ErrMonitorLimitReached storeError = "monitor limit reached — maximum 50 active monitors"

// ── Helpers ───────────────────────────────────────────────────────────────────
func nullableInt(n int) interface{} {
	if n == 0 {
		return nil
	}
	return n
}

// ── User type ─────────────────────────────────────────────────────────────────
type User struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	Handle        string     `json:"handle"`
	PasswordHash  string     `json:"-"` // never serialized
	Role          string     `json:"role"`
	MFASecret     *string    `json:"-"` // never serialized
	MFAEnabled    bool       `json:"mfa_enabled"`
	EmailVerified bool       `json:"email_verified"`
	LastLogin     *time.Time `json:"last_login"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ── APIKey type ───────────────────────────────────────────────────────────────
type APIKey struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Name      string     `json:"name"`
	KeyHash   string     `json:"-"` // never serialized
	LastUsed  *time.Time `json:"last_used"`
	ExpiresAt *time.Time `json:"expires_at"`
	Active    bool       `json:"active"`
	CreatedAt time.Time  `json:"created_at"`
}

// ── User queries ──────────────────────────────────────────────────────────────

// CreateUser inserts a new user. Returns error on duplicate email or handle.
func CreateUser(db *sql.DB, email, handle, passwordHash string) (*User, error) {
	var u User
	err := db.QueryRow(`
		INSERT INTO users (email, handle, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, handle, password_hash, role,
		          mfa_secret, mfa_enabled, email_verified,
		          last_login, created_at, updated_at`,
		email, handle, passwordHash,
	).Scan(
		&u.ID, &u.Email, &u.Handle, &u.PasswordHash, &u.Role,
		&u.MFASecret, &u.MFAEnabled, &u.EmailVerified,
		&u.LastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByEmail retrieves a user by email for login.
func GetUserByEmail(db *sql.DB, email string) (*User, error) {
	var u User
	err := db.QueryRow(`
		SELECT id, email, handle, password_hash, role,
		       mfa_secret, mfa_enabled, email_verified,
		       last_login, created_at, updated_at
		FROM users WHERE email = $1`,
		email,
	).Scan(
		&u.ID, &u.Email, &u.Handle, &u.PasswordHash, &u.Role,
		&u.MFASecret, &u.MFAEnabled, &u.EmailVerified,
		&u.LastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByID retrieves a user by UUID.
func GetUserByID(db *sql.DB, id string) (*User, error) {
	var u User
	err := db.QueryRow(`
		SELECT id, email, handle, password_hash, role,
		       mfa_secret, mfa_enabled, email_verified,
		       last_login, created_at, updated_at
		FROM users WHERE id = $1`,
		id,
	).Scan(
		&u.ID, &u.Email, &u.Handle, &u.PasswordHash, &u.Role,
		&u.MFASecret, &u.MFAEnabled, &u.EmailVerified,
		&u.LastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByHandle retrieves a user by handle.
func GetUserByHandle(db *sql.DB, handle string) (*User, error) {
	var u User
	err := db.QueryRow(`
		SELECT id, email, handle, password_hash, role,
		       mfa_secret, mfa_enabled, email_verified,
		       last_login, created_at, updated_at
		FROM users WHERE handle = $1`,
		handle,
	).Scan(
		&u.ID, &u.Email, &u.Handle, &u.PasswordHash, &u.Role,
		&u.MFASecret, &u.MFAEnabled, &u.EmailVerified,
		&u.LastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateLastLogin stamps the login time.
func UpdateLastLogin(db *sql.DB, userID string) error {
	_, err := db.Exec(
		"UPDATE users SET last_login = now(), updated_at = now() WHERE id = $1",
		userID,
	)
	return err
}

// EnableMFA stores the encrypted TOTP secret and enables MFA.
func EnableMFA(db *sql.DB, userID, encryptedSecret string) error {
	_, err := db.Exec(`
		UPDATE users
		SET mfa_secret  = $1,
		    mfa_enabled = true,
		    updated_at  = now()
		WHERE id = $2`,
		encryptedSecret, userID,
	)
	return err
}

// ── API Key queries ───────────────────────────────────────────────────────────

// CreateAPIKey stores a new API key for a user.
func CreateAPIKey(db *sql.DB, userID, name, keyHash string) (*APIKey, error) {
	var k APIKey
	err := db.QueryRow(`
		INSERT INTO api_keys (user_id, name, key_hash)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, name, key_hash, last_used, expires_at, active, created_at`,
		userID, name, keyHash,
	).Scan(
		&k.ID, &k.UserID, &k.Name, &k.KeyHash,
		&k.LastUsed, &k.ExpiresAt, &k.Active, &k.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// GetUserByAPIKey looks up a user by their hashed API key.
func GetUserByAPIKey(db *sql.DB, keyHash string) (*User, error) {
	var u User
	err := db.QueryRow(`
		SELECT u.id, u.email, u.handle, u.password_hash, u.role,
		       u.mfa_secret, u.mfa_enabled, u.email_verified,
		       u.last_login, u.created_at, u.updated_at
		FROM users u
		JOIN api_keys k ON k.user_id = u.id
		WHERE k.key_hash = $1
		  AND k.active = true
		  AND (k.expires_at IS NULL OR k.expires_at > now())`,
		keyHash,
	).Scan(
		&u.ID, &u.Email, &u.Handle, &u.PasswordHash, &u.Role,
		&u.MFASecret, &u.MFAEnabled, &u.EmailVerified,
		&u.LastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Stamp last_used on the API key
	db.Exec(
		"UPDATE api_keys SET last_used = now() WHERE key_hash = $1",
		keyHash,
	)

	return &u, nil
}

// ListAPIKeys returns all active API keys for a user (hashes omitted).
func ListAPIKeys(db *sql.DB, userID string) ([]APIKey, error) {
	rows, err := db.Query(`
		SELECT id, user_id, name, key_hash, last_used, expires_at, active, created_at
		FROM api_keys
		WHERE user_id = $1 AND active = true
		ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(
			&k.ID, &k.UserID, &k.Name, &k.KeyHash,
			&k.LastUsed, &k.ExpiresAt, &k.Active, &k.CreatedAt,
		); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// RevokeAPIKey deactivates an API key by ID.
func RevokeAPIKey(db *sql.DB, keyID, userID string) error {
	_, err := db.Exec(
		"UPDATE api_keys SET active = false WHERE id = $1 AND user_id = $2",
		keyID, userID,
	)
	return err
}
