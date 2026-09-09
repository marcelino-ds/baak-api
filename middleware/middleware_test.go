package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yafyx/baak-api/config"
)

func TestCORSMiddlewareAllowsConfiguredOrigin(t *testing.T) {
	previous := config.AppConfig
	config.AppConfig.AllowedOrigins = []string{"https://app.example"}
	defer func() { config.AppConfig = previous }()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set("Origin", "https://app.example")
	response := httptest.NewRecorder()
	CORSMiddleware(next).ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want configured origin", got)
	}
	if got := response.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin", got)
	}
}

func TestCORSMiddlewareRejectsDisallowedPreflight(t *testing.T) {
	previous := config.AppConfig
	config.AppConfig.AllowedOrigins = []string{"https://app.example"}
	defer func() { config.AppConfig = previous }()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler ran for a disallowed preflight")
	})
	request := httptest.NewRequest(http.MethodOptions, "/health", nil)
	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	response := httptest.NewRecorder()
	CORSMiddleware(next).ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed origin received ACAO header %q", got)
	}
}

func TestCORSMiddlewareHandlesWildcardAndMissingOrigin(t *testing.T) {
	previous := config.AppConfig
	config.AppConfig.AllowedOrigins = []string{"*"}
	defer func() { config.AppConfig = previous }()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	for _, origin := range []string{"", "https://any.example"} {
		request := httptest.NewRequest(http.MethodGet, "/health", nil)
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		response := httptest.NewRecorder()
		CORSMiddleware(next).ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("origin %q status = %d, want 204", origin, response.Code)
		}
		if origin != "" && response.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("origin %q did not receive wildcard ACAO", origin)
		}
	}
}

func TestRateLimitMiddlewareUsesConfiguredBurst(t *testing.T) {
	limiter := newIPRateLimiter(60, 2)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := rateLimitMiddleware(next, limiter)
	for attempt := 0; attempt < 3; attempt++ {
		request := httptest.NewRequest(http.MethodGet, "/health", nil)
		request.RemoteAddr = "192.0.2.1:1234"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		want := http.StatusNoContent
		if attempt == 2 {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("attempt %d status = %d, want %d", attempt+1, response.Code, want)
		}
	}
}

func TestClientIPHandlesIPv6RemoteAddress(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.RemoteAddr = "[2001:db8::1]:4321"
	if got := getClientIP(request); got != "2001:db8::1" {
		t.Fatalf("client IP = %q, want IPv6 host", got)
	}
}

func TestCORSMiddlewareVariesResponsesWithoutOrigin(t *testing.T) {
	previous := config.AppConfig
	config.AppConfig.AllowedOrigins = []string{"https://app.example"}
	t.Cleanup(func() { config.AppConfig = previous })

	response := httptest.NewRecorder()
	response.Header().Set("Vary", "Accept-Encoding")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	CORSMiddleware(next).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("request without Origin was rejected: %d", response.Code)
	}
	if !headerContains(response.Header(), "Vary", "Origin") || !headerContains(response.Header(), "Vary", "Accept-Encoding") {
		t.Errorf("Vary = %v; must preserve both Origin and Accept-Encoding", response.Header().Values("Vary"))
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("request without Origin received ACAO = %q", got)
	}
}

func TestCORSMiddlewarePreflight(t *testing.T) {
	previous := config.AppConfig
	config.AppConfig.AllowedOrigins = []string{"https://app.example"}
	t.Cleanup(func() { config.AppConfig = previous })
	tests := []struct {
		name    string
		method  string
		headers string
		status  int
	}{
		{name: "GET with allowed headers", method: http.MethodGet, headers: "content-type, AUTHORIZATION", status: 204},
		{name: "unsupported method", method: http.MethodPost, status: 403},
		{name: "unsupported header", method: http.MethodGet, headers: "X-Client-Secret", status: 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodOptions, "/jadwal", nil)
			request.Header.Set("Origin", "https://app.example")
			request.Header.Set("Access-Control-Request-Method", tt.method)
			request.Header.Set("Access-Control-Request-Headers", tt.headers)
			response := httptest.NewRecorder()
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("preflight reached application handler")
			})
			CORSMiddleware(next).ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("preflight status = %d, want %d: %s", response.Code, tt.status, response.Body.String())
			}
			for _, field := range []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"} {
				if !headerContains(response.Header(), "Vary", field) {
					t.Errorf("preflight Vary does not include %s", field)
				}
			}
			if tt.status == http.StatusNoContent {
				if response.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
					t.Error("allowed preflight lacks matching origin")
				}
				if !headerContains(response.Header(), "Access-Control-Allow-Methods", "GET") {
					t.Error("allowed preflight does not advertise GET")
				}
			}
		})
	}
}

func headerContains(header http.Header, name, value string) bool {
	for _, line := range header.Values(name) {
		for _, part := range strings.Split(line, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return true
			}
		}
	}
	return false
}

func TestCORSMiddlewareDoesNotGrantSimilarOrigin(t *testing.T) {
	previous := config.AppConfig
	config.AppConfig.AllowedOrigins = []string{"https://app.example"}
	t.Cleanup(func() { config.AppConfig = previous })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Origin", "https://app.example.attacker.example")
	response := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	CORSMiddleware(next).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("disallowed origin should receive normal response without CORS grant: %d %v", response.Code, response.Header())
	}
}

func TestRateLimitRefillsAtConfiguredRate(t *testing.T) {
	tests := []struct {
		perMinute int
		interval  time.Duration
	}{
		{perMinute: 30, interval: 2 * time.Second},
		{perMinute: 120, interval: 500 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.perMinute), func(t *testing.T) {
			limiter := newIPRateLimiter(tt.perMinute, 1).getLimiter("192.0.2.1")
			now := time.Now()
			if !limiter.AllowN(now, 1) || limiter.AllowN(now.Add(tt.interval/2), 1) {
				t.Fatal("burst was not enforced during refill")
			}
			if !limiter.AllowN(now.Add(tt.interval), 1) {
				t.Fatal("token was not replenished at configured rate")
			}
		})
	}
}

func TestRateLimitIgnoresForwardedHeadersAndClientPort(t *testing.T) {
	limiter := newIPRateLimiter(1, 1)
	handler := rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), limiter)
	for attempt := range 2 {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = fmt.Sprintf("192.0.2.1:%d", 1234+attempt)
		request.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", attempt+1))
		request.Header.Set("X-Real-IP", fmt.Sprintf("198.51.100.%d", attempt+1))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if attempt == 0 && response.Code != http.StatusNoContent {
			t.Fatal("first request was rejected")
		}
		if attempt == 1 {
			if response.Code != http.StatusTooManyRequests {
				t.Fatalf("header or port change bypassed limit: %d", response.Code)
			}
			var body struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Success || body.Code != "RATE_LIMITED" {
				t.Fatalf("unexpected rate limit response: %s", response.Body.String())
			}
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.2:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatal("second client shared the first client's budget")
	}
}

func TestClientIPNormalizesAddresses(t *testing.T) {
	tests := []struct {
		remote string
		want   string
	}{
		{remote: "192.0.2.1:1234", want: "192.0.2.1"},
		{remote: "[::ffff:192.0.2.1]:4321", want: "192.0.2.1"},
		{remote: "2001:db8::1", want: "2001:db8::1"},
		{remote: "[2001:0db8:0:0:0:0:0:1]:4321", want: "2001:db8::1"},
	}
	for _, tt := range tests {
		t.Run(tt.remote, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = tt.remote
			if got := getClientIP(request); got != tt.want {
				t.Errorf("client IP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRateLimitCleanupPreservesUnrefilledBudget(t *testing.T) {
	limiter := newIPRateLimiter(1, 10)
	bucket := limiter.getLimiter("192.0.2.1")
	if !bucket.AllowN(time.Now(), 10) {
		t.Fatal("initial burst was rejected")
	}
	entry := limiter.ips["192.0.2.1"]
	entry.lastSeen = time.Now().Add(-6 * time.Minute)
	limiter.ips["192.0.2.1"] = entry
	limiter.cleanup()
	if limiter.getLimiter("192.0.2.1").Allow() {
		t.Fatal("cleanup reset an exhausted client's budget")
	}
}

func TestRateLimitCleanupOnlyRemovesIdleClients(t *testing.T) {
	limiter := newIPRateLimiter(60, 2)
	_ = limiter.getLimiter("192.0.2.1")
	active := limiter.getLimiter("192.0.2.2")
	entry := limiter.ips["192.0.2.1"]
	entry.lastSeen = time.Now().Add(-6 * time.Minute)
	limiter.ips["192.0.2.1"] = entry
	limiter.cleanup()
	if _, ok := limiter.ips["192.0.2.1"]; ok {
		t.Error("idle client was not removed")
	}
	if got := limiter.getLimiter("192.0.2.2"); got != active {
		t.Error("active client's budget was replaced")
	}
}

func TestRateLimitConcurrentRequestsShareBurst(t *testing.T) {
	limiter := newIPRateLimiter(1, 2)
	var accepted atomic.Int32
	handler := rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accepted.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}), limiter)
	var requests sync.WaitGroup
	for range 20 {
		requests.Add(1)
		go func() {
			defer requests.Done()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = "192.0.2.1:1234"
			handler.ServeHTTP(httptest.NewRecorder(), request)
		}()
	}
	requests.Wait()
	if got := accepted.Load(); got != 2 {
		t.Errorf("accepted %d concurrent requests, want 2", got)
	}
}
