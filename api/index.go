package api

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/handlers"
	"github.com/yafyx/baak-api/middleware"
	"github.com/yafyx/baak-api/utils"
)

func init() {
	config.LoadConfig()
	configurationError = config.AppConfig.Validate()
}

var configurationError error

func Handler(w http.ResponseWriter, r *http.Request) {
	if configurationError != nil {
		log.Printf("invalid configuration: %v", configurationError)
		utils.WriteConfigurationError(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), config.RequestTimeout)
	defer cancel()

	r = r.WithContext(ctx)

	// Apply middleware chain
	handler := middleware.RecoveryMiddleware(
		middleware.LoggingMiddleware(
			middleware.CORSMiddleware(
				middleware.RateLimitMiddleware(
					http.HandlerFunc(handleRoutes),
				),
			),
		),
	)
	handler.ServeHTTP(w, r)
}

func handleRoutes(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/":
		handlers.HandlerHomepage(w, r)
	case r.URL.Path == "/health":
		handlers.HandlerHealth(w, r)
	case r.URL.Path == "/ready":
		handlers.HandlerHealth(w, r)
	case r.URL.Path == "/live":
		handlers.HandlerLive(w, r)
	case r.URL.Path == "/jadwal":
		handlers.HandlerJadwalSearch(w, r)
	case strings.HasPrefix(r.URL.Path, "/jadwal/"):
		handlers.HandlerJadwal(w, r)
	case r.URL.Path == "/kalender":
		handlers.HandlerKegiatan(w, r)
	case strings.HasPrefix(r.URL.Path, "/kelasbaru/"):
		handlers.HandlerKelasbaru(w, r)
	case strings.HasPrefix(r.URL.Path, "/uts/"):
		handlers.HandlerUTS(w, r)
	case strings.HasPrefix(r.URL.Path, "/mahasiswabaru/"):
		handlers.HandlerMahasiswaBaru(w, r)
	default:
		utils.WriteNotFoundError(w)
	}
}
