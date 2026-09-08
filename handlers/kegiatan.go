package handlers

import (
	"net/http"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/utils"
)

func HandlerKegiatan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	scraper, err := utils.NewScraper(config.AppConfig.BaseURL)
	if err != nil {
		utils.WriteInternalServerError(w)
		return
	}
	kegiatanList, err := scraper.GetKegiatan(r.Context())
	if err != nil {
		utils.WriteHTTPError(w, err)
		return
	}

	utils.WriteJSONResponse(w, kegiatanList)
}
