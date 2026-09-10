package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/utils"
)

func HandlerUTS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	search, validationErr := pathSearch(r.URL.Path, "/uts/")
	if validationErr != nil {
		utils.WriteValidationError(w, validationErr.Error())
		return
	}
	scraper, err := utils.NewScraper(config.AppConfig.BaseURL)
	if err != nil {
		utils.WriteInternalServerError(w)
		return
	}

	baseURL := strings.TrimRight(config.AppConfig.BaseURL, "/")
	targetURL := fmt.Sprintf("%s/jadwal/cariUts?teks=%s", baseURL, url.QueryEscape(search))
	uts, err := scraper.GetUTS(r.Context(), targetURL)
	if err != nil {
		utils.WriteHTTPError(w, err)
		return
	}

	utils.WriteJSONResponse(w, uts)
}
