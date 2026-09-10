package utils

import (
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/yafyx/baak-api/models"
)

func parseNewsArticle(doc *goquery.Document, source *url.URL) (*models.NewsArticle, error) {
	id := path.Base(source.Path)
	if !strings.Contains(source.Path, "/beritabaak/") || !publicIDPattern.MatchString(id) {
		return nil, nil
	}
	scope := doc.Find("section .cell-md-8.text-left").First()
	if scope.Length() == 0 {
		scope = doc.Find("main article, article.news-detail").First()
	}
	if scope.Length() == 0 {
		return nil, errPublicPage
	}
	scope = scope.Clone()
	scope.Find("aside,script,style,iframe").Remove()
	title := cleanPublicText(scope.Find("h1,h2,h3").First().Text())
	meta := scope.Find(".post-news-meta li")
	date := cleanPublicText(meta.First().Text())
	published := ""
	if parsed, err := time.Parse("02/01/2006", date); err == nil {
		published = parsed.Format("2006-01-02")
	}
	author := cleanPublicText(meta.Eq(1).Text())
	content := scope.Find(".article-content").First()
	if content.Length() == 0 {
		content = scope.Find("div[style]").FilterFunction(func(_ int, node *goquery.Selection) bool {
			style, _ := node.Attr("style")
			for _, declaration := range strings.Split(style, ";") {
				key, value, ok := strings.Cut(declaration, ":")
				if ok && strings.EqualFold(strings.TrimSpace(key), "text-align") && strings.EqualFold(strings.TrimSpace(value), "justify") {
					return true
				}
			}
			return false
		}).First()
	}
	if content.Length() == 0 {
		return nil, errPublicPage
	}
	text := publicReadableText(content)
	if title == "" || text == "" {
		return nil, errPublicPage
	}
	attachments := make([]models.PublicLink, 0)
	for _, link := range publicLinks(content, source) {
		if link.Kind == "document" {
			attachments = append(attachments, link)
		}
	}
	return &models.NewsArticle{ID: id, Title: title, PublishedAt: published, Author: author, Content: text, Attachments: attachments}, nil
}
