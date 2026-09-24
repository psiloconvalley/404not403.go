package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/psiloconvalley/404not403/internal/domain"
)

// ChatWorkspace represents a tenant integration with an external chat platform.
type ChatWorkspace struct {
	ID                string                  `json:"id"`
	OrgID             string                  `json:"org_id"`
	Provider          domain.ChatProviderName `json:"provider"`
	WorkspaceID       string                  `json:"workspace_id"`
	WorkspaceName     *string                 `json:"workspace_name,omitempty"`
	BotToken          string                  `json:"bot_token"`
	BotUserID         *string                 `json:"bot_user_id,omitempty"`
	InstalledByUserID *string                 `json:"installed_by_user_id,omitempty"`
	InstalledAt       time.Time               `json:"installed_at"`
	RevokedAt         *time.Time              `json:"revoked_at,omitempty"`
}

// TicketChannelContext links a ticket to its origin thread on a chat network.
type TicketChannelContext struct {
	TicketID       string                  `json:"ticket_id"`
	OrgID          string                  `json:"org_id"`
	Provider       domain.ChatProviderName `json:"provider"`
	WorkspaceID    string                  `json:"workspace_id"`
	ChannelID      string                  `json:"channel_id"`
	ThreadID       string                  `json:"thread_id"`
	ProviderUserID string                  `json:"provider_user_id"`
	RawContext     json.RawMessage         `json:"raw_context"`
	CreatedAt      time.Time               `json:"created_at"`
}

// ── Workspace Queries ─────────────────────────────────────────────────────────

// GetChatWorkspaceByWorkspaceID resolves an external workspace ID to its tenant record.
func GetChatWorkspaceByWorkspaceID(db *sql.DB, provider domain.ChatProviderName, workspaceID string) (*ChatWorkspace, error) {
	var w ChatWorkspace
	var prov string
	err := db.QueryRow(`
		SELECT id, org_id, provider, workspace_id, workspace_name,
		       bot_token, bot_user_id, installed_by_user_id, installed_at, revoked_at
		FROM chat_workspaces
		WHERE provider = $1 AND workspace_id = $2 AND revoked_at IS NULL
		LIMIT 1`,
		string(provider), workspaceID,
	).Scan(
		&w.ID, &w.OrgID, &prov, &w.WorkspaceID, &w.WorkspaceName,
		&w.BotToken, &w.BotUserID, &w.InstalledByUserID, &w.InstalledAt, &w.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	w.Provider = domain.ChatProviderName(prov)
	return &w, nil
}

// UpsertChatWorkspace installs or updates a chat workspace record.
func UpsertChatWorkspace(db *sql.DB, w ChatWorkspace) error {
	_, err := db.Exec(`
		INSERT INTO chat_workspaces (
			org_id, provider, workspace_id, workspace_name,
			bot_token, bot_user_id, installed_by_user_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (provider, workspace_id) DO UPDATE SET
			org_id = EXCLUDED.org_id,
			workspace_name = EXCLUDED.workspace_name,
			bot_token = EXCLUDED.bot_token,
			bot_user_id = EXCLUDED.bot_user_id,
			revoked_at = NULL`,
		w.OrgID, string(w.Provider), w.WorkspaceID, w.WorkspaceName,
		w.BotToken, w.BotUserID, w.InstalledByUserID,
	)
	return err
}

// ── Ticket Channel Context Queries ────────────────────────────────────────────

// GetTicketIDByChannelThread looks up if an existing ticket exists for a chat thread.
func GetTicketIDByChannelThread(db *sql.DB, provider domain.ChatProviderName, workspaceID, channelID, threadID string) (ticketID string, orgID string, err error) {
	err = db.QueryRow(`
		SELECT ticket_id, org_id
		FROM ticket_channel_context
		WHERE provider = $1 AND workspace_id = $2 AND channel_id = $3 AND thread_id = $4
		LIMIT 1`,
		string(provider), workspaceID, channelID, threadID,
	).Scan(&ticketID, &orgID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}
	return ticketID, orgID, nil
}

// CreateTicketChannelContextTx records the origin thread context within a transaction.
func CreateTicketChannelContextTx(tx *sql.Tx, ctx TicketChannelContext) error {
	if len(ctx.RawContext) == 0 {
		ctx.RawContext = json.RawMessage("{}")
	}
	_, err := tx.Exec(`
		INSERT INTO ticket_channel_context (
			ticket_id, org_id, provider, workspace_id,
			channel_id, thread_id, provider_user_id, raw_context
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		ctx.TicketID, ctx.OrgID, string(ctx.Provider), ctx.WorkspaceID,
		ctx.ChannelID, ctx.ThreadID, ctx.ProviderUserID, ctx.RawContext,
	)
	return err
}

// GetChannelContextByTicketID retrieves the chat destination for outbound replies.
func GetChannelContextByTicketID(db *sql.DB, orgID, ticketID string) (*TicketChannelContext, error) {
	var ctx TicketChannelContext
	var prov string
	err := db.QueryRow(`
		SELECT ticket_id, org_id, provider, workspace_id,
		       channel_id, thread_id, provider_user_id, raw_context, created_at
		FROM ticket_channel_context
		WHERE org_id = $1 AND ticket_id = $2
		LIMIT 1`,
		orgID, ticketID,
	).Scan(
		&ctx.TicketID, &ctx.OrgID, &prov, &ctx.WorkspaceID,
		&ctx.ChannelID, &ctx.ThreadID, &ctx.ProviderUserID, &ctx.RawContext, &ctx.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ctx.Provider = domain.ChatProviderName(prov)
	return &ctx, nil
}
