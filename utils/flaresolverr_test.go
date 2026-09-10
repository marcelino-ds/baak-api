package utils

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yafyx/baak-api/config"
)

func TestFlareSolverrAcceptsFractionalCookieExpiry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","solution":{"status":200,"response":"<html>Schedule</html>","cookies":[{"name":"session","value":"fixture","domain":"baak.example","path":"/","expires":1810000000.25,"httpOnly":true,"secure":true}],"userAgent":"fixture-browser"}}`))
	}))
	defer server.Close()
	fs := &FlareSolverr{url: server.URL, client: server.Client()}

	html, err := fs.Fetch("https://baak.example/jadwal")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if html != "<html>Schedule</html>" {
		t.Fatalf("unexpected document: %q", html)
	}
}

func TestFlareSolverrRejectsHTMLFromExternalRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","solution":{"url":"http://127.0.0.1/private","status":200,"response":"<html>private</html>"}}`))
	}))
	defer server.Close()
	fs := &FlareSolverr{url: server.URL, client: server.Client()}
	if _, err := fs.Fetch("https://baak.example/buku_pedoman"); !errors.Is(err, ErrFlareSolverr) {
		t.Fatalf("external solver result accepted: %v", err)
	}
}

func TestFlareSolverrLimitsConcurrentRequests(t *testing.T) {
	tests := []struct {
		value string
		limit int
	}{
		{value: "", limit: 2},
		{value: "1", limit: 1},
		{value: "3", limit: 3},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			previousConfig := config.AppConfig
			t.Cleanup(func() { config.AppConfig = previousConfig })
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var active, maxActive atomic.Int32
			started := make(chan struct{}, tt.limit+2)
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusOK)
					return
				}
				current := active.Add(1)
				for {
					previous := maxActive.Load()
					if current <= previous || maxActive.CompareAndSwap(previous, current) {
						break
					}
				}
				started <- struct{}{}
				<-release
				active.Add(-1)
				_, _ = w.Write([]byte(`{"status":"ok","solution":{"status":200,"response":"<html>solved</html>"}}`))
			}))
			defer server.Close()
			var calls sync.WaitGroup
			defer func() {
				cancel()
				unblock()
				calls.Wait()
			}()
			t.Setenv("FLARESOLVERR_URL", server.URL)
			t.Setenv("FLARESOLVERR_MAX_CONCURRENT", tt.value)
			config.LoadConfig()
			results := make(chan error, tt.limit+2)
			for range tt.limit + 2 {
				fs := GetFlareSolverr()
				calls.Add(1)
				go func() {
					defer calls.Done()
					_, err := fs.FetchContext(ctx, "https://baak.example", nil)
					results <- err
				}()
			}
			for range tt.limit {
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatalf("configured concurrency %d was not reached", tt.limit)
				}
			}
			for range 2 {
				select {
				case err := <-results:
					if !errors.Is(err, ErrFlareSolverrBusy) {
						t.Fatalf("extra request = %v, want immediate capacity rejection", err)
					}
				case <-started:
					t.Fatal("FlareSolverr concurrency limit was exceeded")
				case <-ctx.Done():
					t.Fatal("extra requests queued instead of failing fast")
				}
			}
			if err := GetFlareSolverr().CheckHealthContext(ctx); err != nil {
				t.Fatalf("health check blocked by occupied solver slots: %v", err)
			}
			unblock()
			calls.Wait()
			for range tt.limit {
				if err := <-results; err != nil {
					t.Errorf("accepted request failed: %v", err)
				}
			}
			if _, err := GetFlareSolverr().FetchContext(ctx, "https://baak.example", nil); err != nil {
				t.Fatalf("completed requests did not release capacity: %v", err)
			}
			if got := maxActive.Load(); got != int32(tt.limit) {
				t.Fatalf("maximum concurrent requests = %d, want %d", got, tt.limit)
			}
		})
	}
}

func TestFlareSolverrRejectsFailedSolution(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":   "ok",
					"solution": map[string]any{"status": status, "response": "<html>Error</html>"},
				})
			}))
			defer server.Close()
			fs := &FlareSolverr{url: server.URL, client: server.Client()}

			if _, err := fs.Fetch("https://baak.example/jadwal"); err == nil {
				t.Fatalf("accepted solution with status %d", status)
			}
		})
	}
}
