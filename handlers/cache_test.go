package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/utils"
)

type cacheUpstream struct {
	*httptest.Server
	requests atomic.Int64
	blocked  atomic.Bool
	revision atomic.Int64
}

func newCacheUpstream(t *testing.T) *cacheUpstream {
	t.Helper()
	upstream := &cacheUpstream{}
	upstream.revision.Store(1)
	upstream.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstream.requests.Add(1)
		if upstream.blocked.Load() {
			http.Error(w, "unavailable", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/jadwal":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "fixture", Path: "/"})
			_, _ = fmt.Fprint(w, `<input name="_token" value="fixture-token">`)
		case "/jadwal/cariJadKul":
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value != "fixture" || r.URL.Query().Get("_token") != "fixture-token" {
				http.Error(w, "invalid session", http.StatusForbidden)
				return
			}
			_, _ = fmt.Fprintf(w,
				`<table><tr><td>1</td><td>Senin</td><td>Algoritma %s v%d</td>`+
					`<td>1</td><td>D201</td><td>Dr. Ada</td></tr></table>`,
				html.EscapeString(r.URL.Query().Get("teks")), upstream.revision.Load(),
			)
		case "/kuliahUjian/6":
			_, _ = fmt.Fprint(w, `<table class="cell-xs-6"><tr><td>1</td><td>07.30 - 08.20</td></tr></table>`)
		case "/":
			_, _ = fmt.Fprintf(w, `<table><tr><td>Perkuliahan v%d</td><td>9 September 2026</td></tr></table>`,
				upstream.revision.Load())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	return upstream
}

func configureCacheTest(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("FLARESOLVERR_URL", "")
	t.Setenv("HTTP_PROXIES", "")
	previousConfig := config.AppConfig
	config.AppConfig = config.Config{
		BaseURL:          baseURL,
		CacheEnabled:     true,
		CacheTTLJadwal:   time.Minute,
		CacheTTLKalender: time.Minute,
	}
	utils.GetCache().Clear()
	utils.GetCircuitBreaker().Reset()
	t.Cleanup(func() {
		config.AppConfig = previousConfig
		utils.GetCache().Clear()
		utils.GetCircuitBreaker().Reset()
	})
}

type cachedEndpoint struct {
	name    string
	path    string
	handler http.HandlerFunc
	loads   int64
}

var cachedEndpoints = []cachedEndpoint{
	{name: "jadwal path", path: "/jadwal/1IA01", handler: HandlerJadwal, loads: 3},
	{name: "jadwal query", path: "/jadwal?q=1IA01", handler: HandlerJadwalSearch, loads: 3},
	{name: "kalender", path: "/kalender", handler: HandlerKegiatan, loads: 1},
}

func (endpoint cachedEndpoint) get(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	endpoint.handler(response, httptest.NewRequest(http.MethodGet, endpoint.path, nil))
	return response
}

func TestEndpointCacheServesFreshDataWithoutUpstream(t *testing.T) {
	for _, endpoint := range cachedEndpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			upstream := newCacheUpstream(t)
			configureCacheTest(t, upstream.URL)
			first := endpoint.get(t)
			if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), "v1") {
				t.Fatalf("first response: %d %s", first.Code, first.Body.String())
			}

			upstream.blocked.Store(true)
			second := endpoint.get(t)
			if second.Code != http.StatusOK || second.Body.String() != first.Body.String() {
				t.Fatalf("cached response: %d %s", second.Code, second.Body.String())
			}
			if got := upstream.requests.Load(); got != endpoint.loads {
				t.Errorf("upstream requests = %d, want %d for a single load", got, endpoint.loads)
			}
			if got := utils.GetCache().Stats().ValidItems; got != 1 {
				t.Errorf("valid cached results = %d, want 1", got)
			}
		})
	}
}

func TestJadwalCacheSharedBetweenRoutes(t *testing.T) {
	for _, firstQuery := range []bool{false, true} {
		t.Run(fmt.Sprintf("query first=%t", firstQuery), func(t *testing.T) {
			upstream := newCacheUpstream(t)
			configureCacheTest(t, upstream.URL)
			path, query := cachedEndpoints[0], cachedEndpoints[1]
			first, second := path, query
			if firstQuery {
				first, second = query, path
			}
			if response := first.get(t); response.Code != http.StatusOK {
				t.Fatalf("first response: %d %s", response.Code, response.Body.String())
			}
			upstream.blocked.Store(true)
			response := second.get(t)
			if response.Code != http.StatusOK {
				t.Fatalf("cached response: %d %s", response.Code, response.Body.String())
			}
			var envelope struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			field, absent := "query", "kelas"
			if firstQuery {
				field, absent = "kelas", "query"
			}
			if string(envelope.Data[field]) != `"1IA01"` || envelope.Data[absent] != nil {
				t.Fatalf("wrong route envelope: %s", response.Body.String())
			}
			if got := upstream.requests.Load(); got != 3 {
				t.Errorf("upstream requests = %d, want 3 across both routes", got)
			}
		})
	}
}

func TestEndpointCacheRefreshesAfterExpiry(t *testing.T) {
	for _, endpoint := range cachedEndpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			upstream := newCacheUpstream(t)
			configureCacheTest(t, upstream.URL)
			const ttl = 20 * time.Millisecond
			config.AppConfig.CacheTTLJadwal = ttl
			config.AppConfig.CacheTTLKalender = ttl
			if response := endpoint.get(t); response.Code != http.StatusOK {
				t.Fatalf("first response: %s", response.Body.String())
			}
			upstream.revision.Store(2)
			time.Sleep(2 * ttl)
			response := endpoint.get(t)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "v2") {
				t.Fatalf("refreshed response: %d %s", response.Code, response.Body.String())
			}
			if got := upstream.requests.Load(); got != 2*endpoint.loads {
				t.Errorf("upstream requests = %d, want %d for two loads", got, 2*endpoint.loads)
			}
		})
	}
}

func TestEndpointCacheBypass(t *testing.T) {
	modes := []struct {
		name  string
		apply func()
	}{
		{name: "disabled", apply: func() { config.AppConfig.CacheEnabled = false }},
		{name: "zero TTL", apply: func() {
			config.AppConfig.CacheTTLJadwal = 0
			config.AppConfig.CacheTTLKalender = 0
		}},
		{name: "negative TTL", apply: func() {
			config.AppConfig.CacheTTLJadwal = -time.Second
			config.AppConfig.CacheTTLKalender = -time.Second
		}},
	}
	for _, endpoint := range cachedEndpoints {
		for _, mode := range modes {
			t.Run(endpoint.name+"/"+mode.name, func(t *testing.T) {
				upstream := newCacheUpstream(t)
				configureCacheTest(t, upstream.URL)
				if response := endpoint.get(t); response.Code != http.StatusOK {
					t.Fatalf("first response: %s", response.Body.String())
				}
				mode.apply()
				upstream.revision.Store(2)
				response := endpoint.get(t)
				if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "v2") {
					t.Fatalf("bypassed response: %d %s", response.Code, response.Body.String())
				}
				utils.GetCache().Clear()
				endpoint.get(t)
				if got := utils.GetCache().Stats().TotalItems; got != 0 {
					t.Errorf("bypassed request stored %d cache entries", got)
				}
			})
		}
	}
}

func TestEndpointCacheDoesNotStoreFailures(t *testing.T) {
	for _, endpoint := range cachedEndpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			upstream := newCacheUpstream(t)
			configureCacheTest(t, upstream.URL)
			upstream.blocked.Store(true)
			if response := endpoint.get(t); response.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected upstream failure: %d %s", response.Code, response.Body.String())
			}
			if got := utils.GetCache().Stats().TotalItems; got != 0 {
				t.Errorf("failed request stored %d cache entries", got)
			}
			upstream.blocked.Store(false)
			if response := endpoint.get(t); response.Code != http.StatusOK {
				t.Fatalf("recovery response: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestEndpointCacheSeparatesBaseURLs(t *testing.T) {
	for _, endpoint := range cachedEndpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			first, second := newCacheUpstream(t), newCacheUpstream(t)
			configureCacheTest(t, first.URL)
			endpoint.get(t)
			config.AppConfig.BaseURL = second.URL
			second.revision.Store(2)
			response := endpoint.get(t)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "v2") {
				t.Fatalf("different upstream response: %d %s", response.Code, response.Body.String())
			}
			if second.requests.Load() != endpoint.loads {
				t.Fatal("different upstream was not fetched")
			}
		})
	}
}

func TestJadwalCacheSeparatesSearchTerms(t *testing.T) {
	upstream := newCacheUpstream(t)
	configureCacheTest(t, upstream.URL)
	cachedEndpoints[0].get(t)
	other := cachedEndpoint{path: "/jadwal?q=2IA02", handler: HandlerJadwalSearch}
	response := other.get(t)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Algoritma 2IA02") {
		t.Fatalf("different search response: %d %s", response.Code, response.Body.String())
	}
	if got := upstream.requests.Load(); got != 6 {
		t.Errorf("upstream requests = %d, want 6 for different searches", got)
	}
}

func TestEndpointCacheUsesIndependentTTLs(t *testing.T) {
	for _, expireSchedule := range []bool{false, true} {
		t.Run(fmt.Sprintf("expire schedule=%t", expireSchedule), func(t *testing.T) {
			upstream := newCacheUpstream(t)
			configureCacheTest(t, upstream.URL)
			const ttl = 20 * time.Millisecond
			expiring, fresh := cachedEndpoints[2], cachedEndpoints[0]
			config.AppConfig.CacheTTLKalender = ttl
			if expireSchedule {
				expiring, fresh = fresh, expiring
				config.AppConfig.CacheTTLJadwal = ttl
				config.AppConfig.CacheTTLKalender = time.Minute
			}
			expiring.get(t)
			fresh.get(t)
			upstream.revision.Store(2)
			time.Sleep(2 * ttl)
			if response := fresh.get(t); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "v1") {
				t.Fatalf("unexpired result changed: %d %s", response.Code, response.Body.String())
			}
			if response := expiring.get(t); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "v2") {
				t.Fatalf("expired result not refreshed: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestEndpointCacheStillValidatesRequests(t *testing.T) {
	upstream := newCacheUpstream(t)
	configureCacheTest(t, upstream.URL)
	for _, endpoint := range cachedEndpoints {
		endpoint.get(t)
		response := httptest.NewRecorder()
		endpoint.handler(response, httptest.NewRequest(http.MethodPost, endpoint.path, nil))
		if response.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s accepted POST with populated cache: %d", endpoint.name, response.Code)
		}
	}
	before := upstream.requests.Load()
	for _, endpoint := range []cachedEndpoint{
		{path: "/jadwal/ab", handler: HandlerJadwal},
		{path: "/jadwal?q=ab", handler: HandlerJadwalSearch},
	} {
		if response := endpoint.get(t); response.Code != http.StatusBadRequest {
			t.Errorf("invalid search response: %d %s", response.Code, response.Body.String())
		}
	}
	if upstream.requests.Load() != before {
		t.Error("invalid search reached upstream")
	}
}

func TestCachedValueSharesConcurrentLoads(t *testing.T) {
	configureCacheTest(t, "https://baak.example")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var loads atomic.Int64
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	load := func(ctx context.Context) (string, error) {
		loads.Add(1)
		started <- struct{}{}
		select {
		case <-release:
			return "academic data", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	type result struct {
		value string
		err   error
	}
	results := make(chan result, 8)
	for range 8 {
		go func() {
			value, err := cachedValue(ctx, true, time.Minute, "concurrent", load)
			results <- result{value: value, err: err}
		}()
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("loader did not start")
	}
	// Hold the first load open while the other callers reach the cache.
	select {
	case <-started:
		t.Error("a second loader started for the same cache key")
	case <-time.After(50 * time.Millisecond):
	}
	unblock()
	for range 8 {
		select {
		case got := <-results:
			if got.err != nil || got.value != "academic data" {
				t.Errorf("shared result = %q, %v", got.value, got.err)
			}
		case <-ctx.Done():
			t.Fatal("concurrent requests did not complete")
		}
	}
	if got := loads.Load(); got != 1 {
		t.Errorf("loader executions = %d, want 1", got)
	}
}

func TestCachedValueCanceledRequestDoesNotReadOrPopulateCache(t *testing.T) {
	configureCacheTest(t, "https://baak.example")
	utils.GetCache().Set("existing", "stale request", time.Minute)
	for _, key := range []string{"existing", "missing"} {
		t.Run(key, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			value, err := cachedValue(ctx, true, time.Minute, key, func(context.Context) (string, error) {
				t.Error("loader ran for a canceled request")
				return "unwanted", nil
			})
			if !errors.Is(err, context.Canceled) || value != "" {
				t.Errorf("canceled result = %q, %v", value, err)
			}
		})
	}
}
