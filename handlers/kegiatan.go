package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/models"
	"github.com/yafyx/baak-api/utils"
)

func HandlerKegiatan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	baseURL := strings.TrimRight(config.AppConfig.BaseURL, "/")
	key := cacheKey("kalender", baseURL)
	kegiatanList, err := cachedValue(
		r.Context(),
		config.AppConfig.CacheEnabled,
		config.AppConfig.CacheTTLKalender,
		key,
		func(ctx context.Context) ([]models.Kegiatan, error) {
			scraper, err := utils.NewScraper(baseURL)
			if err != nil {
				return nil, err
			}
			return scraper.GetKegiatan(ctx)
		},
	)
	if err != nil {
		utils.WriteHTTPError(w, err)
		return
	}

	utils.WriteJSONResponse(w, kegiatanList)
}
