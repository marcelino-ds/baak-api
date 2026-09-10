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
	searchTerm, searchTypes, validationErr := mahasiswaBaruSearch(r)
	if validationErr != nil {
		utils.WriteValidationError(w, validationErr.Error())
		return
	}
	scraper, err := utils.NewScraper(config.AppConfig.BaseURL)
	if err != nil {
		utils.WriteInternalServerError(w)
		return
	}

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

func mahasiswaBaruSearch(r *http.Request) (string, []string, error) {
	if strings.HasPrefix(r.URL.Path, "/mahasiswabaru/") {
		searchTerm, err := pathSearch(r.URL.Path, "/mahasiswabaru/")
		return searchTerm, []string{"Kelas", "Nama"}, err
	}
	if r.URL.Path != "/cariMhsBaru" {
		return "", nil, fmt.Errorf("invalid student search path")
	}
	searchTerm, err := querySearch(r.URL.Query().Get("teks"))
	if err != nil {
		return "", nil, err
	}
	searchType := strings.TrimSpace(r.URL.Query().Get("tipeMhsBaru"))
	if searchType != "Kelas" && searchType != "Nama" {
		return "", nil, fmt.Errorf("tipeMhsBaru must be Kelas or Nama")
	}
	return searchTerm, []string{searchType}, nil
}
