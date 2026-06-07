package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"log"
	"time"


	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/auth"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/store"
)

// Register handles POST /api/auth/register
func Register(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		var input struct {
			Email    string `json:"email"`
			Handle   string `json:"handle"`
			Password string `json:"password"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}

		input.Email = strings.TrimSpace(strings.ToLower(input.Email))
		input.Handle = strings.TrimSpace(strings.ToLower(input.Handle))

		if input.Email == "" || input.Handle == "" || input.Password == "" {
			http.Error(w, `{"error":"email, handle, and password are required"}`, http.StatusBadRequest)
			return
		}

		if len(input.Password) < 12 {
			http.Error(w, `{"error":"password must be at least 12 characters"}`, http.StatusBadRequest)
			return
		}

		if len(input.Handle) < 3 || len(input.Handle) > 32 {
			http.Error(w, `{"error":"handle must be 3-32 characters"}`, http.StatusBadRequest)
			return
		}

		if !strings.Contains(input.Email, "@") || !strings.Contains(input.Email, ".") {
			http.Error(w, `{"error":"invalid email format"}`, http.StatusBadRequest)
			return
		}

		hash, err := auth.HashPassword(input.Password)
		if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}

		user, err := store.CreateUser(a.DB, input.Email, input.Handle, hash)
		if err != nil {
			if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
				http.Error(w, `{"error":"email or handle already taken"}`, http.StatusConflict)
				return
			}
			http.Error(w, `{"error":"failed to create account"}`, http.StatusInternalServerError)
			return
		}

		token, err := auth.GenerateToken(user.ID, user.Handle, user.Role, false)
		if err != nil {
			http.Error(w, `{"error":"failed to create session"}`, http.StatusInternalServerError)
			return
		}

		setSessionCookie(w, token)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     user.ID,
			"email":  user.Email,
			"handle": user.Handle,
			"role":   user.Role,
		})
	}
}

// Login handles POST /api/auth/login
func Login(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		var input struct {
		    Identifier string `json:"identifier"`
		    Password   string `json:"password"`
		    MFACode    string `json:"mfa_code"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}

		input.Identifier = strings.TrimSpace(strings.ToLower(input.Identifier))

		if input.Identifier == "" || input.Password == "" {
			http.Error(w, `{"error":"identifier and password are required"}`, http.StatusBadRequest)
			return
		}

		// Look up by email or handle
		var user *store.User
		var err error
		if strings.Contains(input.Identifier, "@") {
			user, err = store.GetUserByEmail(a.DB, input.Identifier)
		} else {
			user, err = store.GetUserByHandle(a.DB, input.Identifier)
		}

		if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
			return
		}
			
		valid, err := auth.VerifyPassword(input.Password, user.PasswordHash)
		if err != nil || !valid {
			http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
			return
		}

		mfaVerified := false
		if user.MFAEnabled {
			if input.MFACode == "" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"mfa_required": true,
				})
				return
			}

			if user.MFASecret == nil {
				http.Error(w, `{"error":"MFA configuration error"}`, http.StatusInternalServerError)
				return
			}

			valid, err := auth.VerifyTOTP(*user.MFASecret, input.MFACode)
			if err != nil || !valid {
				http.Error(w, `{"error":"invalid MFA code"}`, http.StatusUnauthorized)
				return
			}
			mfaVerified = true
		}

		token, err := auth.GenerateToken(user.ID, user.Handle, user.Role, mfaVerified)
		if err != nil {
			http.Error(w, `{"error":"failed to create session"}`, http.StatusInternalServerError)
			return
		}

		setSessionCookie(w, token)
		store.UpdateLastLogin(a.DB, user.ID)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     user.ID,
			"email":  user.Email,
			"handle": user.Handle,
			"role":   user.Role,
		})
	}
}

// Logout handles POST /api/auth/logout
func Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"logged out"}`))
}

// Me handles GET /api/auth/me
func Me(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

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

		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":             user.ID,
			"email":          user.Email,
			"handle":         user.Handle,
			"role":           user.Role,
			"mfa_enabled":    user.MFAEnabled,
			"email_verified": user.EmailVerified,
		})
	}
}

// CheckHandle handles GET /api/auth/check-handle?handle=...
func CheckHandle(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		handle := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("handle")))
		if handle == "" || len(handle) < 3 {
			w.Write([]byte(`{"available":false,"reason":"handle must be at least 3 characters"}`))
			return
		}
		if len(handle) > 32 {
			w.Write([]byte(`{"available":false,"reason":"handle must be 32 characters or less"}`))
			return
		}

		existing, err := store.GetUserByHandle(a.DB, handle)
		if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}

		if existing != nil {
			w.Write([]byte(`{"available":false,"reason":"handle is taken"}`))
			return
		}

		w.Write([]byte(`{"available":true}`))
	}
}

// ── Cookie helper ─────────────────────────────────────────────────────────────
func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// ForgotPassword handles POST /api/auth/forgot
func ForgotPassword(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		var input struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}

		input.Email = strings.TrimSpace(strings.ToLower(input.Email))
		if input.Email == "" {
			http.Error(w, `{"error":"email is required"}`, http.StatusBadRequest)
			return
		}

		// Always return success — never reveal if email exists
		w.Write([]byte(`{"status":"if an account exists, a reset link has been sent"}`))

		// Process in background so response is instant
		go func() {
			user, err := store.GetUserByEmail(a.DB, input.Email)
			if err != nil {
				log.Printf("⚠️  ForgotPassword: user lookup error: %v", err)
				return
			}
			if user == nil {
				return
			}

			raw, hash, err := auth.GenerateAPIKey()
			if err != nil {
				log.Printf("⚠️  ForgotPassword: token generation error: %v", err)
				return
			}

			expiry := time.Now().Add(1 * time.Hour)
			if err := store.CreatePasswordReset(a.DB, user.ID, hash, expiry); err != nil {
				log.Printf("⚠️  ForgotPassword: store reset error: %v", err)
				return
			}

			if err := auth.SendPasswordResetEmail(user.Email, raw); err != nil {
				log.Printf("⚠️  ForgotPassword: email send error: %v", err)
				return
			}

			log.Printf("✅ ForgotPassword: reset email sent to %s", user.Email)
		}()
		}
}

// ResetPassword handles POST /api/auth/reset
func ResetPassword(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		var input struct {
			Token    string `json:"token"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}

		if input.Token == "" || input.Password == "" {
			http.Error(w, `{"error":"token and password are required"}`, http.StatusBadRequest)
			return
		}

		if len(input.Password) < 12 {
			http.Error(w, `{"error":"password must be at least 12 characters"}`, http.StatusBadRequest)
			return
		}

		// Hash the token to look it up
		tokenHash := auth.HashAPIKey(input.Token)

		resetID, userID, err := store.GetPasswordReset(a.DB, tokenHash)
		if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		if resetID == "" {
			http.Error(w, `{"error":"invalid or expired reset link"}`, http.StatusUnauthorized)
			return
		}

		// Hash new password
		newHash, err := auth.HashPassword(input.Password)
		if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}

		// Update password
		if err := store.UpdatePassword(a.DB, userID, newHash); err != nil {
			http.Error(w, `{"error":"failed to update password"}`, http.StatusInternalServerError)
			return
		}

		// Mark token as used
		store.MarkResetUsed(a.DB, resetID)

		w.Write([]byte(`{"status":"password updated successfully"}`))
	}
}
// ResetPage serves the password reset form.
func ResetPage(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.Templates.ExecuteTemplate(w, "reset.html", nil); err != nil {
			http.Error(w, "System Error", http.StatusInternalServerError)
		}
	}
}
