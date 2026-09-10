package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	RequestTimeout                   = 50 * time.Second
	HealthTimeout                    = 3 * time.Second
	ShutdownTimeout                  = 55 * time.Second
	DefaultFlareSolverrMaxConcurrent = 2
)

type Config struct {
	Port                      string
	BaseURL                   string
	RateLimitPerMin           int
	RateLimitBurst            int
	AllowedOrigins            []string
	FlareSolverrURL           string
	CacheTTLJadwal            time.Duration
	CacheTTLKalender          time.Duration
	CacheEnabled              bool
	CacheMaxEntries           int
	HTTPProxies               []string
	TrustedProxies            []string
	FlareSolverrMaxConcurrent int
}

var AppConfig Config

func LoadConfig() {
	port := strings.TrimSpace(getEnvOrDefault("PORT", "8080"))
	// Ensure port has colon prefix for http.ListenAndServe
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	AppConfig = Config{
		Port:                      port,
		BaseURL:                   strings.TrimRight(strings.TrimSpace(getEnvOrDefault("BASE_URL", "https://baak.gunadarma.ac.id")), "/"),
		RateLimitPerMin:           getPositiveIntOrDefault("RATE_LIMIT_PER_MIN", 60),
		RateLimitBurst:            getPositiveIntOrDefault("RATE_LIMIT_BURST", 10),
		AllowedOrigins:            getEnvSliceOrDefault("ALLOWED_ORIGINS", []string{"*"}),
		FlareSolverrURL:           strings.TrimRight(strings.TrimSpace(os.Getenv("FLARESOLVERR_URL")), "/"),
		CacheTTLJadwal:            getTTLOrDefault("CACHE_TTL_JADWAL", 300),
		CacheTTLKalender:          getTTLOrDefault("CACHE_TTL_KALENDER", 3600),
		CacheEnabled:              getEnvBoolOrDefault("CACHE_ENABLED", true),
		CacheMaxEntries:           getPositiveIntOrDefault("CACHE_MAX_ENTRIES", 1024),
		HTTPProxies:               getEnvSliceOrDefault("HTTP_PROXIES", nil),
		TrustedProxies:            getEnvSliceOrDefault("TRUSTED_PROXIES", nil),
		FlareSolverrMaxConcurrent: getPositiveIntOrDefault("FLARESOLVERR_MAX_CONCURRENT", DefaultFlareSolverrMaxConcurrent),
	}
}

// Validate rejects unusable connection settings before accepting requests.
func (c Config) Validate() error {
	port, err := strconv.Atoi(strings.TrimPrefix(c.Port, ":"))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("PORT must be an integer between 1 and 65535")
	}
	if err := ValidateBaseURL(c.BaseURL); err != nil {
		return fmt.Errorf("BASE_URL: %w", err)
	}
	if c.FlareSolverrURL != "" {
		if err := ValidateBaseURL(c.FlareSolverrURL); err != nil {
			return fmt.Errorf("FLARESOLVERR_URL: %w", err)
		}
	}
	for _, proxy := range c.HTTPProxies {
		u, err := url.Parse(proxy)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("HTTP_PROXIES contains an invalid proxy URL")
		}
	}
	if _, err := c.TrustedProxyPrefixes(); err != nil {
		return fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}
	return nil
}

// TrustedProxyPrefixes parses the proxy addresses allowed to provide client IP headers.
func (c Config) TrustedProxyPrefixes() ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(c.TrustedProxies))
	for _, value := range c.TrustedProxies {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		var prefix netip.Prefix
		if parsed, err := netip.ParsePrefix(value); err == nil {
			prefix = parsed
		} else if address, addressErr := netip.ParseAddr(value); addressErr == nil {
			if address.Zone() != "" {
				return nil, fmt.Errorf("%q must not contain an IPv6 zone", value)
			}
			prefix = netip.PrefixFrom(address.Unmap(), address.Unmap().BitLen())
		} else {
			return nil, fmt.Errorf("%q must be an IP address or CIDR", value)
		}

		address := prefix.Addr()
		if !address.IsValid() || address.Zone() != "" {
			return nil, fmt.Errorf("%q must be a valid IP address or CIDR", value)
		}
		if address.Is4In6() {
			bits := prefix.Bits()
			if bits < 96 {
				return nil, fmt.Errorf("%q has an invalid IPv4-mapped prefix length", value)
			}
			prefix = netip.PrefixFrom(address.Unmap(), bits-96)
		} else {
			prefix = netip.PrefixFrom(address, prefix.Bits())
		}
		if prefix.Bits() == 0 {
			return nil, fmt.Errorf("%q must not trust every address", value)
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

func ValidateBaseURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("must be an absolute HTTP or HTTPS URL")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return fmt.Errorf("must not contain credentials, a query, or a fragment")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("URL port must be between 1 and 65535")
		}
	}
	return nil
}

func getTTLOrDefault(key string, fallback int) time.Duration {
	seconds := getEnvIntOrDefault(key, fallback)
	if seconds <= 0 {
		return 0
	}
	if int64(seconds) > int64((1<<63-1)/time.Second) {
		seconds = fallback
	}
	return time.Duration(seconds) * time.Second
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
