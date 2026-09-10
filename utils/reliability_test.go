package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yafyx/baak-api/config"
)

func TestFlareSolverrOutageIsBoundedAcrossRequests(t *testing.T) {
	var directCalls, flareCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1" {
			flareCalls.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		directCalls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	previousConfig := config.AppConfig
	config.AppConfig.FlareSolverrURL = server.URL
	GetCircuitBreaker().Reset()
	GetFlareCircuitBreaker().Reset()
	t.Cleanup(func() {
		config.AppConfig = previousConfig
		GetCircuitBreaker().Reset()
		GetFlareCircuitBreaker().Reset()
	})

	for attempt := 0; attempt < 8; attempt++ {
		scraper, err := NewScraper(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, err = scraper.FetchDocument(context.Background(), server.URL+"/target")
		if attempt < 5 && !errors.Is(err, ErrFlareSolverr) {
			t.Fatalf("request %d error = %v, want FlareSolverr failure", attempt, err)
		}
		if attempt >= 5 && !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("request %d error = %v, want open circuit", attempt, err)
		}
	}
	if directCalls.Load() != 5 || flareCalls.Load() != 5 {
		t.Fatalf("outage still sent requests: direct=%d, FlareSolverr=%d", directCalls.Load(), flareCalls.Load())
	}
}

func TestOpenCircuitUsesFlareSolverrFallback(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/v1" {
			fmt.Fprint(w, `{"status":"ok","solution":{"status":200,"response":"<html>fallback</html>"}}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	cb := &CircuitBreaker{state: CircuitOpen, timeout: time.Hour, lastFailureTime: time.Now()}
	scraper := &Scraper{client: server.Client(), breaker: cb, flareSolverr: &FlareSolverr{url: server.URL, client: server.Client()}}
	doc, err := scraper.FetchDocument(context.Background(), server.URL+"/target")
	if err != nil || doc.Text() != "fallback" || requests.Load() != 1 {
		t.Fatalf("fallback failed: %v, calls=%d", err, requests.Load())
	}
}

func TestPermanentHTTPFailureIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(http.StatusNotFound) }))
	defer server.Close()
	scraper := &Scraper{client: server.Client()}
	_, err := scraper.fetchWithRetry(context.Background(), server.URL, 3)
	if err == nil || calls.Load() != 1 {
		t.Fatalf("404 requests=%d, err=%v", calls.Load(), err)
	}
}

func TestPaginatedParsersBoundRunawayPagination(t *testing.T) {
	for _, student := range []bool{false, true} {
		t.Run(fmt.Sprint(student), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if len(r.URL.Query()["page"]) != 1 || r.URL.Query().Get("teks") != "A & B" {
					t.Error("pagination corrupted query")
				}
				row := "<td>1</td><td>123</td><td>Name</td><td>Class</td><td>New class</td>"
				if student {
					row += "<td>Note</td>"
				}
				fmt.Fprint(w, "<table><tr>"+row+"</tr></table><a rel=\"next\">Next</a>")
			}))
			defer server.Close()
			scraper := &Scraper{client: server.Client(), breaker: &CircuitBreaker{failureThreshold: 5}}
			target := server.URL + "?teks=A+%26+B&page=8"
			var err error
			if student {
				_, err = scraper.GetMahasiswaBaru(context.Background(), target)
			} else {
				_, err = scraper.GetKelasbaru(context.Background(), target)
			}
			if !errors.Is(err, ErrUnexpectedPage) || calls.Load() != maxResultPages {
				t.Fatalf("pagination was not bounded: calls=%d, err=%v", calls.Load(), err)
			}
		})
	}
}

func TestFlareSolverrRejectsHTTPFailureWithSuccessfulJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"status":"ok","solution":{"status":200,"response":"<html>wrong</html>"}}`)
	}))
	defer server.Close()
	fs := &FlareSolverr{url: server.URL, client: server.Client()}
	if _, err := fs.Fetch("https://baak.example"); !errors.Is(err, ErrFlareSolverr) {
		t.Fatalf("HTTP failure accepted: %v", err)
	}
}

func TestResponseSerializationFailsBeforeWritingSuccess(t *testing.T) {
	response := httptest.NewRecorder()
	WriteJSONResponse(response, make(chan string))
	if response.Code != 500 || !json.Valid(response.Body.Bytes()) || !strings.Contains(response.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("invalid response: %d %s", response.Code, response.Body.String())
	}
}

func TestScrapersShareConnectionsButNotSessions(t *testing.T) {
	first, err := NewScraper("https://baak.example/")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewScraper("https://baak.example")
	if err != nil {
		t.Fatal(err)
	}
	if first.client.Transport != second.client.Transport {
		t.Fatal("connection pools not reused")
	}
	if first.client.Jar == second.client.Jar {
		t.Fatal("sessions shared between requests")
	}
}
