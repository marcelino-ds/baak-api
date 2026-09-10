package utils

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/yafyx/baak-api/models"
	"golang.org/x/net/html"
)

var (
	errPublicPage      = fmt.Errorf("%w: public BAAK page is missing usable content", ErrUnexpectedPage)
	publicKeyPattern   = regexp.MustCompile(`[^a-z0-9_]+`)
	publicTotalPattern = regexp.MustCompile(`(?i)Total\s+Records\s*:\s*([0-9]+)`)
	publicIDPattern    = regexp.MustCompile(`^[1-9][0-9]{0,8}$`)
)

// ParsePublicPage selects the requested BAAK content and returns text, records and safe source links.
func ParsePublicPage(doc *goquery.Document, source string) (models.PublicPage, error) {
	if doc == nil {
		return models.PublicPage{}, errPublicPage
	}
	base, err := url.Parse(source)
	if err != nil || base.Host == "" || base.User != nil {
		return models.PublicPage{}, errPublicPage
	}
	pageTitle := cleanPublicText(doc.Find("title").Text())
	if isPublicErrorPage(pageTitle) {
		return models.PublicPage{}, errPublicPage
	}
	scope := publicScope(doc, base.Path)
	if scope == nil {
		return models.PublicPage{}, errPublicPage
	}
	rootLinks := make([]models.PublicLink, 0)
	if base.Path == "/" {
		rootLinks = publicLinks(doc.Find("body").First(), base)
	}
	scope = scope.Clone()
	scope.Find("script, style, noscript, iframe, nav, header, footer, aside, input, button, .rd-navbar, .breadcrumb-classic, .resp-tabs-list, .resp-accordion").Remove()
	title := cleanPublicText(scope.Find("h1, h2, h3, h4, h5").First().Text())
	if title == "" {
		title = pageTitle
	}
	text := publicReadableText(scope)
	if text == "" || isPublicErrorPage(title+" "+text) || len(text) > 200000 {
		return models.PublicPage{}, errPublicPage
	}
	page := models.PublicPage{
		Section: publicSectionForPath(base.Path),
		Path:    base.Path, Source: safePublicURL(base, source), Title: title, Text: text,
		Links: publicLinks(scope, base), Tables: make([]models.PublicTable, 0),
		Options: make(map[string][]models.PublicOption), News: publicNews(scope, base),
		Pagination: publicPagination(scope, base),
	}
	if base.Path == "/" {
		page.Links = rootLinks
	}
	scope.Find("select[name]").Each(func(_ int, box *goquery.Selection) {
		name, _ := box.Attr("name")
		options := make([]models.PublicOption, 0)
		box.Find("option[value]").Each(func(_ int, option *goquery.Selection) {
			value, _ := option.Attr("value")
			if value != "" {
				options = append(options, models.PublicOption{Label: cleanPublicText(option.Text()), Value: value})
			}
		})
		page.Options[name] = options
	})
	seenTables := make(map[string]bool)
	scope.Find("table").Each(func(_ int, table *goquery.Selection) {
		// stacktable adds mobile copies. Keep the original when a desktop table exists.
		if table.HasClass("small-only") && !table.HasClass("large-only") {
			return
		}
		parsed := publicTable(table)
		if len(parsed.Headers) == 0 && len(parsed.Rows) == 0 {
			return
		}
		var encoded []byte
		if len(parsed.Records) > 0 {
			encoded, _ = json.Marshal(parsed.Records)
		} else {
			encoded, _ = json.Marshal(parsed.Headers)
		}
		key := string(encoded)
		if seenTables[key] {
			return
		}
		seenTables[key] = true
		page.Tables = append(page.Tables, parsed)
	})
	return page, nil
}

func publicScope(doc *goquery.Document, sourcePath string) *goquery.Selection {
	if match := regexp.MustCompile(`/kuliahUjian/([1-9])(?:/[1-5])?$`).FindStringSubmatch(sourcePath); len(match) == 2 {
		index, _ := strconv.Atoi(match[1])
		// BAAK adds resp-tab-content classes in JavaScript; raw HTML only has divs.
		panels := doc.Find(".resp-tabs-container").First().ChildrenFiltered("div").Not(".resp-accordion")
		if panels.Length() < index {
			return nil
		}
		return panels.Eq(index - 1)
	}
	if match := regexp.MustCompile(`/adminAkademik/([1-9])$`).FindStringSubmatch(sourcePath); len(match) == 2 {
		index, _ := strconv.Atoi(match[1])
		panels := doc.Find(".resp-tabs-container").First().ChildrenFiltered("div").Not(".resp-accordion")
		if panels.Length() < index {
			return nil
		}
		return panels.Eq(index - 1)
	}
	if main := doc.Find("main").First(); main.Length() > 0 {
		return main
	}
	sections := doc.Find("section").Not(".breadcrumb-classic")
	if sections.Length() > 0 {
		return sections
	}
	return doc.Find("body").First()
}

func publicSectionForPath(sourcePath string) string {
	switch {
	case strings.HasPrefix(sourcePath, "/jadwal/cariUas"):
		return "uas"
	case strings.HasPrefix(sourcePath, "/jadwal/cariUtama"):
		return "ujian-utama"
	case strings.HasPrefix(sourcePath, "/beritabaak"):
		return "berita"
	case strings.HasPrefix(sourcePath, "/adminAkademik/"):
		return "layanan-administrasi"
	case strings.HasPrefix(sourcePath, "/kuliahUjian/"):
		return "informasi-akademik"
	default:
		return "ringkasan"
	}
}

func publicReadableText(scope *goquery.Selection) string {
	var result strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			result.WriteString(node.Data)
			return
		}
		if node.Type != html.ElementNode {
			return
		}
		block := false
		switch node.Data {
		case "br", "p", "div", "section", "main", "article", "li", "h1", "h2", "h3", "h4", "h5", "h6", "tr":
			block = true
		case "select", "form":
			// Select options are represented separately. Preserve readable form headings.
			if node.Data == "select" {
				return
			}
		}
		if block {
			result.WriteByte('\n')
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if block {
			result.WriteByte('\n')
		} else {
			result.WriteByte(' ')
		}
	}
	for _, node := range scope.Nodes {
		walk(node)
	}
	lines := make([]string, 0)
	for _, line := range strings.Split(result.String(), "\n") {
		if text := cleanPublicText(line); text != "" {
			lines = append(lines, text)
		}
	}
	return strings.Join(lines, "\n")
}

func cleanPublicText(text string) string { return strings.Join(strings.Fields(text), " ") }

func safePublicURL(base *url.URL, href string) string {
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil || u.User != nil {
		return ""
	}
	u = base.ResolveReference(u)
	if u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	u.Fragment = ""
	query := u.Query()
	for key := range query {
		switch strings.ToLower(key) {
		case "_token", "token", "access_token", "session", "sessionid", "password":
			query.Del(key)
		}
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func publicLinks(scope *goquery.Selection, base *url.URL) []models.PublicLink {
	links := make([]models.PublicLink, 0)
	seen := make(map[string]bool)
	scope.Find("a[href], img[src], object[data], embed[src]").Each(func(_ int, link *goquery.Selection) {
		attr := "href"
		if link.Is("img, embed") {
			attr = "src"
		}
		if link.Is("object") {
			attr = "data"
		}
		href, _ := link.Attr(attr)
		if href == "" || strings.HasPrefix(href, "#") {
			return
		}
		resolved := safePublicURL(base, href)
		if resolved == "" || seen[resolved] {
			return
		}
		seen[resolved] = true
		u, _ := url.Parse(resolved)
		kind := "link"
		if link.Is("img") {
			kind = "image"
		}
		switch strings.ToLower(path.Ext(u.Path)) {
		case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".zip":
			kind = "document"
		}
		if strings.Contains(u.Path, "/downloadAkademik/") {
			kind = "document"
		}
		label := cleanPublicText(link.Text())
		if label == "" {
			label, _ = link.Attr("alt")
		}
		if label == "" {
			label = path.Base(u.Path)
		}
		links = append(links, models.PublicLink{Text: label, URL: resolved, Kind: kind})
	})
	return links
}

func publicTable(table *goquery.Selection) models.PublicTable {
	if table.Find(".st-key").Length() > 0 {
		return publicStackTable(table)
	}
	result := models.PublicTable{Headers: []string{}, Rows: [][]string{}, Records: []map[string]string{}}
	rows := table.Find("tr").FilterFunction(func(_ int, row *goquery.Selection) bool {
		return row.Closest("table").IsSelection(table)
	})
	rows.Each(func(_ int, row *goquery.Selection) {
		cells := row.ChildrenFiltered("th,td")
		if cells.Length() == 0 {
			return
		}
		values := make([]string, cells.Length())
		cells.Each(func(index int, cell *goquery.Selection) { values[index] = cleanPublicText(cell.Text()) })
		if len(result.Rows) == 0 && len(result.Headers) == 0 && row.ChildrenFiltered("th").Length() > 0 {
			result.Headers = values
			return
		}
		result.Rows = append(result.Rows, values)
		record := make(map[string]string, len(values))
		for index, value := range values {
			key := "column_" + strconv.Itoa(index+1)
			if index < len(result.Headers) {
				if header := publicColumnKey(result.Headers[index]); header != "" {
					key = header
				}
			}
			if _, exists := record[key]; exists {
				key += "_" + strconv.Itoa(index+1)
			}
			record[key] = value
		}
		result.Records = append(result.Records, record)
	})
	return result
}

func publicStackTable(table *goquery.Selection) models.PublicTable {
	result := models.PublicTable{Headers: []string{}, Rows: [][]string{}, Records: []map[string]string{}}
	var current map[string]string
	var key string
	finish := func() {
		if len(current) == 0 {
			return
		}
		values := make([]string, 0, len(current))
		for _, header := range result.Headers {
			values = append(values, current[header])
		}
		if len(values) == 0 {
			for field, value := range current {
				result.Headers = append(result.Headers, field)
				values = append(values, value)
			}
		}
		result.Rows = append(result.Rows, values)
		result.Records = append(result.Records, current)
		current = nil
	}
	table.Find("tr").FilterFunction(func(_ int, row *goquery.Selection) bool {
		return row.Closest("table").IsSelection(table)
	}).Each(func(_ int, row *goquery.Selection) {
		if row.Find(".st-head-row-main").Length() > 0 {
			finish()
			key = publicColumnKey(row.Text())
			if key != "" && len(result.Headers) == 0 {
				result.Headers = append(result.Headers, key)
			}
			return
		}
		if row.Find(".st-head-row").Length() > 0 {
			finish()
			if current == nil {
				current = make(map[string]string)
			}
			if key != "" {
				current[key] = cleanPublicText(row.Text())
			}
			return
		}
		field := cleanPublicText(row.Find(".st-key").Text())
		value := cleanPublicText(row.Find(".st-val").Text())
		if field == "" {
			return
		}
		if current == nil {
			current = make(map[string]string)
		}
		field = publicColumnKey(field)
		if len(result.Headers) == 0 || !containsString(result.Headers, field) {
			result.Headers = append(result.Headers, field)
		}
		current[field] = value
	})
	finish()
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func publicColumnKey(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(value), "_"))
	return strings.Trim(publicKeyPattern.ReplaceAllString(value, ""), "_")
}

func publicPagination(scope *goquery.Selection, base *url.URL) models.Pagination {
	page := 1
	if value, err := strconv.Atoi(base.Query().Get("page")); err == nil && value > 0 {
		page = value
	}
	pagination := models.Pagination{Page: page}
	for _, rel := range []string{"next", "prev", "previous"} {
		href, _ := scope.Find(`a[rel="` + rel + `"]`).First().Attr("href")
		if href == "" {
			continue
		}
		u, err := base.Parse(href)
		if err != nil || u.Host != base.Host || u.Path != base.Path {
			continue
		}
		value, err := strconv.Atoi(u.Query().Get("page"))
		if err != nil || value < 1 {
			continue
		}
		if rel == "next" {
			pagination.Next = value
		} else {
			pagination.Previous = value
		}
	}
	if match := publicTotalPattern.FindStringSubmatch(scope.Text()); len(match) == 2 {
		value, err := strconv.Atoi(match[1])
		if err == nil {
			pagination.Total = &value
		}
	}
	return pagination
}

func publicNews(scope *goquery.Selection, base *url.URL) []models.NewsItem {
	items := make([]models.NewsItem, 0)
	scope.Find("article.post-news").Each(func(_ int, item *goquery.Selection) {
		link := item.Find("h6 a[href], h3 a[href]").First()
		href, _ := link.Attr("href")
		resolved := safePublicURL(base, href)
		if resolved == "" {
			return
		}
		u, _ := url.Parse(resolved)
		id := path.Base(u.Path)
		if !publicIDPattern.MatchString(id) {
			return
		}
		items = append(items, models.NewsItem{
			ID: id, Title: cleanPublicText(link.Text()), URL: resolved,
			Date: cleanPublicText(item.Find(".post-news-meta").Text()),
		})
	})
	return items
}

func isPublicErrorPage(title string) bool {
	title = strings.ToLower(title)
	return strings.Contains(title, "server error") || strings.Contains(title, "just a moment") ||
		strings.Contains(title, "not found") || strings.Contains(title, "page expired")
}
