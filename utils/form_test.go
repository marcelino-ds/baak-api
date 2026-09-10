package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFormFallbackRejectsUnsolvedChallengeAndPreservesPayload(t *testing.T) {
	for _, response := range []string{`{"status":"ok","solution":{"status":200,"response":"<h1>solved</h1>"}}`,
		`{"status":"ok","solution":{"status":200,"response":"<title>Just a moment...</title>"}}`} {
		flare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request FlareSolverrRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			form, err := url.ParseQuery(request.PostData)
			if err != nil || request.Cmd != "request.post" || form.Get("teks") != "A & B" || form.Get("_token") != "fixture" {
				t.Errorf("invalid form solver request: %+v", request)
			}
			fmt.Fprint(w, response)
		}))
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
		scraper := &Scraper{BaseURL: upstream.URL, client: upstream.Client(), breaker: NewCircuitBreaker(), flareBreaker: NewCircuitBreaker(), flareSolverr: &FlareSolverr{url: flare.URL, client: flare.Client()}}
		doc, err := scraper.FetchFormDocument(context.Background(), upstream.URL, url.Values{"teks": {"A & B"}, "_token": {"fixture"}})
		if strings.Contains(response, "Just a moment") {
			if !errors.Is(err, ErrCloudflareBlocked) {
				t.Errorf("unsolved POST challenge accepted: %v", err)
			}
		} else if err != nil || doc.Text() != "solved" {
			t.Errorf("POST fallback: %v", err)
		}
		upstream.Close()
		flare.Close()
	}
}

func TestFormFallbackKeepsTypedSolverFailure(t *testing.T) {
	flare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{`) }))
	defer flare.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer upstream.Close()
	scraper := &Scraper{BaseURL: upstream.URL, client: upstream.Client(), breaker: NewCircuitBreaker(), flareBreaker: NewCircuitBreaker(), flareSolverr: &FlareSolverr{url: flare.URL, client: flare.Client()}}
	_, err := scraper.FetchFormDocument(context.Background(), upstream.URL, url.Values{"teks": {"1IA01"}})
	if !errors.Is(err, ErrFlareSolverr) {
		t.Fatalf("missing solver error type: %v", err)
	}
}

func TestFetchFormDocumentPostsFormData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/jadwal/cariUas" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("teks") != "1IA01" || r.Form.Get("_token") != "fixture" {
			t.Fatalf("form = %#v, err = %v", r.Form, err)
		}
		_, _ = w.Write([]byte(`<table><tr><th>Hari</th><th>Tanggal</th></tr><tr><td>Selasa</td><td>28 Juli 2026</td></tr></table>`))
	}))
	defer server.Close()
	scraper := &Scraper{BaseURL: server.URL, client: server.Client(), breaker: NewCircuitBreaker()}
	doc, err := scraper.FetchFormDocument(context.Background(), server.URL+"/jadwal/cariUas", url.Values{"_token": {"fixture"}, "teks": {"1IA01"}})
	if err != nil || !strings.Contains(doc.Text(), "28 Juli 2026") {
		t.Fatalf("form document = %v, %v", doc, err)
	}
}
