package ai

import (
	"context"
	"log"
)

// Disabled is the AI provider used when no API key is configured.
// It returns empty results without error.
// The system operates normally — tickets are created, assigned, resolved.
// They just don't get AI enrichment until a real provider is configured.
type Disabled struct{}

// NewDisabled returns a disabled AI provider.
func NewDisabled() *Disabled {
	return &Disabled{}
}

// Classify returns an empty result. Logs once that AI is not configured.
func (d *Disabled) Classify(_ context.Context, input ClassifyInput) (*ClassifyResult, error) {
	log.Printf("ℹ️  AI disabled — skipping classification for ticket %s", input.TicketID)
	return &ClassifyResult{}, nil
}

// Draft returns an empty result. Logs once that AI is not configured.
func (d *Disabled) Draft(_ context.Context, input DraftInput) (*DraftResult, error) {
	log.Printf("ℹ️  AI disabled — skipping draft for ticket %s", input.TicketID)
	return &DraftResult{}, nil
}
