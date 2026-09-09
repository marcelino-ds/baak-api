package config

import (
	"reflect"
	"testing"
)

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
