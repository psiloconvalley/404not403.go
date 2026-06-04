package middleware

import (
	"net/http"

	"github.com/psiloconvalley/404not403/internal/app"
)

// RateLimiter returns middleware that enforces per-IP rate limits.
func RateLimiter(a *app.App) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := RealIP(r)
			limiter := a.Limiter.Get(ip)
			if !limiter.Allow() {
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
