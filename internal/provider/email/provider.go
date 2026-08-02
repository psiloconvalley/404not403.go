package email

import "context"

// Provider defines the contract for sending outbound communications.
type Provider interface {
	Send(ctx context.Context, input SendInput) error
}

// SendInput carries the payload for an outbound email.
type SendInput struct {
	To      string
	Subject string
	Body    string // HTML or Plain text depending on implementation
	From    string // Optional override
}
