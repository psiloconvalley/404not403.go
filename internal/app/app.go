package app

import (
	"database/sql"
	"html/template"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"github.com/psiloconvalley/404not403/internal/provider/ai"
	"github.com/psiloconvalley/404not403/internal/provider/email"
)

type App struct {
	DB         *sql.DB
	Templates  *template.Template
	HTTPClient *http.Client
	Limiter    *LimiterMap
	AI         ai.Provider
	Email      email.Provider
}

type LimiterMap struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func NewLimiterMap() *LimiterMap {
	return &LimiterMap{
		limiters: make(map[string]*rate.Limiter),
	}
}

func (lm *LimiterMap) Get(ip string) *rate.Limiter {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	l, exists := lm.limiters[ip]
	if !exists {
		l = rate.NewLimiter(rate.Every(100*time.Millisecond), 30)
		lm.limiters[ip] = l
	}
	return l
}

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
