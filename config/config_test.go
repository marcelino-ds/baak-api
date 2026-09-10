package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestConfigValidatesTrustedProxies(t *testing.T) {
	tests := []struct {
		value string
		valid bool
	}{
		{value: "", valid: true},
		{value: " 127.0.0.1, 10.2.3.0/24, ::1, 2001:db8::/32 ", valid: true},
		{value: "::ffff:192.0.2.0/120", valid: true},
		{value: "*"},
		{value: "proxy.example"},
		{value: "192.0.2.1:8080"},
		{value: "192.0.2.0/99"},
		{value: "fe80::1%eth0"},
		{value: "::ffff:192.0.2.0/80"},
		{value: "192.0.2.0/24,invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			restoreConfig(t)
			t.Setenv("TRUSTED_PROXIES", tt.value)
			t.Setenv("PORT", "8080")
			t.Setenv("BASE_URL", "https://baak.example")
			t.Setenv("FLARESOLVERR_URL", "")
			t.Setenv("HTTP_PROXIES", "")
			LoadConfig()
			err := AppConfig.Validate()
			if (err == nil) != tt.valid {
				t.Fatalf("Validate() = %v, valid = %t", err, tt.valid)
			}
			if err != nil && !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
				t.Fatalf("error does not identify invalid setting: %v", err)
			}
		})
	}
}

func restoreConfig(t *testing.T) {
	t.Helper()
	previous := AppConfig
	t.Cleanup(func() { AppConfig = previous })
}

func TestLoadConfigTrimsAllowedOrigins(t *testing.T) {
	restoreConfig(t)
	t.Setenv("PORT", "")
	t.Setenv("BASE_URL", "")
	t.Setenv("RATE_LIMIT_PER_MIN", "")
	t.Setenv("ALLOWED_ORIGINS", " https://app.example ,https://admin.example ")
	t.Setenv("FLARESOLVERR_URL", "")
	t.Setenv("CACHE_TTL_JADWAL", "")
	t.Setenv("CACHE_TTL_KALENDER", "")
	t.Setenv("CACHE_ENABLED", "")

	LoadConfig()
	want := []string{"https://app.example", "https://admin.example"}
	if len(AppConfig.AllowedOrigins) != len(want) {
		t.Fatalf("AllowedOrigins = %#v, want %#v", AppConfig.AllowedOrigins, want)
	}
	for index, origin := range want {
		if AppConfig.AllowedOrigins[index] != origin {
			t.Errorf("AllowedOrigins[%d] = %q, want %q", index, AppConfig.AllowedOrigins[index], origin)
		}
	}
}

func TestLoadConfigKeepsPositiveRateLimit(t *testing.T) {
	restoreConfig(t)
	t.Setenv("RATE_LIMIT_PER_MIN", "120")
	t.Setenv("RATE_LIMIT_BURST", "4")
	LoadConfig()
	if AppConfig.RateLimitPerMin != 120 {
		t.Fatalf("RateLimitPerMin = %d, want 120", AppConfig.RateLimitPerMin)
	}
	if AppConfig.RateLimitBurst != 4 {
		t.Fatalf("RateLimitBurst = %d, want 4", AppConfig.RateLimitBurst)
	}
}

func TestLoadConfigFallsBackForInvalidRateLimit(t *testing.T) {
	for _, value := range []string{"", "not-a-number", "0", "-1"} {
		t.Run(value, func(t *testing.T) {
			restoreConfig(t)
			t.Setenv("RATE_LIMIT_PER_MIN", value)
			t.Setenv("RATE_LIMIT_BURST", value)
			LoadConfig()
			if AppConfig.RateLimitPerMin != 60 || AppConfig.RateLimitBurst != 10 {
				t.Fatalf("invalid rate config: rate=%d, burst=%d", AppConfig.RateLimitPerMin, AppConfig.RateLimitBurst)
			}
		})
	}
}

func TestLoadConfigFallsBackForInvalidFlareSolverrConcurrency(t *testing.T) {
	for _, value := range []string{"", "not-a-number", "0", "-1"} {
		t.Run(value, func(t *testing.T) {
			restoreConfig(t)
			t.Setenv("FLARESOLVERR_MAX_CONCURRENT", value)
			LoadConfig()
			if AppConfig.FlareSolverrMaxConcurrent != DefaultFlareSolverrMaxConcurrent {
				t.Fatalf("invalid FlareSolverr concurrency = %d, want %d", AppConfig.FlareSolverrMaxConcurrent, DefaultFlareSolverrMaxConcurrent)
			}
		})
	}
}

func TestLoadConfigFiltersEmptyOrigins(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "default", input: "", want: []string{"*"}},
		{name: "empty entries", input: ", https://app.example, ,", want: []string{"https://app.example"}},
		{name: "all empty denies cross origin", input: " , , ", want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreConfig(t)
			t.Setenv("ALLOWED_ORIGINS", tt.input)
			LoadConfig()
			if !reflect.DeepEqual(AppConfig.AllowedOrigins, tt.want) {
				t.Errorf("AllowedOrigins = %#v, want %#v", AppConfig.AllowedOrigins, tt.want)
			}
		})
	}
}

func TestConfigValidateRejectsUnsafeURLsAndPorts(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "invalid port", cfg: Config{Port: ":70000", BaseURL: "https://baak.example"}},
		{name: "credentials", cfg: Config{Port: ":8080", BaseURL: "https://user:pass@baak.example"}},
		{name: "query", cfg: Config{Port: ":8080", BaseURL: "https://baak.example/?token=secret"}},
		{name: "invalid proxy", cfg: Config{Port: ":8080", BaseURL: "https://baak.example", HTTPProxies: []string{"ftp://proxy.example"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Fatal("Validate accepted invalid configuration")
			}
		})
	}
}

func TestConfigValidateAcceptsURLPathAndHTTPProxy(t *testing.T) {
	cfg := Config{
		Port: ":8080", BaseURL: "https://baak.example/portal",
		FlareSolverrURL: "http://127.0.0.1:8191", HTTPProxies: []string{"http://127.0.0.1:3128"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate rejected valid configuration: %v", err)
	}
}
