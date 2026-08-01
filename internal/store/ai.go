package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/lib/pq"
)

// ── AIAnalysis ────────────────────────────────────────────────────────────────

// AIAnalysis stores the output of an AI classification or draft response.
// Separated from the ticket table because:
// 1. AI enrichment is async — ticket exists before AI runs
// 2. Multiple analyses can exist per ticket (re-analysis on new comments)
// 3. AI output must be auditable — model, version, confidence, reasoning
// 4. Learning loop fields track whether humans agreed with AI
type AIAnalysis struct {
	ID                string          `json:"id"`
	TicketID          string          `json:"ticket_id"`
	OrgID             string          `json:"org_id"`
	Model             string          `json:"model"`
	ModelVersion      *string         `json:"model_version,omitempty"`
	InputHash         string          `json:"input_hash"`
	Category          *string         `json:"category,omitempty"`
	Subcategory       *string         `json:"subcategory,omitempty"`
	SuggestedPriority *string         `json:"suggested_priority,omitempty"`
	Confidence        *float64        `json:"confidence,omitempty"`
	Sentiment         *string         `json:"sentiment,omitempty"`
	TimeSensitive     bool            `json:"time_sensitive"`
	TimeReference     *string         `json:"time_reference,omitempty"`
	IsPotentialP0     bool            `json:"is_potential_p0"`
	Reasoning         *string         `json:"reasoning,omitempty"`
	Entities          json.RawMessage `json:"entities"`
	Summary           *string         `json:"summary,omitempty"`
	DraftResponse     *string         `json:"draft_response,omitempty"`
	ResolutionSteps   []string        `json:"resolution_steps,omitempty"`
	EstimatedMinutes  *int            `json:"estimated_minutes,omitempty"`
	KBArticlesUsed    []string        `json:"kb_articles_used,omitempty"`
	HumanPriority     *string         `json:"human_priority,omitempty"`
	PriorityDelta     *int            `json:"priority_delta,omitempty"`
	DraftAccepted     *bool           `json:"draft_accepted,omitempty"`
	DraftEditDistance *int            `json:"draft_edit_distance,omitempty"`
	ResolutionMatched *bool           `json:"resolution_matched,omitempty"`
	RawOutput         json.RawMessage `json:"raw_output,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	ReviewedAt        *time.Time      `json:"reviewed_at,omitempty"`
}

// ── Create ────────────────────────────────────────────────────────────────────

// CreateAIAnalysisParams contains everything the AI worker produces.
type CreateAIAnalysisParams struct {
	TicketID          string
	OrgID             string
	Model             string
	ModelVersion      *string
	InputHash         string
	Category          *string
	Subcategory       *string
	SuggestedPriority *string
	Confidence        *float64
	Sentiment         *string
	TimeSensitive     bool
	TimeReference     *string
	IsPotentialP0     bool
	Reasoning         *string
	Entities          json.RawMessage
	Summary           *string
	DraftResponse     *string
	ResolutionSteps   []string
	EstimatedMinutes  *int
	KBArticlesUsed    []string
	RawOutput         json.RawMessage
}

// CreateAIAnalysis inserts an AI analysis result.
// Called by the AI worker after classification or draft generation.
func CreateAIAnalysis(db *sql.DB, p CreateAIAnalysisParams) (*AIAnalysis, error) {
	if p.Entities == nil {
		p.Entities = json.RawMessage(`[]`)
	}

	var a AIAnalysis
	err := db.QueryRow(`
		INSERT INTO ai_analysis (
			ticket_id, org_id, model, model_version, input_hash,
			category, subcategory, suggested_priority, confidence,
			sentiment, time_sensitive, time_reference,
			is_potential_p0, reasoning, entities, summary,
			draft_response, resolution_steps, estimated_minutes,
			kb_articles_used, raw_output
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12,
			$13, $14, $15, $16,
			$17, $18, $19,
			$20, $21
		)
		RETURNING id, ticket_id, org_id, model, model_version, input_hash,
		          category, subcategory, suggested_priority, confidence,
		          sentiment, time_sensitive, time_reference,
		          is_potential_p0, reasoning, entities, summary,
		          draft_response, resolution_steps, estimated_minutes,
		          kb_articles_used, human_priority, priority_delta,
		          draft_accepted, draft_edit_distance, resolution_matched,
		          raw_output, created_at, reviewed_at`,
		p.TicketID, p.OrgID, p.Model, p.ModelVersion, p.InputHash,
		p.Category, p.Subcategory, p.SuggestedPriority, p.Confidence,
		p.Sentiment, p.TimeSensitive, p.TimeReference,
		p.IsPotentialP0, p.Reasoning, p.Entities, p.Summary,
		p.DraftResponse, pq.Array(p.ResolutionSteps), p.EstimatedMinutes,
		pq.Array(p.KBArticlesUsed), p.RawOutput,
	).Scan(
		&a.ID, &a.TicketID, &a.OrgID, &a.Model, &a.ModelVersion, &a.InputHash,
		&a.Category, &a.Subcategory, &a.SuggestedPriority, &a.Confidence,
		&a.Sentiment, &a.TimeSensitive, &a.TimeReference,
		&a.IsPotentialP0, &a.Reasoning, &a.Entities, &a.Summary,
		&a.DraftResponse, pq.Array(&a.ResolutionSteps), &a.EstimatedMinutes,
		pq.Array(&a.KBArticlesUsed), &a.HumanPriority, &a.PriorityDelta,
		&a.DraftAccepted, &a.DraftEditDistance, &a.ResolutionMatched,
		&a.RawOutput, &a.CreatedAt, &a.ReviewedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ── Read ──────────────────────────────────────────────────────────────────────

// GetLatestAnalysis returns the most recent AI analysis for a ticket.
func GetLatestAnalysis(db *sql.DB, orgID, ticketID string) (*AIAnalysis, error) {
	var a AIAnalysis
	err := db.QueryRow(`
		SELECT id, ticket_id, org_id, model, model_version, input_hash,
		       category, subcategory, suggested_priority, confidence,
		       sentiment, time_sensitive, time_reference,
		       is_potential_p0, reasoning, entities, summary,
		       draft_response, resolution_steps, estimated_minutes,
		       kb_articles_used, human_priority, priority_delta,
		       draft_accepted, draft_edit_distance, resolution_matched,
		       raw_output, created_at, reviewed_at
		FROM ai_analysis
		WHERE org_id = $1 AND ticket_id = $2
		ORDER BY created_at DESC
		LIMIT 1`,
		orgID, ticketID,
	).Scan(
		&a.ID, &a.TicketID, &a.OrgID, &a.Model, &a.ModelVersion, &a.InputHash,
		&a.Category, &a.Subcategory, &a.SuggestedPriority, &a.Confidence,
		&a.Sentiment, &a.TimeSensitive, &a.TimeReference,
		&a.IsPotentialP0, &a.Reasoning, &a.Entities, &a.Summary,
		&a.DraftResponse, pq.Array(&a.ResolutionSteps), &a.EstimatedMinutes,
		pq.Array(&a.KBArticlesUsed), &a.HumanPriority, &a.PriorityDelta,
		&a.DraftAccepted, &a.DraftEditDistance, &a.ResolutionMatched,
		&a.RawOutput, &a.CreatedAt, &a.ReviewedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAnalysesByTicket returns all AI analyses for a ticket, newest first.
// Multiple analyses exist when a ticket is re-analyzed (e.g., after new comments).
func ListAnalysesByTicket(db *sql.DB, orgID, ticketID string) ([]AIAnalysis, error) {
	rows, err := db.Query(`
		SELECT id, ticket_id, org_id, model, model_version, input_hash,
		       category, subcategory, suggested_priority, confidence,
		       sentiment, time_sensitive, time_reference,
		       is_potential_p0, reasoning, entities, summary,
		       draft_response, resolution_steps, estimated_minutes,
		       kb_articles_used, human_priority, priority_delta,
		       draft_accepted, draft_edit_distance, resolution_matched,
		       raw_output, created_at, reviewed_at
		FROM ai_analysis
		WHERE org_id = $1 AND ticket_id = $2
		ORDER BY created_at DESC`,
		orgID, ticketID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var analyses []AIAnalysis
	for rows.Next() {
		var a AIAnalysis
		if err := rows.Scan(
			&a.ID, &a.TicketID, &a.OrgID, &a.Model, &a.ModelVersion, &a.InputHash,
			&a.Category, &a.Subcategory, &a.SuggestedPriority, &a.Confidence,
			&a.Sentiment, &a.TimeSensitive, &a.TimeReference,
			&a.IsPotentialP0, &a.Reasoning, &a.Entities, &a.Summary,
			&a.DraftResponse, pq.Array(&a.ResolutionSteps), &a.EstimatedMinutes,
			pq.Array(&a.KBArticlesUsed), &a.HumanPriority, &a.PriorityDelta,
			&a.DraftAccepted, &a.DraftEditDistance, &a.ResolutionMatched,
			&a.RawOutput, &a.CreatedAt, &a.ReviewedAt,
		); err != nil {
			return nil, err
		}
		analyses = append(analyses, a)
	}
	return analyses, rows.Err()
}

// ── Learning Loop ─────────────────────────────────────────────────────────────

// RecordHumanReview updates an AI analysis with what the human actually did.
// This is the learning loop — over time we know where AI is accurate.
func RecordHumanReview(db *sql.DB, analysisID string, humanPriority *string, priorityDelta *int, draftAccepted *bool, draftEditDistance *int, resolutionMatched *bool) error {
	_, err := db.Exec(`
		UPDATE ai_analysis
		SET human_priority = $1,
		    priority_delta = $2,
		    draft_accepted = $3,
		    draft_edit_distance = $4,
		    resolution_matched = $5,
		    reviewed_at = now()
		WHERE id = $6`,
		humanPriority, priorityDelta, draftAccepted,
		draftEditDistance, resolutionMatched, analysisID,
	)
	return err
}
