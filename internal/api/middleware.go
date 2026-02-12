package api

import (
	"log/slog"
	"net/http"
	"sync"
	"time"
)

func AgentIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentID := r.Header.Get("X-Agent-ID")
		if agentID == "" {
			http.Error(w, `{"error":"X-Agent-ID header required"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func AdminAuthMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+token {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"duration_ms", time.Since(start).Milliseconds(),
				"agent", r.Header.Get("X-Agent-ID"),
			)
		})
	}
}

type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func RateLimitMiddleware(requestsPerMinute int) func(http.Handler) http.Handler {
	rl := &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    requestsPerMinute,
		window:   time.Minute,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-Agent-ID")
			if key == "" {
				key = r.RemoteAddr
			}
			rl.mu.Lock()
			now := time.Now()
			cutoff := now.Add(-rl.window)
			var valid []time.Time
			for _, t := range rl.requests[key] {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) >= rl.limit {
				rl.mu.Unlock()
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			rl.requests[key] = append(valid, now)
			rl.mu.Unlock()
			next.ServeHTTP(w, r)
		})
	}
}
