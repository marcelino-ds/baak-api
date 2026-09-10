package handlers

import (
	"context"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/models"
	"github.com/yafyx/baak-api/utils"
)

// HandlerDocumentText resolves a content ID from an official catalog before any download.
func HandlerDocumentText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/dokumen/"), "/")
	if len(parts) != 3 || parts[2] != "teks" {
		utils.WriteNotFoundError(w)
		return
	}
	category, id := parts[0], parts[1]
	switch category {
	case "mata-kuliah", "buku-pedoman", "frs", "kalender":
	default:
		utils.WriteNotFoundError(w)
		return
	}
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 32 {
		utils.WriteValidationError(w, "document ID must be the SHA-256 ID returned by the catalog")
		return
	}
	resource, _ := utils.PublicPath("dokumen-"+category, "")
	target, err := utils.PublicTarget(config.AppConfig.BaseURL, resource, nil)
	if err != nil {
		utils.WriteConfigurationError(w)
		return
	}
	catalog, err := loadPublicPage(r.Context(), "dokumen-"+category, target)
	if err != nil {
		utils.WriteHTTPError(w, err)
		return
	}
	var selected *models.PublicLink
	for _, link := range catalog.Links {
		if link.ID == id && link.Kind == "document" {
			copy := link
			selected = &copy
			break
		}
	}
	if selected == nil {
		utils.WriteNotFoundError(w)
		return
	}
	const ttl = 24 * time.Hour
	key := cacheKey("pdf", config.AppConfig.BaseURL, id)
	document, err := cachedValue(r.Context(), config.AppConfig.CacheEnabled, ttl, key, func(ctx context.Context) (models.PDFDocument, error) {
		data, err := utils.DownloadPDF(ctx, config.AppConfig.BaseURL, selected.URL)
		if err != nil {
			return models.PDFDocument{}, err
		}
		parsed, err := utils.ExtractPDF(ctx, data)
		if err != nil {
			return models.PDFDocument{}, err
		}
		parsed.ID = id
		parsed.Source = selected.URL
		if parsed.Title == "" {
			parsed.Title = selected.Text
		}
		now := time.Now().UTC()
		parsed.Metadata = models.SourceMetadata{FetchedAt: now, ExpiresAt: now.Add(ttl), ContentHash: utils.DocumentID(string(data))}
		return parsed, nil
	})
	if err != nil {
		utils.WriteHTTPError(w, err)
		return
	}
	utils.WriteJSONResponse(w, document)
}
