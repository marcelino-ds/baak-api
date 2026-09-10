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
	applicationHandler = buildHandler(http.HandlerFunc(handleRoutes))
}

var configurationError error
var applicationHandler http.Handler

func buildHandler(next http.Handler) http.Handler {
	routes := middleware.RateLimitMiddleware(next)
	configuredRoutes := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if configurationError != nil {
			log.Printf("invalid configuration: %v", configurationError)
			utils.WriteConfigurationError(w)
			return
		}
		routes.ServeHTTP(w, r)
	})
	return middleware.LoggingMiddleware(
		middleware.RecoveryMiddleware(
			middleware.CORSMiddleware(configuredRoutes),
		),
	)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), config.RequestTimeout)
	defer cancel()

	applicationHandler.ServeHTTP(w, r.WithContext(ctx))
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
	case r.URL.Path == "/public" || r.URL.Path == "/public/" || r.URL.Path == "/catalog":
		handlers.HandlerPublicCatalog(w, r)
	case strings.HasPrefix(r.URL.Path, "/public/"):
		handlers.HandlerPublic(w, r)
	case strings.HasPrefix(r.URL.Path, "/layanan/"):
		handlers.HandlerLayanan(w, r)
	case r.URL.Path == "/layanan":
		handlers.HandlerLayanan(w, r)
	case r.URL.Path == "/dokumen" || strings.HasPrefix(r.URL.Path, "/dokumen/"):
		handlers.HandlerDocuments(w, r)
	case r.URL.Path == "/ktm" || strings.HasPrefix(r.URL.Path, "/ktm/"):
		handlers.HandlerKTM(w, r)
	case r.URL.Path == "/arsip-kalender":
		handlers.HandlerPublicAlias("arsip-kalender")(w, r)
	case r.URL.Path == "/ujian-utama" || strings.HasPrefix(r.URL.Path, "/ujian-utama/"):
		handlers.HandlerPublicAlias("ujian-utama")(w, r)
	case strings.HasPrefix(r.URL.Path, "/uas/"):
		handlers.HandlerPublicAlias("uas")(w, r)
	case r.URL.Path == "/uas":
		handlers.HandlerPublicAlias("uas")(w, r)
	case r.URL.Path == "/mata-kuliah":
		handlers.HandlerPublicAlias("mata-kuliah")(w, r)
	case r.URL.Path == "/koordinator":
		handlers.HandlerPublicAlias("koordinator")(w, r)
	case r.URL.Path == "/pembimbing-pi":
		handlers.HandlerPublicAlias("pembimbing-pi")(w, r)
	case r.URL.Path == "/panduan-kuliah":
		handlers.HandlerPublicAlias("panduan-kuliah")(w, r)
	case r.URL.Path == "/jadwal-ujian":
		handlers.HandlerPublicAlias("jadwal-ujian")(w, r)
	case r.URL.Path == "/ujian-bentrok":
		handlers.HandlerPublicAlias("ujian-bentrok")(w, r)
	case r.URL.Path == "/frs":
		handlers.HandlerPublicAlias("frs")(w, r)
	case r.URL.Path == "/buku-pedoman":
		handlers.HandlerPublicAlias("buku-pedoman")(w, r)
	case r.URL.Path == "/situs":
		handlers.HandlerPublicAlias("situs")(w, r)
	case r.URL.Path == "/loket":
		handlers.HandlerPublicAlias("loket")(w, r)
	case r.URL.Path == "/ringkasan":
		handlers.HandlerPublicAlias("ringkasan")(w, r)
	case r.URL.Path == "/berita" || strings.HasPrefix(r.URL.Path, "/berita/"):
		handlers.HandlerPublicAlias("berita")(w, r)
	case r.URL.Path == "/dosen-wali" || strings.HasPrefix(r.URL.Path, "/dosen-wali/"):
		handlers.HandlerPublicAlias("dosen-wali")(w, r)
	case r.URL.Path == "/daftar-ulang":
		handlers.HandlerPublicAlias("daftar-ulang")(w, r)
	case r.URL.Path == "/cuti":
		handlers.HandlerPublicAlias("cuti")(w, r)
	case r.URL.Path == "/nonaktif":
		handlers.HandlerPublicAlias("nonaktif")(w, r)
	case r.URL.Path == "/pengecekan-nilai":
		handlers.HandlerPublicAlias("pengecekan-nilai")(w, r)
	case r.URL.Path == "/pindah-lokasi":
		handlers.HandlerPublicAlias("pindah-lokasi")(w, r)
	case r.URL.Path == "/pindah-jurusan":
		handlers.HandlerPublicAlias("pindah-jurusan")(w, r)
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
