package chat

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/psiloconvalley/404not403/internal/domain"
)

type SlackProvider struct {
	httpClient *http.Client
}

func NewSlackProvider(httpClient *http.Client) *SlackProvider {
	if httpClient == nil {
	httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &SlackProvider{httpClient: httpClient}
}

func (s *SlackProvider) Name() domain.ChatProviderName {
	return domain.ChatProviderSlack
}

// VerifyWebhook validates the X-Slack-Signature using HMAC-SHA256.
func (s *SlackProvider) VerifyWebhook(r *http.Request, signingSecret string, bodyBytes []byte) error {
	if signingSecret == "" {
		return &PermanentError{Err: errors.New("slack signing secret not configured")}
	}

	timestampStr := r.Header.Get("X-Slack-Request-Timestamp")
	if timestampStr == "" {
		return &PermanentError{Err: errors.New("missing X-Slack-Request-Timestamp header")}
	}

	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return &PermanentError{Err: fmt.Errorf("invalid timestamp format: %w", err)}
	}

	// Prevent replay attacks (5 minute window)
	now := time.Now().Unix()
	if math.Abs(float64(now-timestamp)) > 300 {
		return &PermanentError{Err: errors.New("slack request timestamp out of bounds (replay protection)")}
	}

	slackSig := r.Header.Get("X-Slack-Signature")
	if slackSig == "" {
		return &PermanentError{Err: errors.New("missing X-Slack-Signature header")}
	}

	sigBase := fmt.Sprintf("v0:%d:%s", timestamp, string(bodyBytes))
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(sigBase))
	expectedSig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(slackSig), []byte(expectedSig)) {
		return &PermanentError{Err: errors.New("slack signature mismatch")}
	}

	return nil
}

// Raw Slack Events Schema Models
type slackEventEnvelope struct {
	Type        string          `json:"type"`
	Challenge   string          `json:"challenge"`
	TeamID      string          `json:"team_id"`
	APIAppID    string          `json:"api_app_id"`
	EventID     string          `json:"event_id"`
	EventTime   int64           `json:"event_time"`
	Event       json.RawMessage `json:"event"`
}

type slackInnerEvent struct {
	Type           string `json:"type"`
	Subtype        string `json:"subtype"`
	User           string `json:"user"`
	BotID          string `json:"bot_id"`
	Text           string `json:"text"`
	Channel        string `json:"channel"`
	ChannelType    string `json:"channel_type"`
	ThreadTS       string `json:"thread_ts"`
	TS             string `json:"ts"`
	EventTimestamp string `json:"event_ts"`
}

// ParseInbound extracts a normalized ChatMessage from a Slack event payload.
func (s *SlackProvider) ParseInbound(bodyBytes []byte) (*ChatMessage, error) {
	var env slackEventEnvelope
	if err := json.Unmarshal(bodyBytes, &env); err != nil {
		return nil, &PermanentError{Err: fmt.Errorf("invalid slack json: %w", err)}
	}

	if env.Type != "event_callback" {
		return nil, nil // Ignored envelope type (e.g. url_verification handled separately)
	}

	var inner slackInnerEvent
	if err := json.Unmarshal(env.Event, &inner); err != nil {
		return nil, &PermanentError{Err: fmt.Errorf("invalid slack inner event json: %w", err)}
	}

	// 1. Filter out bot messages and loop triggers
	if inner.BotID != "" || inner.Subtype == "bot_message" {
		return nil, nil
	}

	isDM := inner.ChannelType == "im"
	isMention := inner.Type == "app_mention"

	// 2. Only process direct messages or explicit app mentions
	if !isDM && !isMention {
		return nil, nil
	}

	threadID := inner.ThreadTS
	if threadID == "" {
		threadID = inner.TS
	}

	cleanedText := strings.TrimSpace(inner.Text)

	return &ChatMessage{
		Provider:        domain.ChatProviderSlack,
		WorkspaceID:     env.TeamID,
		ChannelID:       inner.Channel,
		ThreadID:        threadID,
		ProviderUserID:  inner.User,
		Body:            cleanedText,
		IsDirectMessage: isDM,
		IsMention:       isMention,
		ReceivedAt:      time.Unix(env.EventTime, 0).UTC(),
		RawPayload:      bodyBytes,
	}, nil
}

// SendReply sends an outbound reply into the specific thread.
func (s *SlackProvider) SendReply(ctx context.Context, botToken string, target ChatTarget, body string) error {
	if botToken == "" {
		return &PermanentError{Err: errors.New("slack bot token is required")}
	}

	payload := map[string]any{
		"channel":   target.ChannelID,
		"thread_ts": target.ThreadID,
		"text":      body,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return &PermanentError{Err: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(jsonBytes))
	if err != nil {
		return &PermanentError{Err: err}
	}

	req.Header.Set("Authorization", "Bearer "+botToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return &RetryableError{Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return &RetryableError{Err: fmt.Errorf("slack api returned status %d", resp.StatusCode)}
	}

	var slackResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&slackResp); err != nil {
		return &RetryableError{Err: err}
	}

	if !slackResp.OK {
		return &PermanentError{Err: fmt.Errorf("slack chat.postMessage failed: %s", slackResp.Error)}
	}

	return nil
}

// ResolveUser fetches user details from Slack users.info API.
func (s *SlackProvider) ResolveUser(ctx context.Context, botToken string, providerUserID string) (*ChatUser, error) {
	if botToken == "" || providerUserID == "" {
		return nil, &PermanentError{Err: errors.New("bot token and user id are required")}
	}

	url := fmt.Sprintf("https://slack.com/api/users.info?user=%s", providerUserID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &PermanentError{Err: err}
	}

	req.Header.Set("Authorization", "Bearer "+botToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, &RetryableError{Err: err}
	}
	defer resp.Body.Close()

	var slackUserResp struct {
		OK    bool `json:"ok"`
		User  struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			RealName string `json:"real_name"`
			Profile  struct {
				Email       string `json:"email"`
				DisplayName string `json:"display_name"`
			} `json:"profile"`
		} `json:"user"`
		Error string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&slackUserResp); err != nil {
		return nil, &RetryableError{Err: err}
	}

	if !slackUserResp.OK {
		return nil, &PermanentError{Err: fmt.Errorf("slack users.info error: %s", slackUserResp.Error)}
	}

	var emailPtr *string
	if slackUserResp.User.Profile.Email != "" {
		e := strings.ToLower(strings.TrimSpace(slackUserResp.User.Profile.Email))
		emailPtr = &e
	}

	displayName := slackUserResp.User.Profile.DisplayName
	if displayName == "" {
		displayName = slackUserResp.User.RealName
	}
	if displayName == "" {
		displayName = slackUserResp.User.Name
	}

	return &ChatUser{
		ProviderUserID: slackUserResp.User.ID,
		Email:          emailPtr,
		DisplayName:    displayName,
		Handle:         slackUserResp.User.Name,
	}, nil
}
