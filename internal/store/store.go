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

// ── RunMigrations ─────────────────────────────────────────────────────────────
// Migrations are append-only. Never edit a migration that has run.
// Add new migrations at the end only.

func RunMigrations(db *sql.DB) {
	if db == nil {
		return
	}

	migrations := []struct {
		name string
		sql  string
	}{
		// ── Users ────────────────────────────────────────────────────────────
		{
			name: "create_users_table",
			sql: `CREATE TABLE IF NOT EXISTS users (
				id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				email          TEXT NOT NULL UNIQUE,
				handle         TEXT NOT NULL UNIQUE,
				password_hash  TEXT NOT NULL,
				role           TEXT NOT NULL DEFAULT 'agent',
				mfa_secret     TEXT,
				mfa_enabled    BOOLEAN NOT NULL DEFAULT false,
				email_verified BOOLEAN NOT NULL DEFAULT false,
				last_login     TIMESTAMPTZ,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
				updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
		},
		{name: "idx_users_email",  sql: `CREATE INDEX IF NOT EXISTS idx_users_email  ON users(email)`},
		{name: "idx_users_handle", sql: `CREATE INDEX IF NOT EXISTS idx_users_handle ON users(handle)`},

		// ── API Keys ─────────────────────────────────────────────────────────
		{
			name: "create_api_keys_table",
			sql: `CREATE TABLE IF NOT EXISTS api_keys (
				id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				user_id    UUID NOT NULL REFERENCES users(id),
				name       TEXT NOT NULL,
				key_hash   TEXT NOT NULL UNIQUE,
				last_used  TIMESTAMPTZ,
				expires_at TIMESTAMPTZ,
				active     BOOLEAN NOT NULL DEFAULT true,
				created_at TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
		},
		{name: "idx_api_keys_user", sql: `CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id)`},
		{name: "idx_api_keys_hash", sql: `CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash)`},

		// ── Password Resets ───────────────────────────────────────────────────
		{
			name: "create_password_resets_table",
			sql: `CREATE TABLE IF NOT EXISTS password_resets (
				id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				user_id    UUID NOT NULL REFERENCES users(id),
				token_hash TEXT NOT NULL UNIQUE,
				expires_at TIMESTAMPTZ NOT NULL,
				used       BOOLEAN NOT NULL DEFAULT false,
				created_at TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
		},
		{name: "idx_resets_token", sql: `CREATE INDEX IF NOT EXISTS idx_resets_token ON password_resets(token_hash)`},
		{name: "idx_resets_user",  sql: `CREATE INDEX IF NOT EXISTS idx_resets_user  ON password_resets(user_id)`},

		// ── Organizations ─────────────────────────────────────────────────────
		{
			name: "create_organizations_table",
			sql: `CREATE TABLE IF NOT EXISTS organizations (
				id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				name                 TEXT NOT NULL,
				slug                 TEXT NOT NULL UNIQUE,
				domain               TEXT,
				plan                 TEXT NOT NULL DEFAULT 'starter',
				inbound_email        TEXT UNIQUE,
				slack_team_id        TEXT UNIQUE,
				slack_channel_id     TEXT,
				created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
				updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
		},
		{name: "idx_orgs_slug", sql: `CREATE INDEX IF NOT EXISTS idx_orgs_slug ON organizations(slug)`},

		// ── Org Members ───────────────────────────────────────────────────────
		{
			name: "create_org_members_table",
			sql: `CREATE TABLE IF NOT EXISTS org_members (
				org_id     UUID NOT NULL REFERENCES organizations(id),
				user_id    UUID NOT NULL REFERENCES users(id),
				role       TEXT NOT NULL DEFAULT 'agent',
				created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				PRIMARY KEY (org_id, user_id)
			)`,
		},
		{name: "idx_org_members_user", sql: `CREATE INDEX IF NOT EXISTS idx_org_members_user ON org_members(user_id)`},

		// ── Customers ─────────────────────────────────────────────────────────
		{
			name: "create_customers_table",
			sql: `CREATE TABLE IF NOT EXISTS customers (
				id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				org_id        UUID NOT NULL REFERENCES organizations(id),
				email         TEXT,
				slack_user_id TEXT,
				full_name     TEXT,
				department    TEXT,
				created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
				updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
				UNIQUE (org_id, email),
				UNIQUE (org_id, slack_user_id)
			)`,
		},
		{name: "idx_customers_org",   sql: `CREATE INDEX IF NOT EXISTS idx_customers_org   ON customers(org_id)`},
		{name: "idx_customers_email", sql: `CREATE INDEX IF NOT EXISTS idx_customers_email ON customers(org_id, email)`},

		// ── Tickets ───────────────────────────────────────────────────────────
		{
			name: "create_tickets_table",
			sql: `CREATE TABLE IF NOT EXISTS tickets (
				id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				org_id      UUID NOT NULL REFERENCES organizations(id),
				customer_id UUID REFERENCES customers(id),
				assigned_to UUID REFERENCES users(id),
				subject     TEXT NOT NULL,
				body        TEXT NOT NULL,
				status      TEXT NOT NULL DEFAULT 'open',
				priority    TEXT NOT NULL DEFAULT 'P2',
				category    TEXT,
				source_type TEXT NOT NULL,
				thread_id   TEXT UNIQUE,
				incident_id UUID,
				sla_due_at  TIMESTAMPTZ,
				sla_breached BOOLEAN NOT NULL DEFAULT false,
				created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
				updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
				resolved_at TIMESTAMPTZ
			)`,
		},
		{name: "idx_tickets_org",      sql: `CREATE INDEX IF NOT EXISTS idx_tickets_org      ON tickets(org_id)`},
		{name: "idx_tickets_status",   sql: `CREATE INDEX IF NOT EXISTS idx_tickets_status   ON tickets(org_id, status)`},
		{name: "idx_tickets_assigned", sql: `CREATE INDEX IF NOT EXISTS idx_tickets_assigned ON tickets(assigned_to)`},
		{name: "idx_tickets_priority", sql: `CREATE INDEX IF NOT EXISTS idx_tickets_priority ON tickets(org_id, priority)`},
		{
			name: "tickets_search_vector",
			sql: `ALTER TABLE tickets ADD COLUMN IF NOT EXISTS
				search_vector tsvector
				GENERATED ALWAYS AS (
					to_tsvector('english',
						coalesce(subject, '') || ' ' || coalesce(body, '')
					)
				) STORED`,
		},
		{name: "idx_tickets_search", sql: `CREATE INDEX IF NOT EXISTS idx_tickets_search ON tickets USING GIN(search_vector)`},

		// ── Comments ──────────────────────────────────────────────────────────
		{
			name: "create_comments_table",
			sql: `CREATE TABLE IF NOT EXISTS comments (
				id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ticket_id   UUID NOT NULL REFERENCES tickets(id),
				author_id   UUID REFERENCES users(id),
				customer_id UUID REFERENCES customers(id),
				body        TEXT NOT NULL,
				is_internal BOOLEAN NOT NULL DEFAULT false,
				source_type TEXT NOT NULL DEFAULT 'app',
				external_id TEXT UNIQUE,
				ai_drafted  BOOLEAN NOT NULL DEFAULT false,
				ai_accepted BOOLEAN,
				created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
		},
		{name: "idx_comments_ticket", sql: `CREATE INDEX IF NOT EXISTS idx_comments_ticket ON comments(ticket_id)`},

		// ── Ticket Events (append-only audit log) ─────────────────────────────
		{
			name: "create_ticket_events_table",
			sql: `CREATE TABLE IF NOT EXISTS ticket_events (
				id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ticket_id      UUID NOT NULL REFERENCES tickets(id),
				org_id         UUID NOT NULL REFERENCES organizations(id),
				actor_user_id  UUID REFERENCES users(id),
				actor_type     TEXT NOT NULL DEFAULT 'user',
				event_type     TEXT NOT NULL,
				payload        JSONB NOT NULL DEFAULT '{}',
				created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
		},
		{name: "idx_events_ticket", sql: `CREATE INDEX IF NOT EXISTS idx_events_ticket ON ticket_events(ticket_id)`},
		{name: "idx_events_org",    sql: `CREATE INDEX IF NOT EXISTS idx_events_org    ON ticket_events(org_id)`},

		// ── Config Items (CMDB) ───────────────────────────────────────────────
		{
			name: "create_config_items_table",
			sql: `CREATE TABLE IF NOT EXISTS config_items (
				id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				org_id        UUID NOT NULL REFERENCES organizations(id),
				ci_type       TEXT NOT NULL,
				name          TEXT NOT NULL,
				asset_tag     TEXT,
				serial_number TEXT,
				assigned_to   UUID REFERENCES customers(id),
				managed_by    UUID REFERENCES users(id),
				status        TEXT NOT NULL DEFAULT 'active',
				metadata      JSONB NOT NULL DEFAULT '{}',
				purchased_at  TIMESTAMPTZ,
				warranty_ends TIMESTAMPTZ,
				created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
				updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
		},
		{name: "idx_ci_org",    sql: `CREATE INDEX IF NOT EXISTS idx_ci_org    ON config_items(org_id)`},
		{name: "idx_ci_type",   sql: `CREATE INDEX IF NOT EXISTS idx_ci_type   ON config_items(org_id, ci_type)`},
		{name: "idx_ci_owner",  sql: `CREATE INDEX IF NOT EXISTS idx_ci_owner  ON config_items(assigned_to)`},

		// ── Ticket ↔ Config Item (many-to-many) ───────────────────────────────
		{
			name: "create_ticket_config_items_table",
			sql: `CREATE TABLE IF NOT EXISTS ticket_config_items (
				ticket_id UUID NOT NULL REFERENCES tickets(id),
				ci_id     UUID NOT NULL REFERENCES config_items(id),
				PRIMARY KEY (ticket_id, ci_id)
			)`,
		},

		// ── Inbound Messages (raw signal — never lose the original) ────────────
		{
			name: "create_inbound_messages_table",
			sql: `CREATE TABLE IF NOT EXISTS inbound_messages (
				id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				org_id       UUID REFERENCES organizations(id),
				source_type  TEXT NOT NULL,
				external_id  TEXT NOT NULL UNIQUE,
				raw_payload  JSONB NOT NULL,
				processed    BOOLEAN NOT NULL DEFAULT false,
				ticket_id    UUID REFERENCES tickets(id),
				received_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
				processed_at TIMESTAMPTZ
			)`,
		},
		{name: "idx_inbound_processed", sql: `CREATE INDEX IF NOT EXISTS idx_inbound_processed ON inbound_messages(processed) WHERE processed = false`},
		{name: "idx_inbound_org",       sql: `CREATE INDEX IF NOT EXISTS idx_inbound_org       ON inbound_messages(org_id)`},

		// ── Job Queue (durable async work) ────────────────────────────────────
		{
			name: "create_jobs_table",
			sql: `CREATE TABLE IF NOT EXISTS jobs (
				id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				org_id          UUID REFERENCES organizations(id),
				job_type        TEXT NOT NULL,
				payload         JSONB NOT NULL DEFAULT '{}',
				status          TEXT NOT NULL DEFAULT 'pending',
				attempts        INT NOT NULL DEFAULT 0,
				max_attempts    INT NOT NULL DEFAULT 3,
				last_error      TEXT,
				idempotency_key TEXT UNIQUE,
				available_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
				claimed_at      TIMESTAMPTZ,
				completed_at    TIMESTAMPTZ,
				created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
		},
		{name: "idx_jobs_pending", sql: `CREATE INDEX IF NOT EXISTS idx_jobs_pending ON jobs(status, available_at) WHERE status IN ('pending', 'failed')`},
		{name: "idx_jobs_org",     sql: `CREATE INDEX IF NOT EXISTS idx_jobs_org     ON jobs(org_id)`},

		// ── AI Analysis ───────────────────────────────────────────────────────
		{
			name: "create_ai_analysis_table",
			sql: `CREATE TABLE IF NOT EXISTS ai_analysis (
				id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ticket_id            UUID NOT NULL REFERENCES tickets(id),
				org_id               UUID NOT NULL REFERENCES organizations(id),
				model                TEXT NOT NULL,
				model_version        TEXT,
				input_hash           TEXT NOT NULL,
				category             TEXT,
				subcategory          TEXT,
				suggested_priority   TEXT,
				confidence           FLOAT,
				sentiment            TEXT,
				time_sensitive       BOOLEAN NOT NULL DEFAULT false,
				time_reference       TEXT,
				is_potential_p0      BOOLEAN NOT NULL DEFAULT false,
				reasoning            TEXT,
				entities             JSONB NOT NULL DEFAULT '[]',
				summary              TEXT,
				draft_response       TEXT,
				resolution_steps     TEXT[],
				estimated_minutes    INT,
				kb_articles_used     UUID[],
				human_priority       TEXT,
				priority_delta       INT,
				draft_accepted       BOOLEAN,
				draft_edit_distance  INT,
				resolution_matched   BOOLEAN,
				raw_output           JSONB,
				created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
				reviewed_at          TIMESTAMPTZ
			)`,
		},
		{name: "idx_ai_ticket", sql: `CREATE INDEX IF NOT EXISTS idx_ai_ticket ON ai_analysis(ticket_id)`},
		{name: "idx_ai_org",    sql: `CREATE INDEX IF NOT EXISTS idx_ai_org    ON ai_analysis(org_id)`},

		// ── Customer Tracking Tokens ──────────────────────────────────────────
		{
			name: "add_tracking_token_to_customers",
			sql: `ALTER TABLE customers
				ADD COLUMN IF NOT EXISTS tracking_token_hash TEXT UNIQUE,
				ADD COLUMN IF NOT EXISTS tracking_token_expires TIMESTAMPTZ`,
		},
		{name: "idx_customers_token", sql: `CREATE INDEX IF NOT EXISTS idx_customers_token ON customers(tracking_token_hash) WHERE tracking_token_hash IS NOT NULL`},

		// ── Auth Events ───────────────────────────────────────────────────────
		{
			name: "create_auth_events_table",
			sql: `CREATE TABLE IF NOT EXISTS auth_events (
				id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				user_id        UUID REFERENCES users(id),
				org_id         UUID REFERENCES organizations(id),
				event_type     TEXT NOT NULL,
				provider       TEXT NOT NULL DEFAULT 'email',
				ip_address     TEXT,
				user_agent     TEXT,
				success        BOOLEAN NOT NULL,
				failure_reason TEXT,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
		},
		{name: "idx_auth_events_user", sql: `CREATE INDEX IF NOT EXISTS idx_auth_events_user ON auth_events(user_id)`},

		// ── Account Lockout ───────────────────────────────────────────────────
		{
			name: "add_lockout_to_users",
			sql: `ALTER TABLE users
				ADD COLUMN IF NOT EXISTS failed_login_attempts INT NOT NULL DEFAULT 0,
				ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ`,
		},
	}

	for _, m := range migrations {
		if _, err := db.Exec(m.sql); err != nil {
			log.Fatalf("❌ Migration failed [%s]: %v", m.name, err)
		}
		log.Printf("✅ Migration OK: %s", m.name)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func nullableInt(n int) interface{} {
	if n == 0 {
		return nil
	}
	return n
}
