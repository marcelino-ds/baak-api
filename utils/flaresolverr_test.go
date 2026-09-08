package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
