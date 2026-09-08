package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/models"
	"github.com/yafyx/baak-api/utils"
)

func HandlerMahasiswaBaru(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	searchTerm := strings.TrimPrefix(r.URL.Path, "/mahasiswabaru/")
	if searchTerm == "" {
		utils.WriteValidationError(w, "Missing search term in URL")
		return
	}
	scraper, err := utils.NewScraper(config.AppConfig.BaseURL)
	if err != nil {
		utils.WriteInternalServerError(w)
		return
	}

	searchTypes := []string{"Kelas", "Nama"}
	var mahasiswaBaru []models.MahasiswaBaru
	mhsBaruBaseURL := fmt.Sprintf("%s/cariMhsBaru", config.AppConfig.BaseURL)
	token, err := scraper.GetCSRFToken(r.Context(), mhsBaruBaseURL)
	if err != nil {
		token, err = scraper.GetCSRFToken(r.Context(), config.AppConfig.BaseURL)
		if err != nil {
			utils.WriteHTTPError(w, err)
			return
		}
	}

	for _, searchType := range searchTypes {
		searchURL := fmt.Sprintf("%s/cariMhsBaru?_token=%s&tipeMhsBaru=%s&teks=%s",
			config.AppConfig.BaseURL,
			url.QueryEscape(token),
			url.QueryEscape(searchType),
			url.QueryEscape(searchTerm),
		)

		mahasiswaBaru, err = scraper.GetMahasiswaBaru(r.Context(), searchURL)
		if err != nil {
			utils.WriteHTTPError(w, err)
			return
		}
		if len(mahasiswaBaru) > 0 {
			break
		}
	}

	if len(mahasiswaBaru) == 0 {
		utils.WriteNotFoundError(w)
		return
	}

	utils.WriteJSONResponse(w, mahasiswaBaru)
}
