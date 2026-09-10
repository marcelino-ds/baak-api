package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
