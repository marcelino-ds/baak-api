package utils

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDocumentDownloadRejectsExternalAndUnsafePaths(t *testing.T) {
	for _, target := range []string{"https://evil.example/file/a.pdf", "https://baak.example/adminAkademik/1",
		"https://baak.example/file/../adminAkademik/1", "https://baak.example/file/%2e%2e/admin", "https://u:p@baak.example/file/a.pdf"} {
		if _, err := DownloadPDF(context.Background(), "https://baak.example", target); err == nil {
			t.Errorf("unsafe source accepted: %s", target)
		}
	}
}

func TestDocumentDownloadValidatesRedirectBeforeSending(t *testing.T) {
	var called atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called.Store(true) }))
	defer other.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL+"/file/secret.pdf", 302) }))
	defer source.Close()
	if _, err := DownloadPDF(context.Background(), source.URL, source.URL+"/file/a.pdf"); err == nil {
		t.Fatal("external redirect accepted")
	}
	if called.Load() {
		t.Fatal("external server received a request")
	}
}

func TestDocumentDownloadLimitsBytesIncludingStreamingBodies(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("streaming=%t", streaming), func(t *testing.T) {
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !streaming {
					w.Header().Set("Content-Length", "16777217")
					return
				}
				w.(http.Flusher).Flush()
				_, _ = w.Write([]byte("%PDF-"))
				chunk := make([]byte, 64<<10)
				for range 257 {
					if _, err := w.Write(chunk); err != nil {
						return
					}
				}
			}))
			defer source.Close()
			if _, err := DownloadPDF(context.Background(), source.URL, source.URL+"/file/large.pdf"); !errors.Is(err, ErrPDFInvalid) {
				t.Fatalf("oversized PDF: %v", err)
			}
		})
	}
}

func TestDocumentDownloadCancelsSlowBody(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("%PDF-"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer source.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := DownloadPDF(ctx, source.URL, source.URL+"/file/slow.pdf"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("slow PDF: %v", err)
	}
}

func TestDocumentDownloadFollowsOnlyBoundedOfficialRedirects(t *testing.T) {
	var loopRequests atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/downloadAkademik/13":
			http.Redirect(w, r, "/file/kalender.pdf", http.StatusFound)
		case "/file/kalender.pdf":
			_, _ = w.Write(testTextPDF("Kalender"))
		case "/file/loop.pdf":
			loopRequests.Add(1)
			http.Redirect(w, r, "/file/loop.pdf", http.StatusFound)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer source.Close()
	if _, err := DownloadPDF(context.Background(), source.URL, source.URL+"/downloadAkademik/13"); err != nil {
		t.Fatal(err)
	}
	if _, err := DownloadPDF(context.Background(), source.URL, source.URL+"/file/loop.pdf"); !errors.Is(err, ErrPDFInvalid) {
		t.Fatalf("redirect loop: %v", err)
	}
	if loopRequests.Load() > 3 {
		t.Fatalf("redirect limit exceeded: %d requests", loopRequests.Load())
	}
}

func TestDocumentDownloadAcceptsPDFAndRejectsHTML(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/file/html.pdf" {
			w.Write([]byte("<html>error</html>"))
			return
		}
		w.Write(testTextPDF("BAAK"))
	}))
	defer source.Close()
	if _, err := DownloadPDF(context.Background(), source.URL, source.URL+"/file/a.pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := DownloadPDF(context.Background(), source.URL, source.URL+"/file/html.pdf"); err == nil {
		t.Fatal("HTML accepted")
	}
}

func TestDocumentDownloadRejectsDoubleEncodedTraversal(t *testing.T) {
	var calls atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write(testTextPDF("private"))
	}))
	defer source.Close()
	for _, resource := range []string{
		"/file/%2e%2e/admin.pdf",
		"/file/%252e%252e/admin.pdf",
		"/file/%255c..%255cadmin.pdf",
	} {
		if _, err := DownloadPDF(context.Background(), source.URL, source.URL+resource); !errors.Is(err, ErrPDFInvalid) {
			t.Errorf("encoded path %s: error = %v", resource, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("unsafe paths reached the source %d times", calls.Load())
	}
}
