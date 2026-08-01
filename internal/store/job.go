package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// ── Job ───────────────────────────────────────────────────────────────────────

// Job represents a unit of asynchronous work.
// Jobs are durable — they survive process restarts.
// A Go worker pool reads pending jobs and processes them.
//
// This replaces in-memory channels for critical work:
// AI enrichment, email delivery, Slack posting, SLA checks.
//
// If the process dies mid-job, the job remains in the database.
// On restart, unclaimed jobs are picked up again.
type Job struct {
	ID             string          `json:"id"`
	OrgID          *string         `json:"org_id,omitempty"`
	JobType        string          `json:"job_type"`
	Payload        json.RawMessage `json:"payload"`
	Status         string          `json:"status"`
	Attempts       int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	LastError      *string         `json:"last_error,omitempty"`
	IdempotencyKey *string         `json:"idempotency_key,omitempty"`
	AvailableAt    time.Time       `json:"available_at"`
	ClaimedAt      *time.Time      `json:"claimed_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// Known job types. Prevents string drift.
const (
	JobTypeAIClassify     = "ai.classify"
	JobTypeAIDraft        = "ai.draft_response"
	JobTypeEmailSend      = "email.send"
	JobTypeSlackPost      = "slack.post"
	JobTypeSLACheck       = "sla.check"
	JobTypeWebhookDeliver = "webhook.deliver"
)

// Known job statuses.
const (
	JobStatusPending   = "pending"
	JobStatusClaimed   = "claimed"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusDead      = "dead" // exceeded max attempts
)

// ── Enqueue ───────────────────────────────────────────────────────────────────

// EnqueueJobParams contains everything needed to create a job.
type EnqueueJobParams struct {
	OrgID          *string
	JobType        string
	Payload        json.RawMessage
	IdempotencyKey *string
	MaxAttempts    int
	Delay          time.Duration // schedule for future execution
}

// EnqueueJob inserts a new job into the queue.
// If an idempotency key is provided and a job with that key already exists,
// this is a no-op — returns nil without error.
func EnqueueJob(db *sql.DB, p EnqueueJobParams) (*Job, error) {
	if p.Payload == nil {
		p.Payload = json.RawMessage(`{}`)
	}
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}

	availableAt := time.Now()
	if p.Delay > 0 {
		availableAt = availableAt.Add(p.Delay)
	}

	var j Job
	err := db.QueryRow(`
		INSERT INTO jobs (org_id, job_type, payload, max_attempts, idempotency_key, available_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, org_id, job_type, payload, status, attempts,
		          max_attempts, last_error, idempotency_key,
		          available_at, claimed_at, completed_at, created_at`,
		p.OrgID, p.JobType, p.Payload, p.MaxAttempts, p.IdempotencyKey, availableAt,
	).Scan(
		&j.ID, &j.OrgID, &j.JobType, &j.Payload, &j.Status,
		&j.Attempts, &j.MaxAttempts, &j.LastError, &j.IdempotencyKey,
		&j.AvailableAt, &j.ClaimedAt, &j.CompletedAt, &j.CreatedAt,
	)
	if err == sql.ErrNoRows {
		// ON CONFLICT DO NOTHING — idempotent duplicate
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// EnqueueJobTx inserts a job within an existing transaction.
// Used when a job must be created atomically with a ticket or event.
func EnqueueJobTx(tx *sql.Tx, p EnqueueJobParams) error {
	if p.Payload == nil {
		p.Payload = json.RawMessage(`{}`)
	}
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}

	availableAt := time.Now()
	if p.Delay > 0 {
		availableAt = availableAt.Add(p.Delay)
	}

	_, err := tx.Exec(`
		INSERT INTO jobs (org_id, job_type, payload, max_attempts, idempotency_key, available_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		p.OrgID, p.JobType, p.Payload, p.MaxAttempts, p.IdempotencyKey, availableAt,
	)
	return err
}

// ── Claim ─────────────────────────────────────────────────────────────────────

// ClaimJob atomically claims the next available job for processing.
// Uses SELECT FOR UPDATE SKIP LOCKED — multiple workers can run concurrently
// without claiming the same job.
// Returns nil if no jobs are available.
func ClaimJob(db *sql.DB, jobTypes ...string) (*Job, error) {
	if len(jobTypes) == 0 {
		return nil, nil
	}

	// Build IN clause
	query := `
		UPDATE jobs
		SET status = 'claimed',
		    claimed_at = now(),
		    attempts = attempts + 1
		WHERE id = (
			SELECT id FROM jobs
			WHERE status IN ('pending', 'failed')
			  AND available_at <= now()
			  AND attempts < max_attempts
			  AND job_type = ANY($1)
			ORDER BY available_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, org_id, job_type, payload, status, attempts,
		          max_attempts, last_error, idempotency_key,
		          available_at, claimed_at, completed_at, created_at`

	var j Job
	err := db.QueryRow(query, jobTypes).Scan(
		&j.ID, &j.OrgID, &j.JobType, &j.Payload, &j.Status,
		&j.Attempts, &j.MaxAttempts, &j.LastError, &j.IdempotencyKey,
		&j.AvailableAt, &j.ClaimedAt, &j.CompletedAt, &j.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// ── Complete ──────────────────────────────────────────────────────────────────

// CompleteJob marks a job as successfully completed.
func CompleteJob(db *sql.DB, jobID string) error {
	_, err := db.Exec(`
		UPDATE jobs
		SET status = 'completed', completed_at = now()
		WHERE id = $1`,
		jobID,
	)
	return err
}

// FailJob marks a job as failed with an error message.
// If attempts < max_attempts, the job returns to the queue
// with exponential backoff.
func FailJob(db *sql.DB, jobID, errMsg string) error {
	_, err := db.Exec(`
		UPDATE jobs
		SET status = CASE
		        WHEN attempts >= max_attempts THEN 'dead'
		        ELSE 'failed'
		    END,
		    last_error = $1,
		    available_at = CASE
		        WHEN attempts >= max_attempts THEN available_at
		        ELSE now() + (interval '1 second' * power(2, attempts))
		    END
		WHERE id = $2`,
		errMsg, jobID,
	)
	return err
}

// ── Queries ───────────────────────────────────────────────────────────────────

// CountPendingJobs returns the number of jobs waiting to be processed.
// Useful for health checks and dashboard metrics.
func CountPendingJobs(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM jobs
		WHERE status IN ('pending', 'failed')
		  AND available_at <= now()
		  AND attempts < max_attempts`,
	).Scan(&count)
	return count, err
}

// CountDeadJobs returns the number of jobs that exhausted all retry attempts.
// A non-zero count is an operational alert.
func CountDeadJobs(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM jobs WHERE status = 'dead'",
	).Scan(&count)
	return count, err
}
