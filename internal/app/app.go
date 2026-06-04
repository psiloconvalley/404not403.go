package app

import (
	"database/sql"
	"html/template"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// App carries all shared dependencies.
// Every handler and middleware receives a pointer to this struct.
type App struct {
	DB         *sql.DB
	Templates  *template.Template
	HTTPClient *http.Client
	Limiter    *LimiterMap
}

// LimiterMap is a per-IP rate limiter store. Thread-safe.
// Each IP gets: 5 requests per second, burst of 10.
type LimiterMap struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

// NewLimiterMap creates an empty rate limiter map.
func NewLimiterMap() *LimiterMap {
	return &LimiterMap{
		limiters: make(map[string]*rate.Limiter),
	}
}

// Get returns the rate limiter for the given IP, creating one if needed.
func (lm *LimiterMap) Get(ip string) *rate.Limiter {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	l, exists := lm.limiters[ip]
	if !exists {
		l = rate.NewLimiter(rate.Every(200*time.Millisecond), 10)
		lm.limiters[ip] = l
	}
	return l
}

// NewHTTPClient creates the outbound HTTP client with strict timeout
// and redirect limits. Used by the scan engine.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}
