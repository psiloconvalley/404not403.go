package chat

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/psiloconvalley/404not403/internal/domain"
)

func TestSlackVerifyWebhook_Success(t *testing.T) {
	p := NewSlackProvider(nil)
	secret := "test_secret_12345"
	payload := []byte(`{"type":"event_callback"}`)
	now := time.Now().Unix()

	sigBase := fmt.Sprintf("v0:%d:%s", now, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sigBase))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slack/events", nil)
	req.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(now, 10))
	req.Header.Set("X-Slack-Signature", sig)

	if err := p.VerifyWebhook(req, secret, payload); err != nil {
		t.Fatalf("expected valid signature, got error: %v", err)
	}
}

func TestSlackVerifyWebhook_ReplayRejected(t *testing.T) {
	p := NewSlackProvider(nil)
	secret := "test_secret_12345"
	payload := []byte(`{"type":"event_callback"}`)
	oldTimestamp := time.Now().Unix() - 400

	sigBase := fmt.Sprintf("v0:%d:%s", oldTimestamp, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sigBase))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slack/events", nil)
	req.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(oldTimestamp, 10))
	req.Header.Set("X-Slack-Signature", sig)

	if err := p.VerifyWebhook(req, secret, payload); err == nil {
		t.Fatal("expected expired timestamp to fail replay defense check")
	}
}

func TestSlackParseInbound_Mention(t *testing.T) {
	p := NewSlackProvider(nil)
	payload := []byte(`{
		"type": "event_callback",
		"team_id": "T12345",
		"event_time": 1711710000,
		"event": {
			"type": "app_mention",
			"user": "U98765",
			"text": "<@U0404BOT> my screen is cracked",
			"channel": "C55555",
			"ts": "1711710000.123456"
		}
	}`)

	msg, err := p.ParseInbound(payload)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected parsed message, got nil")
	}
	if msg.Provider != domain.ChatProviderSlack {
		t.Errorf("expected provider slack, got %s", msg.Provider)
	}
	if msg.WorkspaceID != "T12345" {
		t.Errorf("expected team T12345, got %s", msg.WorkspaceID)
	}
	if msg.Body != "<@U0404BOT> my screen is cracked" {
		t.Errorf("unexpected body: %s", msg.Body)
	}
}
