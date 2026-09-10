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

func HandlerKelasbaru(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	searchTerm, searchTypes, validationErr := kelasBaruSearch(r)
	if validationErr != nil {
		utils.WriteValidationError(w, validationErr.Error())
		return
	}
	scraper, err := utils.NewScraper(config.AppConfig.BaseURL)
	if err != nil {
		utils.WriteInternalServerError(w)
		return
	}

	var kelasBaru []models.KelasBaru
	baseURL := strings.TrimRight(config.AppConfig.BaseURL, "/")
	kelasBaruBaseURL := fmt.Sprintf("%s/cariKelasBaru", baseURL)
	token, err := scraper.GetCSRFToken(r.Context(), kelasBaruBaseURL)
	if err != nil {
		token, err = scraper.GetCSRFToken(r.Context(), baseURL)
		if err != nil {
			utils.WriteHTTPError(w, err)
			return
		}
	}

	for _, searchType := range searchTypes {
		searchURL := fmt.Sprintf("%s/cariKelasBaru?_token=%s&tipeKelasBaru=%s&teks=%s",
			baseURL,
			url.QueryEscape(token),
			url.QueryEscape(searchType),
			url.QueryEscape(searchTerm),
		)
		kelasBaru, err = scraper.GetKelasbaru(r.Context(), searchURL)
		if err != nil {
			utils.WriteHTTPError(w, err)
			return
		}
		if len(kelasBaru) > 0 {
			break
		}
	}

	if len(kelasBaru) == 0 {
		utils.WriteNotFoundError(w)
		return
	}

	utils.WriteJSONResponse(w, kelasBaru)
}

func kelasBaruSearch(r *http.Request) (string, []string, error) {
	if strings.HasPrefix(r.URL.Path, "/kelasbaru/") {
		searchTerm, err := pathSearch(r.URL.Path, "/kelasbaru/")
		return searchTerm, []string{"Kelas", "NPM", "Nama"}, err
	}
	if r.URL.Path != "/cariKelasBaru" {
		return "", nil, fmt.Errorf("invalid class search path")
	}
	searchTerm, err := querySearch(r.URL.Query().Get("teks"))
	if err != nil {
		return "", nil, err
	}
	searchType := strings.TrimSpace(r.URL.Query().Get("tipeKelasBaru"))
	if searchType != "Kelas" && searchType != "NPM" && searchType != "Nama" {
		return "", nil, fmt.Errorf("tipeKelasBaru must be Kelas, NPM, or Nama")
	}
	return searchTerm, []string{searchType}, nil
}
