package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/utils"
)

func TestHealthReportsDependencyFailureAndRecovers(t *testing.T) {
	for _, upstreamStatus := range []int{http.StatusServiceUnavailable, http.StatusOK} {
		t.Run(http.StatusText(upstreamStatus), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(upstreamStatus)
			}))
			defer server.Close()
			configureCacheTest(t, "https://baak.example")
			config.AppConfig.FlareSolverrURL = server.URL
			response := httptest.NewRecorder()
			HandlerHealth(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
			var envelope struct {
				Success bool
				Data    HealthResponse
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if response.Code != upstreamStatus || envelope.Success != (upstreamStatus == http.StatusOK) {
				t.Fatalf("health did not reflect dependency status: %d %s", response.Code, response.Body.String())
			}
			if envelope.Data.Components["baak"].Status != "unknown" {
				t.Fatal("health claimed to probe BAAK")
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("health response may be cached")
			}
		})
	}
}

func TestHealthTimeoutAndLivenessIndependence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	configureCacheTest(t, "https://baak.example")
	config.AppConfig.FlareSolverrURL = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	response := httptest.NewRecorder()
	HandlerHealth(response, httptest.NewRequest(http.MethodGet, "/ready", nil).WithContext(ctx))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("health = %d", response.Code)
	}
	for range 5 {
		utils.GetCircuitBreaker().RecordFailure()
	}
	response = httptest.NewRecorder()
	HandlerLive(response, httptest.NewRequest(http.MethodGet, "/live", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("liveness depends on upstream: %d", response.Code)
	}
}

func TestHealthReportsOpenCircuit(t *testing.T) {
	configureCacheTest(t, "https://baak.example")
	for range 5 {
		utils.GetCircuitBreaker().RecordFailure()
	}
	response := httptest.NewRecorder()
	HandlerHealth(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("open circuit reported %d", response.Code)
	}
}
