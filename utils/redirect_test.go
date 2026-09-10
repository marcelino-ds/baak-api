package utils

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestScraperRejectsRedirectOutsideConfiguredOrigin(t *testing.T) {
	var externalRequests atomic.Int32
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		externalRequests.Add(1)
		fmt.Fprint(w, `<title>external</title><main>unexpected</main>`)
	}))
	defer external.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, external.URL+"/private", http.StatusFound)
	}))
	defer upstream.Close()

	scraper, err := NewScraper(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scraper.FetchDocument(context.Background(), upstream.URL+"/"); err == nil {
		t.Fatal("scraper accepted an external redirect")
	}
	if externalRequests.Load() != 0 {
		t.Fatal("external redirect target received a request")
	}
	if _, err := scraper.FetchDocument(context.Background(), external.URL+"/"); err == nil {
		t.Fatal("scraper accepted an initial URL outside its configured origin")
	}
	if externalRequests.Load() != 0 {
		t.Fatal("external initial target received a request")
	}
}
