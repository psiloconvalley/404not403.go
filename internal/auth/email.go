package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// SendPasswordResetEmail sends a reset link via Resend.
func SendPasswordResetEmail(toEmail, resetToken string) error {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("RESEND_API_KEY not set")
	}

	resetURL := "https://404not403.com/reset?token=" + resetToken

	body := map[string]interface{}{
		"from":    "404NOT403 <auth@404not403.com>",
		"to":      []string{toEmail},
		"subject": "Password Reset — 404NOT403",
		"html": "<div style=\"font-family:monospace;background:#09090b;color:#f0f0f0;padding:2rem;\">" +
			"<h2 style=\"color:#14b8a6;letter-spacing:2px;\">404NOT403</h2>" +
			"<p>A password reset was requested for your account.</p>" +
			"<p>Click the link below to set a new password. This link expires in 1 hour.</p>" +
			"<p><a href=\"" + resetURL + "\" style=\"color:#14b8a6;\">" + resetURL + "</a></p>" +
			"<p style=\"color:#64748b;margin-top:2rem;font-size:0.8rem;\">If you did not request this, ignore this email.</p>" +
			"</div>",
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal email body: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("resend API returned status %d", resp.StatusCode)
	}

	return nil
}
