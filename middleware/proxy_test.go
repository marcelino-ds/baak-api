package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yafyx/baak-api/config"
)

func TestTrustedProxyUsesNearestUntrustedAddress(t *testing.T) {
	previous := config.AppConfig
	config.AppConfig.TrustedProxies = []string{"192.0.2.0/24", "10.0.0.0/24"}
	t.Cleanup(func() { config.AppConfig = previous })
	resolver := newClientIPResolver()
	tests := []struct {
		name      string
		forwarded []string
		realIP    []string
		want      string
	}{
		{
			name:      "ignore unverified garbage left of the client",
			forwarded: []string{"unknown, 203.0.113.1, 10.0.0.8"}, want: "203.0.113.1",
		},
		{
			name:      "multiple header lines retain hop order",
			forwarded: []string{"198.51.100.9", "203.0.113.1, 10.0.0.8"}, want: "203.0.113.1",
		},
		{
			name:      "forwarded header takes precedence over real IP",
			forwarded: []string{"203.0.113.1"}, realIP: []string{"198.51.100.1"}, want: "203.0.113.1",
		},
		{
			name:      "empty forwarded header cannot enable real IP fallback",
			forwarded: []string{""}, realIP: []string{"198.51.100.1"}, want: "192.0.2.10",
		},
		{
			name:      "malformed nearest hop uses socket peer",
			forwarded: []string{"203.0.113.1, unknown"}, want: "192.0.2.10",
		},
		{
			name:   "duplicate real IP headers use socket peer",
			realIP: []string{"203.0.113.1", "203.0.113.2"}, want: "192.0.2.10",
		},
		{
			name:      "excessive hop count uses socket peer",
			forwarded: []string{strings.Repeat("10.0.0.8,", 64) + "203.0.113.1"}, want: "192.0.2.10",
		},
		{
			name:      "IPv4 mapped header addresses are normalized",
			forwarded: []string{"::ffff:203.0.113.1"}, want: "203.0.113.1",
		},
		{
			name:      "IPv6 zones cannot become separate identities",
			forwarded: []string{"fe80::1%zone"}, want: "192.0.2.10",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = "192.0.2.10:1234"
			for _, value := range tt.forwarded {
				request.Header.Add("X-Forwarded-For", value)
			}
			for _, value := range tt.realIP {
				request.Header.Add("X-Real-IP", value)
			}
			if got := resolver.resolve(request); got != tt.want {
				t.Fatalf("client IP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRateLimitBehindTrustedProxies(t *testing.T) {
	tests := []struct {
		name      string
		trusted   string
		remote    string
		forwarded []string
		realIP    []string
		want      []int
	}{
		{
			name: "separate clients share a trusted socket peer", trusted: "192.0.2.10",
			remote: "192.0.2.10:1234", forwarded: []string{"203.0.113.1", "203.0.113.2", "203.0.113.1"},
			want: []int{204, 204, 429},
		},
		{
			name: "untrusted peer cannot spoof identity", trusted: "192.0.2.10",
			remote: "192.0.2.11:1234", forwarded: []string{"203.0.113.1", "203.0.113.2"},
			want: []int{204, 429},
		},
		{
			name: "forwarding disabled by default", remote: "192.0.2.10:1234",
			forwarded: []string{"203.0.113.1", "203.0.113.2"}, want: []int{204, 429},
		},
		{
			name: "walk trusted hops from the right", trusted: "192.0.2.0/24,10.0.0.0/24",
			remote:    "192.0.2.10:1234",
			forwarded: []string{"198.51.100.1, 203.0.113.1, 10.0.0.8", "198.51.100.2, 203.0.113.1, 10.0.0.9"},
			want:      []int{204, 429},
		},
		{
			name: "real IP from trusted peer", trusted: "192.0.2.10", remote: "192.0.2.10:1234",
			realIP: []string{"203.0.113.1", "203.0.113.2"}, want: []int{204, 204},
		},
		{
			name: "malformed forwarding does not fall back to real IP", trusted: "192.0.2.10",
			remote: "192.0.2.10:1234", forwarded: []string{"unknown", "203.0.113.1,"},
			realIP: []string{"203.0.113.1", "203.0.113.2"}, want: []int{204, 429},
		},
		{
			name: "IPv4 mapped client cannot reset budget", trusted: "::ffff:192.0.2.0/120",
			remote: "[::ffff:192.0.2.10]:1234", forwarded: []string{"::ffff:203.0.113.1", "203.0.113.1"},
			want: []int{204, 429},
		},
		{
			name: "IPv6 proxy and clients", trusted: "2001:db8:1::/48", remote: "[2001:db8:1::1]:1234",
			forwarded: []string{"2001:db8:2::1", "2001:db8:2::2"}, want: []int{204, 204},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := config.AppConfig
			t.Cleanup(func() { config.AppConfig = previous })
			t.Setenv("TRUSTED_PROXIES", tt.trusted)
			config.LoadConfig()
			handler := rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}), newIPRateLimiter(1, 1))
			for i, want := range tt.want {
				request := httptest.NewRequest(http.MethodGet, "/", nil)
				request.RemoteAddr = tt.remote
				if len(tt.forwarded) > 0 {
					request.Header.Set("X-Forwarded-For", tt.forwarded[i])
				}
				if len(tt.realIP) > 0 {
					request.Header.Set("X-Real-IP", tt.realIP[i])
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != want {
					t.Errorf("request %d status = %d, want %d", i, response.Code, want)
				}
			}
		})
	}
}
