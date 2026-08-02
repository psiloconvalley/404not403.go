// Package worker implements the background job processor.
//
// The worker runs as goroutines started from main.go.
// It claims jobs from the jobs table using SELECT FOR UPDATE SKIP LOCKED.
// Multiple workers can run safely — no race conditions, no duplicate processing.
//
// The worker processes:
//   - ai.classify     — classify a ticket via the AI provider
//   - ai.draft_response — draft a response via the AI provider
//
// If the AI provider is disabled, jobs complete immediately with empty results.
// If the AI provider fails, jobs are retried with exponential backoff.
// If a job exceeds max_attempts, it is marked dead for manual review.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/provider/ai"
	"github.com/psiloconvalley/404not403/internal/provider/email"
	"github.com/psiloconvalley/404not403/internal/store"
)

// Start launches the job worker. It polls for pending jobs and processes them.
// Respects the context — exits cleanly on shutdown signal.
// Call this with `go worker.Start(ctx, a)` from main.go.
func Start(ctx context.Context, a *app.App) {
	log.Println("✅ Worker started — polling for jobs.")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("⏳ Worker shutting down.")
			return
		case <-ticker.C:
			processNext(ctx, a)
		}
	}
}

// processNext claims and processes a single job.
// Called on every tick. Returns immediately if no jobs are available.
func processNext(ctx context.Context, a *app.App) {
	job, err := store.ClaimJob(a.DB, store.JobTypeAIClassify, store.JobTypeAIDraft, store.JobTypeEmailSend)
	if err != nil {
		log.Printf("⚠️  Worker: claim error: %v", err)
		return
	}
	if job == nil {
		return // no jobs available
	}

	log.Printf("🔧 Worker: processing job %s (type: %s, attempt: %d/%d)",
		job.ID, job.JobType, job.Attempts, job.MaxAttempts)

	var processErr error
	switch job.JobType {
	case store.JobTypeAIClassify:
		processErr = handleClassify(ctx, a, job)
	case store.JobTypeAIDraft:
		processErr = handleDraft(ctx, a, job)
	case store.JobTypeEmailSend:
		processErr = handleEmailSend(ctx, a, job)
	default:
		processErr = fmt.Errorf("unknown job type: %s", job.JobType)
	}

	if processErr != nil {
		log.Printf("⚠️  Worker: job %s failed: %v", job.ID, processErr)
		if err := store.FailJob(a.DB, job.ID, processErr.Error()); err != nil {
			log.Printf("⚠️  Worker: failed to mark job %s as failed: %v", job.ID, err)
		}
		return
	}

	if err := store.CompleteJob(a.DB, job.ID); err != nil {
		log.Printf("⚠️  Worker: failed to mark job %s as completed: %v", job.ID, err)
		return
	}

	log.Printf("✅ Worker: job %s completed", job.ID)
}

// ── AI Classify Handler ───────────────────────────────────────────────────────

func handleClassify(ctx context.Context, a *app.App, job *store.Job) error {
	// Parse job payload
	var payload struct {
		TicketID string `json:"ticket_id"`
		OrgID    string `json:"org_id"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("parse payload: %w", err)
	}
	if payload.TicketID == "" || payload.OrgID == "" {
		return fmt.Errorf("payload missing ticket_id or org_id")
	}

	// Load the ticket
	ticket, err := store.GetTicketByID(a.DB, payload.OrgID, payload.TicketID)
	if err != nil {
		return fmt.Errorf("load ticket: %w", err)
	}
	if ticket == nil {
		return fmt.Errorf("ticket %s not found", payload.TicketID)
	}

	// Build AI input with available context
	classifyInput := ai.ClassifyInput{
		TicketID: ticket.ID,
		Subject:  ticket.Subject,
		Body:     ticket.Body,
	}

	// Enrich with customer context if available
	if ticket.CustomerID != nil {
		customer, err := store.GetCustomerByID(a.DB, payload.OrgID, *ticket.CustomerID)
		if err == nil && customer != nil {
			if customer.FullName != nil {
				classifyInput.CustomerName = *customer.FullName
			}
			if customer.Department != nil {
				classifyInput.CustomerDepartment = *customer.Department
			}

			// Load device context from CMDB
			devices, err := store.GetConfigItemsByCustomer(a.DB, payload.OrgID, customer.ID)
			if err == nil && len(devices) > 0 {
				classifyInput.DeviceInfo = formatDeviceInfo(devices)
			}
		}
	}

	// Call AI provider
	result, err := a.AI.Classify(ctx, classifyInput)
	if err != nil {
		return fmt.Errorf("AI classify: %w", err)
	}

	// Input hash for deduplication
	inputHash := fmt.Sprintf("%x", inputHashBytes(ticket.Subject, ticket.Body))

	// Store AI analysis
	model := "disabled"
	_, err = store.CreateAIAnalysis(a.DB, store.CreateAIAnalysisParams{
		TicketID:          ticket.ID,
		OrgID:             payload.OrgID,
		Model:             model,
		InputHash:         inputHash,
		Category:          nilIfEmpty(result.Category),
		Subcategory:       nilIfEmpty(result.Subcategory),
		SuggestedPriority: nilIfEmpty(result.Priority),
		Confidence:        nilIfZero(result.Confidence),
		Sentiment:         nilIfEmpty(result.Sentiment),
		TimeSensitive:     result.TimeSensitive,
		TimeReference:     nilIfEmpty(result.TimeReference),
		IsPotentialP0:     result.IsPotentialP0,
		Reasoning:         nilIfEmpty(result.Reasoning),
		Summary:           nilIfEmpty(result.Summary),
	})
	if err != nil {
		return fmt.Errorf("store analysis: %w", err)
	}

	// If AI is confident enough, update ticket category and priority
	if result.Confidence >= 0.8 && result.Category != "" {
		_ = store.UpdateTicketCategory(a.DB, payload.OrgID, ticket.ID, result.Category)
	}

	if result.Confidence >= 0.9 && result.Priority != "" {
		if _, err := domain.ParsePriority(result.Priority); err == nil {
			// Record as AI actor, not user
			_ = store.RecordEvent(a.DB, payload.OrgID, ticket.ID, nil,
				string(domain.ActorAI),
				string(domain.EventAIClassified),
				store.EventPayload(
					"category", result.Category,
					"priority", result.Priority,
					"confidence", fmt.Sprintf("%.2f", result.Confidence),
				),
			)
		}
	}

	return nil
}

// ── AI Draft Handler ──────────────────────────────────────────────────────────

func handleDraft(ctx context.Context, a *app.App, job *store.Job) error {
	// Parse job payload
	var payload struct {
		TicketID string `json:"ticket_id"`
		OrgID    string `json:"org_id"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("parse payload: %w", err)
	}
	if payload.TicketID == "" || payload.OrgID == "" {
		return fmt.Errorf("payload missing ticket_id or org_id")
	}

	// Load ticket
	ticket, err := store.GetTicketByID(a.DB, payload.OrgID, payload.TicketID)
	if err != nil {
		return fmt.Errorf("load ticket: %w", err)
	}
	if ticket == nil {
		return fmt.Errorf("ticket %s not found", payload.TicketID)
	}

	// Load existing analysis for context
	analysis, err := store.GetLatestAnalysis(a.DB, payload.OrgID, payload.TicketID)
	if err != nil {
		return fmt.Errorf("load analysis: %w", err)
	}

	// Build draft input
	draftInput := ai.DraftInput{
		TicketID: ticket.ID,
		Subject:  ticket.Subject,
		Body:     ticket.Body,
	}

	if analysis != nil {
		if analysis.Category != nil {
			draftInput.Category = *analysis.Category
		}
		if analysis.Subcategory != nil {
			draftInput.Subcategory = *analysis.Subcategory
		}
		if analysis.SuggestedPriority != nil {
			draftInput.Priority = *analysis.SuggestedPriority
		}
		if analysis.Sentiment != nil {
			draftInput.Sentiment = *analysis.Sentiment
		}
		if analysis.Summary != nil {
			draftInput.Summary = *analysis.Summary
		}
	}

	// Call AI provider
	result, err := a.AI.Draft(ctx, draftInput)
	if err != nil {
		return fmt.Errorf("AI draft: %w", err)
	}

	// Store draft result as an AI analysis update
	inputHash := fmt.Sprintf("%x", inputHashBytes(ticket.Subject, ticket.Body))
	model := "disabled"

	_, err = store.CreateAIAnalysis(a.DB, store.CreateAIAnalysisParams{
		TicketID:         ticket.ID,
		OrgID:            payload.OrgID,
		Model:            model,
		InputHash:        inputHash,
		DraftResponse:    nilIfEmpty(result.Body),
		ResolutionSteps:  result.ResolutionSteps,
		EstimatedMinutes: nilIfZeroInt(result.EstimatedMinutes),
		Confidence:       nilIfZero(result.Confidence),
	})
	if err != nil {
		return fmt.Errorf("store draft: %w", err)
	}

	// Record event
	_ = store.RecordEvent(a.DB, payload.OrgID, ticket.ID, nil,
		string(domain.ActorAI),
		string(domain.EventAIDraftGenerated),
		store.EventPayload("confidence", fmt.Sprintf("%.2f", result.Confidence)),
	)

	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func formatDeviceInfo(devices []store.ConfigItem) string {
	if len(devices) == 0 {
		return ""
	}
	// Summarize first 3 devices
	var parts []string
	limit := 3
	if len(devices) < limit {
		limit = len(devices)
	}
	for _, d := range devices[:limit] {
		parts = append(parts, fmt.Sprintf("%s (%s)", d.Name, d.CIType))
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}

func inputHashBytes(subject, body string) []byte {
	combined := subject + "\n" + body
	h := make([]byte, 0, 32)
	for _, b := range []byte(combined) {
		h = append(h, b)
	}
	return h
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nilIfZero(f float64) *float64 {
	if f == 0 {
		return nil
	}
	return &f
}

func nilIfZeroInt(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

// ── Email Send Handler ───────────────────────────────────────────────────────

// handleEmailSend delivers an outbound email job via the configured provider.
func handleEmailSend(ctx context.Context, a *app.App, job *store.Job) error {
	var payload struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
		From    string `json:"from,omitempty"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("parse payload: %w", err)
	}
	if payload.To == "" || payload.Subject == "" || payload.Body == "" {
		return fmt.Errorf("email job missing to, subject, or body")
	}
	if a.Email == nil {
		return fmt.Errorf("email provider not configured")
	}
	return a.Email.Send(ctx, email.SendInput{
		To:      payload.To,
		Subject: payload.Subject,
		Body:    payload.Body,
		From:    payload.From,
	})
}
