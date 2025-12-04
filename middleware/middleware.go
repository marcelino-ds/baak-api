package middleware

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yafyx/baak-api/utils"
	"golang.org/x/time/rate"
)

// LoggingMiddleware logs request details
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s %v", r.Method, r.RequestURI, r.RemoteAddr, time.Since(start))
	})
}

// CORSMiddleware handles CORS
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// IPRateLimiter manages per-IP rate limiting
type IPRateLimiter struct {
	ips    map[string]*rate.Limiter
	mu     sync.RWMutex
	rate   rate.Limit
	burst  int
	expiry time.Duration
}

// Global IP rate limiter
var ipLimiter = &IPRateLimiter{
	ips:    make(map[string]*rate.Limiter),
	rate:   rate.Limit(5),  // 5 requests per second
	burst:  10,             // Burst of 10
	expiry: 5 * time.Minute,
}

// getClientIP extracts the real client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (common proxy header)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// getLimiter returns or creates a rate limiter for the given IP
func (l *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	l.mu.RLock()
	limiter, exists := l.ips[ip]
	l.mu.RUnlock()

	if exists {
		return limiter
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists = l.ips[ip]; exists {
		return limiter
	}

	limiter = rate.NewLimiter(l.rate, l.burst)
	l.ips[ip] = limiter
	return limiter
}

// Cleanup removes old entries periodically
func (l *IPRateLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Simple cleanup: clear all if too many entries
	if len(l.ips) > 10000 {
		l.ips = make(map[string]*rate.Limiter)
	}
}

func init() {
	// Start cleanup goroutine
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			ipLimiter.cleanup()
		}
	}()
}

// RateLimitMiddleware handles per-IP rate limiting
func RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)
		limiter := ipLimiter.getLimiter(ip)

		if !limiter.Allow() {
			utils.WriteErrorResponse(w, http.StatusTooManyRequests,
				"Rate limit exceeded. Please try again later.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RecoveryMiddleware handles panics
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic: %v", err)
				utils.WriteErrorResponse(w, http.StatusInternalServerError,
					"An unexpected error occurred")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

