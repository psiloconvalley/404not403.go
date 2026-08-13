package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const (
	csrfCookieName = "csrf_token"
	csrfHeaderName = "X-CSRF-Token"
	csrfTokenBytes = 32
)

// GenerateCSRFToken returns a cryptographically random hex token.
func GenerateCSRFToken() (string, error) {
	b := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SetCSRFCookie writes the CSRF token as a non-HttpOnly cookie.
// Non-HttpOnly so JavaScript can read it and attach to requests.
// SameSite=Strict prevents cross-site requests from including it.
func SetCSRFCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // JS must read this
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// RequireCSRF validates that the X-CSRF-Token header matches the csrf_token cookie.
// Exempt: GET, HEAD, OPTIONS — they never change state.
// Exempt: requests with no session cookie — they can't be CSRF targets.
// If validation fails: 403 Forbidden.
func RequireCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Safe methods never change state — exempt
		if r.Method == http.MethodGet ||
			r.Method == http.MethodHead ||
			r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		// No session cookie — not a CSRF target (login/register flow)
		if _, err := r.Cookie("session"); err != nil {
			next.ServeHTTP(w, r)
			return
		}

		// Validate token
		cookie, err := r.Cookie(csrfCookieName)
		if err != nil || cookie.Value == "" {
			http.Error(w, `{"error":"csrf token missing"}`, http.StatusForbidden)
			return
		}

		header := r.Header.Get(csrfHeaderName)
		if header == "" || header != cookie.Value {
			http.Error(w, `{"error":"csrf token invalid"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	}
}
