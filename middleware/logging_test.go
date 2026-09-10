package middleware

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestLoggingMiddlewareIncludesStatusAndOmitsQuery(t *testing.T) {
	previousOutput := log.Writer()
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousOutput) })

	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	request := httptest.NewRequest(http.MethodGet, "/jadwal?q=secret-token", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusTeapot)
	}
	requestID := response.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Fatal("missing X-Request-ID response header")
	}
	entry := output.String()
	for _, expected := range []string{"status=418", "route=\"/jadwal\"", "request_id=" + requestID} {
		if !strings.Contains(entry, expected) {
			t.Errorf("log entry missing %q: %s", expected, entry)
		}
	}
	if strings.Contains(entry, "secret-token") {
		t.Fatalf("query string leaked into log: %s", entry)
	}
}

func TestLoggingRedactsSearchPathsAndUnmatchedRoutes(t *testing.T) {
	previousOutput := log.Writer()
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousOutput) })

	tests := []struct {
		path  string
		route string
	}{
		{path: "/mahasiswabaru/PrivateName", route: "/mahasiswabaru/{search}"},
		{path: "/kelasbaru/PrivateName", route: "/kelasbaru/{search}"},
		{path: "/jadwal/PrivateName", route: "/jadwal/{search}"},
		{path: "/uts/PrivateName", route: "/uts/{search}"},
		{path: "/unknown/PrivateName", route: "unmatched"},
	}
	seenIDs := make(map[string]bool)
	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	}))
	for _, tt := range tests {
		output.Reset()
		request := httptest.NewRequest(http.MethodGet, tt.path+"?q=QuerySecret", nil)
		request.Header.Set("X-Request-ID", "UserControlledID")
		request.Header.Set("Authorization", "Bearer AuthSecret")
		request.Header.Set("Cookie", "session=CookieSecret")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		entry := output.String()
		for _, secret := range []string{"PrivateName", "QuerySecret", "UserControlledID", "AuthSecret", "CookieSecret"} {
			if strings.Contains(entry, secret) {
				t.Errorf("%s leaked to access log", secret)
			}
		}
		if !strings.Contains(entry, `route="`+tt.route+`"`) || !strings.Contains(entry, "status=200") {
			t.Errorf("missing route/status: %s", entry)
		}
		id := response.Header().Get("X-Request-ID")
		if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) || seenIDs[id] {
			t.Errorf("expected a fresh server-generated ID, got %q", id)
		}
		seenIDs[id] = true
	}
}

func TestLoggingRecordsRecoveredPanicWithoutPayload(t *testing.T) {
	previousOutput := log.Writer()
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousOutput) })
	handler := LoggingMiddleware(RecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("PrivatePanicPayload")
	})))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	entry := output.String()
	if response.Code != http.StatusInternalServerError || !strings.Contains(entry, "status=500") {
		t.Fatalf("panic status = %d, log = %s", response.Code, entry)
	}
	if strings.Contains(entry, "PrivatePanicPayload") {
		t.Fatal("panic payload leaked into logs")
	}
	if strings.Count(entry, "request_id="+response.Header().Get("X-Request-ID")) != 2 {
		t.Fatalf("panic and access entries must share a request ID: %s", entry)
	}
}

func TestLoggingPreservesFinalStatusAfterInformationalResponse(t *testing.T) {
	previousOutput := log.Writer()
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousOutput) })
	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusEarlyHints)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("accepted"))
	}))
	server := httptest.NewServer(handler)
	defer server.Close()
	response, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	server.Close()
	if response.StatusCode != http.StatusAccepted || !strings.Contains(output.String(), "status=202") {
		t.Fatalf("final status = %d, log = %s", response.StatusCode, output.String())
	}
}

func TestLoggingNormalizesUnexpectedMethod(t *testing.T) {
	previousOutput := log.Writer()
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousOutput) })
	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest("GET", "/", nil)
	request.Method = "PRIVATE\nmethod"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !strings.Contains(output.String(), "method=OTHER") {
		t.Fatalf("unexpected method leaked into access log: %s", output.String())
	}
	if strings.Contains(output.String(), "PRIVATE") {
		t.Fatal("raw method was written to access log")
	}
}
