package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type ResendProvider struct {
	apiKey string
	client *http.Client
}

func NewResendProvider(client *http.Client) *ResendProvider {
	return &ResendProvider{
		apiKey: os.Getenv("RESEND_API_KEY"),
		client: client,
	}
}

func (p *ResendProvider) Send(ctx context.Context, input SendInput) error {
	if p.apiKey == "" {
		return fmt.Errorf("RESEND_API_KEY not set")
	}

	from := os.Getenv("RESEND_FROM_EMAIL")
	if from == "" {
		from = "support@404not403.com" // Fallback
	}
	if input.From != "" {
		from = input.From
	}

	payload := map[string]interface{}{
		"from":    from,
		"to":      []string{input.To},
		"subject": input.Subject,
		"html":    input.Body,
	}

	jsonBody, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.resend.com/emails", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("resend api error: status %d", resp.StatusCode)
	}

	return nil
}
