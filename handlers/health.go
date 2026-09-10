package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/utils"
)

// ComponentStatus represents the health status of a component
type ComponentStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status         string                     `json:"status"`
	Timestamp      time.Time                  `json:"timestamp"`
	Version        string                     `json:"version"`
	Components     map[string]ComponentStatus `json:"components"`
	Cache          *utils.CacheStats          `json:"cache,omitempty"`
	CircuitBreaker *utils.CircuitBreakerStats `json:"circuit_breaker,omitempty"`
}

func HandlerLive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	utils.WriteJSONResponse(w, map[string]string{"status": "alive"})
}

func HandlerHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	components := make(map[string]ComponentStatus)
	overallStatus := "healthy"

	// The circuit state is a local readiness signal; probing BAAK here would
	// make health checks expensive and could amplify an upstream outage.
	baakStatus := ComponentStatus{Status: "unknown", Message: "Not checked"}

	// Check FlareSolverr if configured
	fs := utils.GetFlareSolverr()
	if fs != nil && fs.IsConfigured() {
		ctx, cancel := context.WithTimeout(r.Context(), config.HealthTimeout)
		defer cancel()
		if err := fs.CheckHealthContext(ctx); err != nil {
			components["flaresolverr"] = ComponentStatus{
				Status:  "unhealthy",
				Message: "FlareSolverr health check failed",
			}
			overallStatus = "degraded"
		} else {
			components["flaresolverr"] = ComponentStatus{
				Status:  "healthy",
				Message: "FlareSolverr is reachable",
			}
		}
	} else {
		components["flaresolverr"] = ComponentStatus{
			Status:  "not_configured",
			Message: "Set FLARESOLVERR_URL environment variable to enable",
		}
	}

	// Check circuit breaker state
	cb := utils.GetCircuitBreaker()
	cbStats := cb.Stats()
	if !cbStats.AcceptingRequests {
		baakStatus = ComponentStatus{
			Status:  "degraded",
			Message: "Circuit breaker is " + cbStats.State,
		}
		overallStatus = "degraded"
	} else if cbStats.State != "closed" {
		baakStatus = ComponentStatus{
			Status:  "recovering",
			Message: "Circuit breaker is ready for a recovery probe",
		}
	} else {
		baakStatus = ComponentStatus{
			Status:  "unknown",
			Message: "Circuit breaker is closed; upstream data availability is not probed",
		}
	}
	components["baak"] = baakStatus

	// Check cache
	var cacheStats *utils.CacheStats
	if config.AppConfig.CacheEnabled {
		cache := utils.GetCache()
		stats := cache.Stats()
		cacheStats = &stats
	}

	response := HealthResponse{
		Status:         overallStatus,
		Timestamp:      time.Now(),
		Version:        "2.0.0",
		Components:     components,
		Cache:          cacheStats,
		CircuitBreaker: &cbStats,
	}

	statusCode := http.StatusOK
	if overallStatus != "healthy" {
		statusCode = http.StatusServiceUnavailable
	}
	utils.WriteJSONResponseWithStatus(w, statusCode, response)
}
