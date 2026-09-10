package handlers

import (
	"net/http"

	"github.com/yafyx/baak-api/utils"
)

func HandlerHomepage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	endpoints := []string{
		"/live", "/ready", "/health", "/jadwal?q={kelas/dosen}",
		"/jadwal/{kelas}",
		"/kalender",
		"/kelasbaru/{kelas/npm/nama}",
		"/uts/{kelas/dosen}",
		"/mahasiswabaru/{kelas/nama}",
	}
	utils.WriteJSONResponse(w, endpoints)
}
