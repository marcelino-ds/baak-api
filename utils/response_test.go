package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWrappedFlareSolverrBusyResponse(t *testing.T) {
	response := httptest.NewRecorder()
	WriteHTTPError(response, fmt.Errorf("%w: %w", ErrFlareSolverr, ErrFlareSolverrBusy))
	var body Response
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusServiceUnavailable || body.Success || body.Code != "FLARESOLVERR_BUSY" {
		t.Fatalf("busy response = %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Retry-After") != "1" {
		t.Fatal("busy response must tell the caller when to retry")
	}
}

func TestJSONResponsesAreNotCached(t *testing.T) {
	response := httptest.NewRecorder()

	WriteErrorResponseWithCode(response, http.StatusBadGateway, "upstream failed", "UPSTREAM_ERROR")

	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}
