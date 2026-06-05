package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/auth"
	"github.com/psiloconvalley/404not403/internal/store"
)

// contextKey is unexported to prevent collisions.
type contextKey string

const userIDKey contextKey = "user_id"
const userRoleKey contextKey = "user_role"
const userHandleKey contextKey = "user_handle"

// RequireAuth enforces authentication via JWT cookie or API key header.
// If valid, attaches user identity to request context.
// If invalid, returns 401.
func RequireAuth(a *app.App, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// ── Try JWT cookie first ──────────────────────────────────────────
		cookie, err := r.Cookie("session")
		if err == nil && cookie.Value != "" {
			claims, err := auth.VerifyToken(cookie.Value)
			if err == nil {
				ctx := context.WithValue(r.Context(), userIDKey, claims.Subject)
				ctx = context.WithValue(ctx, userRoleKey, claims.Role)
				ctx = context.WithValue(ctx, userHandleKey, claims.Handle)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// ── Try API key header ────────────────────────────────────────────
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			// Also check Authorization: Bearer <key>
			bearer := r.Header.Get("Authorization")
			if strings.HasPrefix(bearer, "Bearer ") {
				apiKey = strings.TrimPrefix(bearer, "Bearer ")
			}
		}

		if apiKey != "" {
			keyHash := auth.HashAPIKey(apiKey)
			user, err := store.GetUserByAPIKey(a.DB, keyHash)
			if err == nil && user != nil {
				ctx := context.WithValue(r.Context(), userIDKey, user.ID)
				ctx = context.WithValue(ctx, userRoleKey, user.Role)
				ctx = context.WithValue(ctx, userHandleKey, user.Handle)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// ── No valid auth found ───────────────────────────────────────────
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
	}
}

// RequireRole wraps a handler and enforces a minimum role level.
// Role hierarchy: observer < analyst < admin
func RequireRole(minRole string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := GetRole(r)
		if !hasMinRole(role, minRole) {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// ── Context helpers — exported for handlers ──────────────────────────────────

// GetUserID extracts the authenticated user ID from context.
func GetUserID(r *http.Request) string {
	val, _ := r.Context().Value(userIDKey).(string)
	return val
}

// GetRole extracts the authenticated user role from context.
func GetRole(r *http.Request) string {
	val, _ := r.Context().Value(userRoleKey).(string)
	return val
}

// GetHandle extracts the authenticated user handle from context.
func GetHandle(r *http.Request) string {
	val, _ := r.Context().Value(userHandleKey).(string)
	return val
}

// ── Role hierarchy ───────────────────────────────────────────────────────────

var roleLevel = map[string]int{
	"observer": 1,
	"analyst":  2,
	"admin":    3,
}

func hasMinRole(userRole, requiredRole string) bool {
	return roleLevel[userRole] >= roleLevel[requiredRole]
}
