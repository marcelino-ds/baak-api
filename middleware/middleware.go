package middleware

import (
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/yafyx/baak-api/config"
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
		addVary(w.Header(), "Origin")
		if r.Method == http.MethodOptions {
			addVary(w.Header(), "Access-Control-Request-Method")
			addVary(w.Header(), "Access-Control-Request-Headers")
		}
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		if !originAllowed(origin, config.AppConfig.AllowedOrigins) {
			if r.Method == http.MethodOptions {
				utils.WriteErrorResponseWithCode(
					w,
					http.StatusForbidden,
					"Origin is not allowed",
					"CORS_FORBIDDEN",
				)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", allowedOriginValue(origin, config.AppConfig.AllowedOrigins))
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			requestedMethod := strings.TrimSpace(r.Header.Get("Access-Control-Request-Method"))
			if requestedMethod != "" && requestedMethod != http.MethodGet {
				utils.WriteErrorResponseWithCode(
					w,
					http.StatusForbidden,
					"Requested method is not allowed",
					"CORS_METHOD_FORBIDDEN",
				)
				return
			}
			if !headersAllowed(r.Header.Get("Access-Control-Request-Headers")) {
				utils.WriteErrorResponseWithCode(
					w,
					http.StatusForbidden,
					"Requested headers are not allowed",
					"CORS_HEADERS_FORBIDDEN",
				)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func addVary(header http.Header, value string) {
	for _, existing := range header.Values("Vary") {
		for _, item := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(item), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}

func originAllowed(origin string, allowedOrigins []string) bool {
	for _, allowed := range allowedOrigins {
		allowed = strings.TrimSpace(allowed)
		if allowed == "*" || allowed == origin {
			return true
		}
	}
	return false
}

func allowedOriginValue(origin string, allowedOrigins []string) string {
	for _, allowed := range allowedOrigins {
		if strings.TrimSpace(allowed) == "*" {
			return "*"
		}
	}
	return origin
}

func headersAllowed(requested string) bool {
	for _, header := range strings.Split(requested, ",") {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		if !strings.EqualFold(header, "Content-Type") && !strings.EqualFold(header, "Authorization") {
			return false
		}
	}
	return true
}

// IPRateLimiter manages per-IP rate limiting
type IPRateLimiter struct {
	ips    map[string]limiterEntry
	mu     sync.Mutex
	rate   rate.Limit
	burst  int
	expiry time.Duration
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	ipLimiter     *IPRateLimiter
	ipLimiterOnce sync.Once
)

func newIPRateLimiter(ratePerMin, burst int) *IPRateLimiter {
	if ratePerMin <= 0 {
		ratePerMin = 60
	}
	if burst <= 0 {
		burst = 10
	}
	return &IPRateLimiter{
		ips:    make(map[string]limiterEntry),
		rate:   rate.Limit(ratePerMin) / 60,
		burst:  burst,
		expiry: 5 * time.Minute,
	}
}

func getIPRateLimiter() *IPRateLimiter {
	ipLimiterOnce.Do(func() {
		ipLimiter = newIPRateLimiter(config.AppConfig.RateLimitPerMin, config.AppConfig.RateLimitBurst)
		go func() {
			ticker := time.NewTicker(10 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				ipLimiter.cleanup()
			}
		}()
	})
	return ipLimiter
}

// Only the socket peer is trusted; proxy headers can be supplied by the client.
func getClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		if address, parseErr := netip.ParseAddr(host); parseErr == nil {
			return address.Unmap().String()
		}
		return host
	}
	if address, parseErr := netip.ParseAddr(strings.Trim(strings.TrimSpace(r.RemoteAddr), "[]")); parseErr == nil {
		return address.Unmap().String()
	}
	return strings.TrimSpace(r.RemoteAddr)
}

// getLimiter returns or creates a rate limiter for the given IP
func (l *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ips == nil {
		l.ips = make(map[string]limiterEntry)
	}
	if entry, exists := l.ips[ip]; exists {
		entry.lastSeen = time.Now()
		l.ips[ip] = entry
		return entry.limiter
	}
	limiter := rate.NewLimiter(l.rate, l.burst)
	l.ips[ip] = limiterEntry{limiter: limiter, lastSeen: time.Now()}
	return limiter
}

// Cleanup removes old entries periodically
func (l *IPRateLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-l.expiry)
	for ip, entry := range l.ips {
		// Removing a partially refilled bucket would give the client a fresh burst.
		if entry.lastSeen.Before(cutoff) && entry.limiter.Tokens() >= float64(l.burst) {
			delete(l.ips, ip)
		}
	}
}

// RateLimitMiddleware handles per-IP rate limiting
func RateLimitMiddleware(next http.Handler) http.Handler {
	return rateLimitMiddleware(next, getIPRateLimiter())
}

func rateLimitMiddleware(next http.Handler, limiter *IPRateLimiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || r.URL.Path == "/live" || r.URL.Path == "/ready" {
			next.ServeHTTP(w, r)
			return
		}
		ip := getClientIP(r)
		clientLimiter := limiter.getLimiter(ip)

		if !clientLimiter.Allow() {
			utils.WriteErrorResponseWithCode(
				w,
				http.StatusTooManyRequests,
				"Rate limit exceeded. Please try again later.",
				"RATE_LIMITED",
			)
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
