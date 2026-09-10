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
	searchTerm, validationErr := pathSearch(r.URL.Path, "/mahasiswabaru/")
	if validationErr != nil {
		utils.WriteValidationError(w, validationErr.Error())
		return
	}
	scraper, err := utils.NewScraper(config.AppConfig.BaseURL)
	if err != nil {
		utils.WriteInternalServerError(w)
		return
	}

	searchTypes := []string{"Kelas", "Nama"}
	var mahasiswaBaru []models.MahasiswaBaru
	baseURL := strings.TrimRight(config.AppConfig.BaseURL, "/")
	mhsBaruBaseURL := fmt.Sprintf("%s/cariMhsBaru", baseURL)
	token, err := scraper.GetCSRFToken(r.Context(), mhsBaruBaseURL)
	if err != nil {
		token, err = scraper.GetCSRFToken(r.Context(), baseURL)
		if err != nil {
			utils.WriteHTTPError(w, err)
			return
		}
	}

	for _, searchType := range searchTypes {
		searchURL := fmt.Sprintf("%s/cariMhsBaru?_token=%s&tipeMhsBaru=%s&teks=%s",
			baseURL,
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
