package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/models"
	"github.com/yafyx/baak-api/utils"
)

func HandlerJadwal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	search := strings.TrimPrefix(r.URL.Path, "/jadwal/")
	if search == "" {
		utils.WriteValidationError(w, "Missing kelas in URL")
		return
	}

	// Validate input
	if len(search) < 3 {
		utils.WriteValidationError(w, "Kelas must be at least 3 characters long")
		return
	}
	jadwal, err := getCachedJadwal(r.Context(), search)
	if err != nil {
		utils.WriteHTTPError(w, err)
		return
	}

	response := struct {
		Kelas  string        `json:"kelas"`
		Jadwal models.Jadwal `json:"jadwal"`
	}{
		Kelas:  search,
		Jadwal: jadwal,
	}

	utils.WriteJSONResponse(w, response)
}

func HandlerJadwalSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	search := r.URL.Query().Get("q")
	if search == "" {
		utils.WriteValidationError(w, "Missing search query parameter 'q'")
		return
	}

	// Validate input
	if len(search) < 3 {
		utils.WriteValidationError(w, "Search query must be at least 3 characters long")
		return
	}
	jadwal, err := getCachedJadwal(r.Context(), search)
	if err != nil {
		utils.WriteHTTPError(w, err)
		return
	}

	response := struct {
		Query  string        `json:"query"`
		Jadwal models.Jadwal `json:"jadwal"`
	}{
		Query:  search,
		Jadwal: jadwal,
	}

	utils.WriteJSONResponse(w, response)
}

func getCachedJadwal(ctx context.Context, search string) (models.Jadwal, error) {
	baseURL := strings.TrimRight(config.AppConfig.BaseURL, "/")
	key := cacheKey("jadwal", baseURL, search)
	return cachedValue(
		ctx,
		config.AppConfig.CacheEnabled,
		config.AppConfig.CacheTTLJadwal,
		key,
		func(ctx context.Context) (models.Jadwal, error) {
			scraper, err := utils.NewScraper(baseURL)
			if err != nil {
				return models.Jadwal{}, err
			}
			// BAAK serves the schedule search form on its homepage; /jadwal is a 404.
			token, err := scraper.GetCSRFToken(ctx, baseURL)
			if err != nil {
				return models.Jadwal{}, err
			}
			searchURL := fmt.Sprintf("%s/jadwal/cariJadKul?_token=%s&teks=%s",
				baseURL,
				url.QueryEscape(token),
				url.QueryEscape(search),
			)
			return scraper.GetJadwal(ctx, searchURL)
		},
	)
}
