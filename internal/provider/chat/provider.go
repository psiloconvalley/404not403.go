package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/psiloconvalley/404not403/internal/domain"
)

// ChatMessage is the normalized carrier representing an inbound message from any chat network.
type ChatMessage struct {
	Provider        domain.ChatProviderName `json:"provider"`
	WorkspaceID     string                  `json:"workspace_id"`
	ChannelID       string                  `json:"channel_id"`
	ThreadID        string                  `json:"thread_id"`
	ProviderUserID  string                  `json:"provider_user_id"`
	Body            string                  `json:"body"`
	IsDirectMessage bool                    `json:"is_direct_message"`
	IsMention       bool                    `json:"is_mention"`
	ReceivedAt      time.Time               `json:"received_at"`
	RawPayload      json.RawMessage         `json:"raw_payload"`
}

// ChatTarget identifies the destination for an outbound chat reply.
type ChatTarget struct {
	Provider        domain.ChatProviderName `json:"provider"`
	WorkspaceID     string                  `json:"workspace_id"`
	ChannelID       string                  `json:"channel_id"`
	ThreadID        string                  `json:"thread_id"`
	ProviderContext json.RawMessage         `json:"provider_context,omitempty"`
}

// ChatUser is the identity profile resolved from an external chat network.
type ChatUser struct {
	ProviderUserID string  `json:"provider_user_id"`
	Email          *string `json:"email,omitempty"`
	DisplayName    string  `json:"display_name"`
	Handle         string  `json:"handle"`
}

// ChatProvider represents any external chat system (Slack, Teams, Discord, IRC, etc.).
type ChatProvider interface {
	Name() domain.ChatProviderName
	VerifyWebhook(r *http.Request, signingSecret string, bodyBytes []byte) error
	ParseInbound(bodyBytes []byte) (*ChatMessage, error)
	SendReply(ctx context.Context, botToken string, target ChatTarget, body string) error
	ResolveUser(ctx context.Context, botToken string, providerUserID string) (*ChatUser, error)
}

// ── Errors ────────────────────────────────────────────────────────────────────

type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("chat provider retryable error: %v", e.Err)
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("chat provider permanent error: %v", e.Err)
}

func (e *PermanentError) Unwrap() error {
	return e.Err
}
