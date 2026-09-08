package utils

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestScraperHonorsCanceledContext(t *testing.T) {
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	scraper := &Scraper{
		BaseURL: "https://baak.example",
		client:  &http.Client{Transport: transport},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := scraper.FetchDocument(ctx, "https://baak.example/jadwal")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("FetchDocument error = %v, want context deadline exceeded", err)
	}
}

func TestScraperPreservesCookiesWhenFallingBackToFlareSolverr(t *testing.T) {
	var flareRequest string
	flare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		flareRequest = string(body)
		_, _ = w.Write([]byte(`{"status":"ok","solution":{"status":200,"response":"<html>solved</html>"}}`))
	}))
	defer flare.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "fixture", Path: "/"})
		w.WriteHeader(http.StatusForbidden)
	}))
	defer upstream.Close()

	scraper := &Scraper{
		BaseURL:      upstream.URL,
		client:       upstream.Client(),
		flareSolverr: &FlareSolverr{url: flare.URL, client: flare.Client()},
	}

	if _, err := scraper.FetchDocument(context.Background(), upstream.URL+"/jadwal"); err != nil {
		t.Fatalf("FetchDocument: %v", err)
	}
	if !strings.Contains(flareRequest, `"name":"session"`) || !strings.Contains(flareRequest, `"value":"fixture"`) {
		t.Fatalf("FlareSolverr request did not preserve upstream cookie: %s", flareRequest)
	}
}

func TestConvertWaktuToJam(t *testing.T) {
	periods := [][]string{
		{"07:30", "08:20"},
		{"08:30", "09:20"},
		{"09:30", "10:20"},
		{"10:30", "11:20"},
	}
	tests := []struct {
		name  string
		waktu string
		want  string
	}{
		{name: "single period", waktu: "2", want: "08:30 - 09:20"},
		{name: "range", waktu: "1-3", want: "07:30 - 10:20"},
		{name: "four periods", waktu: "1/2/3/4", want: "07:30 - 11:20"},
		{name: "reversed range", waktu: "3-1", want: ""},
		{name: "out of bounds", waktu: "1-5", want: ""},
		{name: "unavailable", waktu: "-", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convertWaktuToJam(tt.waktu, periods); got != tt.want {
				t.Errorf("convertWaktuToJam(%q) = %q, want %q", tt.waktu, got, tt.want)
			}
		})
	}
}

func TestFetchRejectsCloudflareChallengeWithOKStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><title>Just a moment...</title><div id="cf-challenge"></div></html>`))
	}))
	defer upstream.Close()

	if _, err := FetchDocumentWithRetry(upstream.URL, upstream.URL, 1); err == nil {
		t.Fatal("Cloudflare challenge was accepted as a successful document")
	}
}

func TestFetchRejectsUntrustedTLSCertificate(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><title>Untrusted</title></html>"))
	}))
	defer upstream.Close()

	if _, err := FetchDocumentWithRetry(upstream.URL, upstream.URL, 1); err == nil {
		t.Fatal("document fetched using an untrusted TLS certificate")
	}
}
