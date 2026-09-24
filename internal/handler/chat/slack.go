package chat

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/psiloconvalley/404not403/internal/provider/chat"
	"github.com/psiloconvalley/404not403/internal/service/intake"
)

type Handler struct {
	intakeSvc     *intake.Service
	slackProvider chat.ChatProvider
}

func New(intakeSvc *intake.Service, slackProvider chat.ChatProvider) *Handler {
	return &Handler{
		intakeSvc:     intakeSvc,
		slackProvider: slackProvider,
	}
}

// HandleSlackEvents receives and processes webhooks from the Slack Events API.
func (h *Handler) HandleSlackEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 1. Verify Slack HMAC Signature
	signingSecret := os.Getenv("SLACK_SIGNING_SECRET")
	if signingSecret != "" {
		if err := h.slackProvider.VerifyWebhook(r, signingSecret, bodyBytes); err != nil {
			log.Printf("❌ Slack webhook signature verification failed: %v", err)
			http.Error(w, "Unauthorized signature", http.StatusUnauthorized)
			return
		}
	} else {
		log.Printf("⚠️ SLACK_SIGNING_SECRET not configured — skipping HMAC validation in dev")
	}

	// 2. Intercept URL verification challenge from Slack setup
	var challengeEnvelope struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(bodyBytes, &challengeEnvelope); err == nil {
		if challengeEnvelope.Type == "url_verification" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"challenge": challengeEnvelope.Challenge,
			})
			return
		}
	}

	// 3. Parse inbound chat message into normalized carrier
	msg, err := h.slackProvider.ParseInbound(bodyBytes)
	if err != nil {
		log.Printf("⚠️ Failed to parse Slack inbound payload: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// 4. Respond 200 OK immediately to satisfy Slack 3-second timeout
	w.WriteHeader(http.StatusOK)

	// 5. Process event asynchronously
	if msg != nil {
		go func() {
			ctx := context.Background()
			if err := h.intakeSvc.HandleInboundMessage(ctx, msg); err != nil {
				log.Printf("❌ Error processing Slack message from %s: %v", msg.ProviderUserID, err)
			}
		}()
	}
}
