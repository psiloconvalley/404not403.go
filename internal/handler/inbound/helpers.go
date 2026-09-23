package inbound

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/psiloconvalley/404not403/internal/provider/email"
	"github.com/psiloconvalley/404not403/internal/store"
)

// sendAutoReply sends a confirmation email with a tracking link and clarifying questions.
func (h *Handler) sendAutoReply(ctx context.Context, org *store.Organization, customer *store.Customer, toEmail string, name *string, displayID, subject string) {
	if h.app.Email == nil {
		return
	}

	displayName := "there"
	if name != nil && *name != "" {
		displayName = *name
	}

	// Ensure customer has a tracking token
	rawToken, tokenHash, err := generateTrackingToken()
	if err == nil {
		expires := time.Now().Add(30 * 24 * time.Hour)
		_ = store.SetTrackingToken(h.app.DB, customer.ID, tokenHash, expires)
	}

	trackingURL := "https://404not403.com"
	if rawToken != "" {
		trackingURL = fmt.Sprintf("https://404not403.com/help/track?token=%s", rawToken)
	}

	emailSubject := fmt.Sprintf("Re: %s [%s]", subject, displayID)

	emailBody := fmt.Sprintf(`
<div style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;max-width:600px;margin:0 auto;padding:2rem;color:#292524;background:#ffffff;border:1px solid #e7e5e4;border-radius:8px;">
	<div style="margin-bottom:1.5rem;border-bottom:1px solid #f5f5f4;padding-bottom:1rem;">
		<span style="display:inline-block;padding:0.25rem 0.6rem;background:#fef3c7;color:#92400e;font-size:0.8rem;font-weight:600;border-radius:4px;letter-spacing:0.05em;">%s</span>
	</div>

	<h2 style="font-size:1.25rem;font-weight:700;margin:0 0 1rem 0;color:#1c1917;">We've received your request</h2>

	<p style="color:#57534e;line-height:1.6;margin:0 0 1rem 0;">
		Hi %s, your request has been logged under reference <strong>%s</strong>. Our IT support team is actively reviewing it.
	</p>

	<div style="background:#fafaf9;border-left:3px solid #ea580c;padding:1rem;margin:1.5rem 0;border-radius:0 4px 4px 0;">
		<p style="font-size:0.875rem;font-weight:600;color:#1c1917;margin:0 0 0.5rem 0;">To help us resolve this as fast as possible, please reply with:</p>
		<ol style="margin:0;padding-left:1.25rem;font-size:0.875rem;color:#57534e;line-height:1.5;">
			<li>When did this issue first start?</li>
			<li>Is this only affecting you, or multiple people on your team?</li>
			<li>What device model or operating system are you using?</li>
		</ol>
		<p style="font-size:0.8rem;color:#78716c;margin:0.5rem 0 0 0;">(You can simply reply directly to this email)</p>
	</div>

	<div style="margin:1.5rem 0;">
		<a href="%s" style="display:inline-block;padding:0.65rem 1.25rem;background:#ea580c;color:#ffffff;text-decoration:none;font-weight:600;font-size:0.875rem;border-radius:6px;">View & Track Request</a>
	</div>

	<div style="border-top:1px solid #f5f5f4;padding-top:1rem;margin-top:2rem;font-size:0.75rem;color:#a8a29e;">
		<p style="margin:0;">%s Support · Powered by 404NOT403</p>
	</div>
</div>`,
		displayID,
		displayName,
		displayID,
		trackingURL,
		org.Name,
	)

	_ = h.app.Email.Send(ctx, email.SendInput{
		To:      toEmail,
		Subject: emailSubject,
		Body:    emailBody,
	})
}

func generateTrackingToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, hash, nil
}

// extractEmail pulls the email address from "Name <email@example.com>" format
func extractEmail(s string) string {
	s = strings.TrimSpace(s)
	if start := strings.Index(s, "<"); start != -1 {
		if end := strings.Index(s, ">"); end > start {
			return strings.TrimSpace(s[start+1 : end])
		}
	}
	return s
}

// extractName pulls the name from "Name <email@example.com>" format
func extractName(s string) *string {
	s = strings.TrimSpace(s)
	if start := strings.Index(s, "<"); start > 0 {
		name := strings.TrimSpace(s[:start])
		if name != "" {
			return &name
		}
	}
	return nil
}
