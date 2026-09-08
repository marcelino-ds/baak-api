package handlers

import (
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
	Status       string                     `json:"status"`
	Timestamp    time.Time                  `json:"timestamp"`
	Version      string                     `json:"version"`
	Components   map[string]ComponentStatus `json:"components"`
	Cache        *utils.CacheStats          `json:"cache,omitempty"`
	CircuitBreaker *utils.CircuitBreakerStats `json:"circuit_breaker,omitempty"`
}

func HandlerHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	components := make(map[string]ComponentStatus)
	overallStatus := "healthy"

	// Check BAAK reachability (non-blocking quick check)
	baakStatus := ComponentStatus{Status: "unknown", Message: "Not checked"}
	
	// Check FlareSolverr if configured
	fs := utils.GetFlareSolverr()
	if fs != nil && fs.IsConfigured() {
		if err := fs.CheckHealthContext(r.Context()); err != nil {
			components["flaresolverr"] = ComponentStatus{
				Status:  "unhealthy",
				Message: err.Error(),
			}
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
	if cbStats.State == "open" {
		baakStatus = ComponentStatus{
			Status:  "degraded",
			Message: "Circuit breaker is open due to repeated failures",
		}
		overallStatus = "degraded"
	} else {
		baakStatus = ComponentStatus{
			Status:  "operational",
			Message: "Circuit breaker is closed",
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

	utils.WriteJSONResponse(w, response)
}

