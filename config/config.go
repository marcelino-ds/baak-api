package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port             string
	BaseURL          string
	RateLimitPerMin  int
	RateLimitBurst   int
	AllowedOrigins   []string
	FlareSolverrURL  string
	CacheTTLJadwal   time.Duration
	CacheTTLKalender time.Duration
	CacheEnabled     bool
}

var AppConfig Config

func LoadConfig() {
	port := getEnvOrDefault("PORT", "8080")
	// Ensure port has colon prefix for http.ListenAndServe
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	AppConfig = Config{
		Port:             port,
		BaseURL:          getEnvOrDefault("BASE_URL", "https://baak.gunadarma.ac.id"),
		RateLimitPerMin:  getPositiveIntOrDefault("RATE_LIMIT_PER_MIN", 60),
		RateLimitBurst:   getPositiveIntOrDefault("RATE_LIMIT_BURST", 10),
		AllowedOrigins:   getEnvSliceOrDefault("ALLOWED_ORIGINS", []string{"*"}),
		FlareSolverrURL:  os.Getenv("FLARESOLVERR_URL"),
		CacheTTLJadwal:   time.Duration(getEnvIntOrDefault("CACHE_TTL_JADWAL", 300)) * time.Second,
		CacheTTLKalender: time.Duration(getEnvIntOrDefault("CACHE_TTL_KALENDER", 3600)) * time.Second,
		CacheEnabled:     getEnvBoolOrDefault("CACHE_ENABLED", true),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getPositiveIntOrDefault(key string, defaultValue int) int {
	value := getEnvIntOrDefault(key, defaultValue)
	if value <= 0 {
		return defaultValue
	}
	return value
}

func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvSliceOrDefault(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		origins := make([]string, 0)
		for _, origin := range strings.Split(value, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				origins = append(origins, origin)
			}
		}
		return origins
	}
	return defaultValue
}
