package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/models"
	"github.com/yafyx/baak-api/utils"
)

const publicPageTTL = 10 * time.Minute

// HandlerPublic serves allowlisted read-only BAAK pages as normalized JSON.
func HandlerPublic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	value := strings.TrimPrefix(r.URL.Path, "/public/")
	if value == "" {
		HandlerPublicCatalog(w, r)
		return
	}
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) == 0 || parts[0] == "" || len(parts) > 2 {
		utils.WriteNotFoundError(w)
		return
	}
	parameter := ""
	if len(parts) == 2 {
		parameter = parts[1]
	}
	handlePublicSection(w, r, parts[0], parameter)
}

func HandlerPublicCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	utils.WriteJSONResponse(w, utils.PublicCatalog())
}

func HandlerLayanan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	prefix := "/layanan/"
	if r.URL.Path == "/layanan" {
		utils.WriteJSONResponse(w, []string{"daftar-ulang", "cuti", "nonaktif", "pengecekan-nilai", "pindah-lokasi", "pindah-jurusan"})
		return
	}
	if !strings.HasPrefix(r.URL.Path, prefix) {
		utils.WriteNotFoundError(w)
		return
	}
	section := strings.TrimPrefix(r.URL.Path, prefix)
	switch section {
	case "daftar-ulang", "cuti", "nonaktif", "pengecekan-nilai", "pindah-lokasi", "pindah-jurusan":
	default:
		utils.WriteNotFoundError(w)
		return
	}
	handlePublicSection(w, r, section, "")
}

func HandlerDocuments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if r.URL.Path == "/dokumen" {
		utils.WriteJSONResponse(w, []string{"/dokumen/mata-kuliah", "/dokumen/buku-pedoman", "/dokumen/frs", "/dokumen/kalender"})
		return
	}
	category := strings.TrimPrefix(r.URL.Path, "/dokumen/")
	switch category {
	case "mata-kuliah", "buku-pedoman", "frs", "kalender":
		handlePublicSection(w, r, "dokumen-"+category, "")
	default:
		utils.WriteNotFoundError(w)
	}
}

func HandlerKTM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if r.URL.Path == "/ktm" {
		handlePublicSection(w, r, "ktm", "")
		return
	}
	search, err := pathSearch(r.URL.Path, "/ktm/")
	if err != nil {
		utils.WriteValidationError(w, err.Error())
		return
	}
	copy := r.Clone(r.Context())
	copy.URL.Path = "/mahasiswabaru/" + search
	copy.URL.RawPath = ""
	HandlerMahasiswaBaru(w, copy)
}

// HandlerPublicAlias exposes a short, stable alias for an allowlisted section.
func HandlerPublicAlias(section string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			utils.WriteErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		parameter := ""
		if prefix := "/" + section + "/"; strings.HasPrefix(r.URL.Path, prefix) {
			parameter = strings.TrimPrefix(r.URL.Path, prefix)
		}
		handlePublicSection(w, r, section, parameter)
	}
}

func handlePublicSection(w http.ResponseWriter, r *http.Request, section, parameter string) {
	if section == "uas" && parameter == "" {
		parameter = strings.TrimSpace(r.URL.Query().Get("q"))
		if parameter == "" {
			utils.WriteValidationError(w, "kelas or query parameter q is required")
			return
		}
	}
	if section == "uas" && parameter != "" {
		handleUAS(w, r, parameter)
		return
	}
	path, ok := utils.PublicPath(section, parameter)
	if !ok || strings.Contains(parameter, "/") {
		utils.WriteNotFoundError(w)
		return
	}

	query, err := publicQuery(section, r.URL.Query())
	if err != nil {
		utils.WriteValidationError(w, err.Error())
		return
	}

	baseURL := strings.TrimRight(config.AppConfig.BaseURL, "/")
	target, err := utils.PublicTarget(baseURL, path, query)
	if err != nil {
		utils.WriteInternalServerError(w)
		return
	}
	key := cacheKey("public", section, target)
	page, err := cachedValue(
		r.Context(),
		config.AppConfig.CacheEnabled,
		publicPageTTL,
		key,
		func(ctx context.Context) (models.PublicPage, error) {
			scraper, createErr := utils.NewScraper(baseURL)
			if createErr != nil {
				return models.PublicPage{}, createErr
			}
			doc, fetchErr := scraper.FetchDocument(ctx, target)
			if fetchErr != nil {
				return models.PublicPage{}, fetchErr
			}
			page, parseErr := utils.ParsePublicPage(doc, target)
			if parseErr != nil {
				return models.PublicPage{}, parseErr
			}
			page.Section = section
			if err := utils.ValidatePublicTables(&page, section); err != nil {
				return models.PublicPage{}, err
			}
			return page, nil
		},
	)
	if err != nil {
		utils.WriteHTTPError(w, err)
		return
	}
	page = projectPublicPage(section, page)
	utils.WriteJSONResponse(w, page)
}

func projectPublicPage(section string, page models.PublicPage) models.PublicPage {
	page.Section = section
	switch section {
	case "situs":
		links := make([]models.PublicLink, 0)
		for _, link := range page.Links {
			u, err := url.Parse(link.URL)
			if err != nil {
				continue
			}
			switch strings.ToLower(u.Hostname()) {
			case "rps.gunadarma.ac.id", "sidang.gunadarma.ac.id", "fti.gunadarma.ac.id", "filkom.gunadarma.ac.id",
				"ftsp.gunadarma.ac.id", "fpsi.gunadarma.ac.id", "fe.gunadarma.ac.id", "fsastra.gunadarma.ac.id":
				links = append(links, link)
			}
		}
		page.Links, page.Tables = links, nil
		page.Title = "Situs Resmi Universitas Gunadarma"
		page.Text = "Tautan resmi RPS, sidang, dan program studi."
		page.News = nil
	case "loket":
		tables := make([]models.PublicTable, 0)
		for _, table := range page.Tables {
			headers := strings.ToLower(strings.Join(table.Headers, " "))
			if strings.Contains(headers, "hari") && strings.Contains(headers, "waktu") {
				tables = append(tables, table)
			}
		}
		page.Tables, page.Links = tables, nil
		page.Title = "Jam Pelayanan Loket BAAK"
		page.Text = "Jam layanan dan istirahat sesuai informasi pada beranda BAAK."
		page.News = nil
	case "arsip-kalender", "dokumen-kalender":
		links := make([]models.PublicLink, 0)
		for _, link := range page.Links {
			if strings.Contains(link.URL, "/downloadAkademik/") || strings.Contains(strings.ToLower(link.Text), "kalender") {
				links = append(links, link)
			}
		}
		page.Links, page.Tables = links, nil
		page.Title = "Arsip Kalender Akademik"
		page.Text = "Tautan dokumen kalender yang dipublikasikan BAAK."
		page.News = nil
	case "mata-kuliah", "buku-pedoman", "dokumen-mata-kuliah", "dokumen-buku-pedoman", "dokumen-frs":
		links := make([]models.PublicLink, 0)
		for _, link := range page.Links {
			if link.Kind == "document" {
				links = append(links, link)
			}
		}
		page.Links = links
		page.News = nil
	}
	return page
}

func publicQuery(section string, input url.Values) (url.Values, error) {
	query := make(url.Values)
	allowed := map[string]bool{"page": true}
	switch section {
	case "ujian-utama":
		allowed["jurusan"] = true
	case "berita":
		allowed["sort"] = true
	case "dosen-wali":
		allowed["search_wali"], allowed["sort"], allowed["q"] = true, true, true
	case "koordinator":
		allowed["search_koor"], allowed["sort"], allowed["q"] = true, true, true
	case "pembimbing-pi":
		allowed["search_pi"], allowed["sort"], allowed["q"] = true, true, true
	}
	for key, values := range input {
		if !allowed[key] {
			continue
		}
		if len(values) != 1 {
			return nil, fmt.Errorf("query parameter must have one value")
		}
		value := strings.TrimSpace(values[0])
		if len(value) > 256 || strings.IndexAny(value, "\r\n") >= 0 {
			return nil, fmt.Errorf("%s is invalid", key)
		}
		if key == "page" {
			page, parseErr := strconv.Atoi(value)
			if parseErr != nil || page < 1 || page > 10000 {
				return nil, fmt.Errorf("page must be between 1 and 10000")
			}
		}
		if value != "" {
			query.Set(key, value)
		}
	}
	if alias := query.Get("q"); alias != "" {
		field := map[string]string{"dosen-wali": "search_wali", "koordinator": "search_koor", "pembimbing-pi": "search_pi"}[section]
		if native := query.Get(field); native != "" && native != alias {
			return nil, fmt.Errorf("q conflicts with %s", field)
		}
		query.Set(field, alias)
		query.Del("q")
	}
	return query, nil
}

func handleUAS(w http.ResponseWriter, r *http.Request, kelas string) {
	path, ok := utils.PublicPath("uas", kelas)
	if !ok {
		utils.WriteValidationError(w, "kelas is invalid")
		return
	}
	baseURL := strings.TrimRight(config.AppConfig.BaseURL, "/")
	target, err := utils.PublicTarget(baseURL, path, nil)
	if err != nil {
		utils.WriteInternalServerError(w)
		return
	}
	key := cacheKey("uas", baseURL, kelas)
	page, err := cachedValue(
		r.Context(), config.AppConfig.CacheEnabled, publicPageTTL, key,
		func(ctx context.Context) (models.PublicPage, error) {
			scraper, createErr := utils.NewScraper(baseURL)
			if createErr != nil {
				return models.PublicPage{}, createErr
			}
			// Establish the same cookie session used by the browser form.
			doc, fetchErr := scraper.FetchDocument(ctx, target)
			if fetchErr != nil {
				return models.PublicPage{}, fetchErr
			}
			values := url.Values{"teks": {kelas}}
			if token, exists := doc.Find(`input[name="_token"]`).First().Attr("value"); exists && strings.TrimSpace(token) != "" {
				values.Set("_token", token)
			}
			result, fetchErr := scraper.FetchFormDocument(ctx, target, values)
			if fetchErr != nil {
				return models.PublicPage{}, fetchErr
			}
			page, parseErr := utils.ParsePublicPage(result, target)
			if parseErr != nil {
				return models.PublicPage{}, parseErr
			}
			page.Section = "uas"
			if err := utils.ValidatePublicTables(&page, "uas"); err != nil {
				return models.PublicPage{}, err
			}
			return page, nil
		},
	)
	if err != nil {
		utils.WriteHTTPError(w, err)
		return
	}
	utils.WriteJSONResponse(w, page)
}
