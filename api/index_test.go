package api

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yafyx/baak-api/config"
)

func TestMiddlewareChainLogsRecoveredPanic(t *testing.T) {
	previous := config.AppConfig
	previousOutput := log.Writer()
	config.AppConfig.AllowedOrigins = []string{"https://app.example"}
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() {
		config.AppConfig = previous
		log.SetOutput(previousOutput)
	})
	handler := buildHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("private input")
	}))
	request := httptest.NewRequest(http.MethodGet, "/live", nil)
	request.Header.Set("Origin", "https://app.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d", response.Code)
	}
	if !strings.Contains(output.String(), "event=request") || !strings.Contains(output.String(), "status=500") {
		t.Fatalf("recovered panic missing from access log: %s", output.String())
	}
	id := response.Header().Get("X-Request-ID")
	if id == "" || strings.Count(output.String(), "request_id="+id) != 2 {
		t.Fatalf("panic and access logs must correlate with the response ID: %s", output.String())
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Fatal("panic response lost CORS headers")
	}
}

func TestRequestIDIsReadableByFrontend(t *testing.T) {
	previous := config.AppConfig
	config.AppConfig.AllowedOrigins = []string{"https://app.example"}
	t.Cleanup(func() { config.AppConfig = previous })
	for _, method := range []string{http.MethodGet, http.MethodOptions} {
		request := httptest.NewRequest(method, "/live", nil)
		request.Header.Set("Origin", "https://app.example")
		response := httptest.NewRecorder()
		Handler(response, request)
		if response.Header().Get("X-Request-ID") == "" {
			t.Errorf("%s response lacks request ID", method)
		}
		exposed := response.Header().Get("Access-Control-Expose-Headers")
		if !strings.Contains(exposed, "X-Request-ID") || !strings.Contains(exposed, "Retry-After") {
			t.Errorf("frontend cannot read diagnostic headers: %q", exposed)
		}
	}
}

func TestLiveProbeDoesNotDependOnUpstream(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/live", nil)
	response := httptest.NewRecorder()
	Handler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("live probe status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestUnknownRouteUsesStandardErrorEnvelope(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	response := httptest.NewRecorder()
	Handler(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d", response.Code)
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("error response is missing nosniff header")
	}
}

func TestConfigurationErrorIsReadableByAllowedOrigin(t *testing.T) {
	previousError, previousConfig := configurationError, config.AppConfig
	configurationError = errors.New("invalid upstream URL")
	config.AppConfig.AllowedOrigins = []string{"https://app.example"}
	t.Cleanup(func() {
		configurationError, config.AppConfig = previousError, previousConfig
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Origin", "https://app.example")
	response := httptest.NewRecorder()
	Handler(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("configuration error status = %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("configuration error CORS origin = %q, want https://app.example", got)
	}
}

func BenchmarkLiveHandler(b *testing.B) {
	previousLogOutput := log.Writer()
	log.SetOutput(io.Discard)
	b.Cleanup(func() { log.SetOutput(previousLogOutput) })
	request := httptest.NewRequest(http.MethodGet, "/live", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		response := httptest.NewRecorder()
		Handler(response, request)
		if response.Code != http.StatusOK {
			b.Fatalf("live probe status = %d", response.Code)
		}
	}
}
