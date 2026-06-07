package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/store"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/checkout/session"
	"github.com/stripe/stripe-go/v76/webhook"
)

func CreateCheckoutSession(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
		priceID := os.Getenv("STRIPE_PRICE_ID")
		userID := middleware.GetUserID(r)

		params := &stripe.CheckoutSessionParams{
			SuccessURL: stripe.String("https://404not403.com/?payment=success"),
			CancelURL:  stripe.String("https://404not403.com/?payment=cancelled"),
			Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
			LineItems: []*stripe.CheckoutSessionLineItemParams{
				{
					Price:    stripe.String(priceID),
					Quantity: stripe.Int64(1),
				},
			},
			ClientReferenceID: stripe.String(userID),
		}

		s, err := session.New(params)
		if err != nil {
			http.Error(w, "failed to create session", http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"url": s.URL})
	}
}

// StripeWebhook handles POST /api/webhooks/stripe
func StripeWebhook(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 65536)
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "payload too large", http.StatusBadRequest)
			return
		}

		endpointSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
		signature := r.Header.Get("Stripe-Signature")
		event, err := webhook.ConstructEvent(payload, signature, endpointSecret)
		if err != nil {
			log.Printf("⚠️  Stripe webhook signature failed: %v", err)
			http.Error(w, "invalid signature", http.StatusBadRequest)
			return
		}

		if event.Type == "checkout.session.completed" {
			var s stripe.CheckoutSession
			if err := json.Unmarshal(event.Data.Raw, &s); err != nil {
				log.Printf("⚠️  Stripe webhook json error: %v", err)
				http.Error(w, "json error", http.StatusBadRequest)
				return
			}

			userID := s.ClientReferenceID
			if userID != "" {
				if err := store.UpgradeUser(a.DB, userID, "analyst"); err != nil {
					log.Printf("⚠️  Stripe webhook: failed to upgrade user %s: %v", userID, err)
				} else {
					log.Printf("💰 User %s upgraded to ANALYST via Stripe", userID)
				}
			}
		}

		w.WriteHeader(http.StatusOK)
	}
}
