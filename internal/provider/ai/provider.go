// Package ai defines the interface for AI ticket analysis.
//
// Implementations:
//   - disabled: returns zero values, logs that AI is not configured
//   - openai:   calls OpenAI API (requires OPENAI_API_KEY)
//
// The provider is injected into the App struct at startup.
// The worker calls it. Handlers never call it directly.
// If the provider fails, the ticket still exists. AI is never blocking.
package ai

import "context"

// Provider is the interface every AI implementation must satisfy.
// Two methods. No more until a real use case demands it.
type Provider interface {
	// Classify analyzes a ticket and returns structured classification.
	// Called by the AI worker after a ticket is created.
	Classify(ctx context.Context, input ClassifyInput) (*ClassifyResult, error)

	// Draft generates a response for an agent to review.
	// Called after classification, when context has been gathered.
	Draft(ctx context.Context, input DraftInput) (*DraftResult, error)
}

// ── Classify ──────────────────────────────────────────────────────────────────

// ClassifyInput is what the AI receives for classification.
// No database types. No store structs. Pure data.
type ClassifyInput struct {
	TicketID string // for logging and tracing only
	Subject  string
	Body     string

	// Optional context — enriches classification accuracy
	CustomerName       string // who submitted it
	CustomerDepartment string // engineering, sales, etc.
	DeviceInfo         string // "MacBook Pro, macOS 14.4" from CMDB
	PriorTicketCount   int    // how many tickets has this customer submitted
}

// ClassifyResult is the structured output of classification.
type ClassifyResult struct {
	Category      string  // "hardware", "software", "access", "billing"
	Subcategory   string  // "laptop", "vpn", "password_reset"
	Priority      string  // "P0", "P1", "P2", "P3"
	Confidence    float64 // 0.0 to 1.0
	Sentiment     string  // "frustrated", "neutral", "positive"
	TimeSensitive bool
	TimeReference string  // "45 minutes", "end of day"
	IsPotentialP0 bool
	Summary       string  // one sentence
	Reasoning     string  // why the AI chose this classification
}

// ── Draft ─────────────────────────────────────────────────────────────────────

// DraftInput is what the AI receives for response drafting.
// Includes the classification and additional context from CMDB and KB.
type DraftInput struct {
	TicketID string
	Subject  string
	Body     string

	// Classification from prior Classify call
	Category    string
	Subcategory string
	Priority    string
	Sentiment   string
	Summary     string

	// Context retrieved from the system
	CustomerName     string
	DeviceInfo       string   // CMDB context
	PriorResolutions []string // summaries of similar resolved tickets
	KBArticles       []string // relevant knowledge base article bodies
}

// DraftResult is the AI-generated response for agent review.
type DraftResult struct {
	Body             string   // the draft reply text
	ResolutionSteps  []string // step-by-step instructions
	EstimatedMinutes int      // how long the fix should take
	Confidence       float64  // how confident the AI is in this draft
}
