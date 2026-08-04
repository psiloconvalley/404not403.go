package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/auth"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/store"
)

var (
	passkeySessionsMu   sync.Mutex
	registerSessions    = map[string]*webauthn.SessionData{}
	loginSessions       = map[string]*webauthn.SessionData{}
)

func getWebAuthn() *webauthn.WebAuthn {
	rpID := os.Getenv("WEBAUTHN_RP_ID")
	if rpID == "" {
		rpID = "404not403.com"
	}
	rpOrigin := os.Getenv("WEBAUTHN_RP_ORIGIN")
	if rpOrigin == "" {
		rpOrigin = "https://404not403.com"
	}
	w, err := auth.NewWebAuthn(rpID, rpOrigin, "404NOT403")
	if err != nil {
		log.Fatalf("webauthn init failed: %v", err)
	}
	return w
}

// PasskeyRegisterBegin handles POST /api/auth/passkey/register/begin
func PasskeyRegisterBegin(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
			return
		}

		userID := middleware.GetUserID(r)
		if userID == "" {
			http.Error(w, `{"error":"not authenticated"}`, http.StatusUnauthorized)
			return
		}

		user, err := store.GetUserByID(a.DB, userID)
		if err != nil || user == nil {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		passkeys, err := store.GetPasskeysForUser(a.DB, userID)
		if err != nil {
			http.Error(w, `{"error":"failed to load passkeys"}`, http.StatusInternalServerError)
			return
		}

		passkeyUser := &auth.PasskeyUser{User: user, Passkeys: passkeys}
		wa := getWebAuthn()

		options, session, err := wa.BeginRegistration(passkeyUser)
		if err != nil {
			log.Printf("passkey: begin registration failed: %v", err)
			http.Error(w, `{"error":"failed to begin registration"}`, http.StatusInternalServerError)
			return
		}

		passkeySessionsMu.Lock()
		registerSessions[userID] = session
		passkeySessionsMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(options)
	}
}

// PasskeyRegisterFinish handles POST /api/auth/passkey/register/finish
func PasskeyRegisterFinish(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
			return
		}

		userID := middleware.GetUserID(r)
		if userID == "" {
			http.Error(w, `{"error":"not authenticated"}`, http.StatusUnauthorized)
			return
		}

		user, err := store.GetUserByID(a.DB, userID)
		if err != nil || user == nil {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		passkeys, err := store.GetPasskeysForUser(a.DB, userID)
		if err != nil {
			http.Error(w, `{"error":"failed to load passkeys"}`, http.StatusInternalServerError)
			return
		}

		passkeySessionsMu.Lock()
		session, ok := registerSessions[userID]
		if ok {
			delete(registerSessions, userID)
		}
		passkeySessionsMu.Unlock()

		if !ok {
			http.Error(w, `{"error":"no registration in progress"}`, http.StatusBadRequest)
			return
		}

		passkeyUser := &auth.PasskeyUser{User: user, Passkeys: passkeys}
		wa := getWebAuthn()

		credential, err := wa.FinishRegistration(passkeyUser, *session, r)
		if err != nil {
			log.Printf("passkey: finish registration failed: %v", err)
			http.Error(w, `{"error":"registration failed"}`, http.StatusBadRequest)
			return
		}

		_, err = store.CreatePasskey(
			a.DB, userID,
			credential.ID,
			credential.PublicKey,
			credential.Authenticator.AAGUID,
			"Passkey",
		)
		if err != nil {
			log.Printf("passkey: store failed: %v", err)
			http.Error(w, `{"error":"failed to save passkey"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
	}
}

// PasskeyLoginBegin handles POST /api/auth/passkey/login/begin
func PasskeyLoginBegin(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
			return
		}

		var input struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Email == "" {
			http.Error(w, `{"error":"email is required"}`, http.StatusBadRequest)
			return
		}

		user, err := store.GetUserByEmail(a.DB, input.Email)
		if err != nil || user == nil {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		passkeys, err := store.GetPasskeysForUser(a.DB, user.ID)
		if err != nil || len(passkeys) == 0 {
			http.Error(w, `{"error":"no passkeys registered"}`, http.StatusBadRequest)
			return
		}

		passkeyUser := &auth.PasskeyUser{User: user, Passkeys: passkeys}
		wa := getWebAuthn()

		options, session, err := wa.BeginLogin(passkeyUser)
		if err != nil {
			log.Printf("passkey: begin login failed: %v", err)
			http.Error(w, `{"error":"failed to begin login"}`, http.StatusInternalServerError)
			return
		}

		passkeySessionsMu.Lock()
		loginSessions[user.ID] = session
		passkeySessionsMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"options": options,
			"user_id": user.ID,
		})
	}
}

// PasskeyLoginFinish handles POST /api/auth/passkey/login/finish
func PasskeyLoginFinish(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
			return
		}

		userID := r.URL.Query().Get("user_id")
		if userID == "" {
			http.Error(w, `{"error":"user_id is required"}`, http.StatusBadRequest)
			return
		}

		user, err := store.GetUserByID(a.DB, userID)
		if err != nil || user == nil {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		passkeys, err := store.GetPasskeysForUser(a.DB, user.ID)
		if err != nil {
			http.Error(w, `{"error":"failed to load passkeys"}`, http.StatusInternalServerError)
			return
		}

		passkeySessionsMu.Lock()
		session, ok := loginSessions[user.ID]
		if ok {
			delete(loginSessions, user.ID)
		}
		passkeySessionsMu.Unlock()

		if !ok {
			http.Error(w, `{"error":"no login in progress"}`, http.StatusBadRequest)
			return
		}

		passkeyUser := &auth.PasskeyUser{User: user, Passkeys: passkeys}
		wa := getWebAuthn()

		credential, err := wa.FinishLogin(passkeyUser, *session, r)
		if err != nil {
			log.Printf("passkey: finish login failed: %v", err)
			http.Error(w, `{"error":"login failed"}`, http.StatusUnauthorized)
			return
		}

		// Update sign count
		store.UpdatePasskeySignCount(a.DB, credential.ID, int64(credential.Authenticator.SignCount))
		store.UpdateLastLogin(a.DB, user.ID)

		// Issue JWT
		token, err := auth.GenerateToken(user.ID, user.Handle, user.Role, user.MFAEnabled)
		if err != nil {
			http.Error(w, `{"error":"failed to create session"}`, http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "token",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":     user.ID,
			"email":  user.Email,
			"handle": user.Handle,
			"role":   user.Role,
		})
	}
}

// PasskeyList handles GET /api/auth/passkeys
func PasskeyList(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"use GET"}`, http.StatusMethodNotAllowed)
			return
		}

		userID := middleware.GetUserID(r)
		if userID == "" {
			http.Error(w, `{"error":"not authenticated"}`, http.StatusUnauthorized)
			return
		}

		passkeys, err := store.GetPasskeysForUser(a.DB, userID)
		if err != nil {
			log.Printf("passkey: list failed: %v", err)
			http.Error(w, `{"error":"failed to load passkeys"}`, http.StatusInternalServerError)
			return
		}

		type passkeyResponse struct {
			ID         string     `json:"id"`
			Name       string     `json:"name"`
			SignCount  int64      `json:"sign_count"`
			CreatedAt  string     `json:"created_at"`
			LastUsedAt *string    `json:"last_used_at,omitempty"`
		}

		items := make([]passkeyResponse, len(passkeys))
		for i, p := range passkeys {
			items[i] = passkeyResponse{
				ID:        p.ID,
				Name:      p.Name,
				SignCount: p.SignCount,
				CreatedAt: p.CreatedAt.Format("2006-01-02T15:04:05Z"),
			}
			if p.LastUsedAt != nil {
				s := p.LastUsedAt.Format("2006-01-02T15:04:05Z")
				items[i].LastUsedAt = &s
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"passkeys": items})
	}
}

// PasskeyDelete handles DELETE /api/auth/passkeys?id=
func PasskeyDelete(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, `{"error":"use DELETE"}`, http.StatusMethodNotAllowed)
			return
		}

		userID := middleware.GetUserID(r)
		if userID == "" {
			http.Error(w, `{"error":"not authenticated"}`, http.StatusUnauthorized)
			return
		}

		passkeyID := r.URL.Query().Get("id")
		if passkeyID == "" {
			http.Error(w, `{"error":"passkey id is required"}`, http.StatusBadRequest)
			return
		}

		if err := store.DeletePasskey(a.DB, userID, passkeyID); err != nil {
			log.Printf("passkey: delete failed: %v", err)
			http.Error(w, `{"error":"failed to delete passkey"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	}
}
