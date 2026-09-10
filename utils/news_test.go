package utils

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestNewsDetailHasArticleFieldsWithoutSidebar(t *testing.T) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<title>BAAK Online</title><body>
	<section class="breadcrumb-classic"><h3>Berita</h3></section><section><div class="cell-sm-8 cell-md-8 text-left">
	<h3>Pengumuman Akademik</h3><ul class="post-news-meta"><li><span>21/08/2026</span></li><li><span>Admin</span></li></ul>
	<div class="offset-md-top-20" style="text-align:justify">Paragraf pertama.<br><br>Paragraf kedua.
	<a href="/file/pengumuman.pdf">Lampiran</a></div></div><aside>Berita lain</aside></section></body>`))
	page, err := ParsePublicPage(doc, "https://baak.example/beritabaak/759")
	if err != nil {
		t.Fatal(err)
	}
	if page.Article == nil || page.Article.ID != "759" || page.Article.PublishedAt != "2026-08-21" || page.Article.Author != "Admin" {
		t.Fatalf("article: %+v", page.Article)
	}
	if strings.Contains(page.Article.Content, "Berita lain") || !strings.Contains(page.Article.Content, "Paragraf kedua.") {
		t.Fatalf("content: %s", page.Article.Content)
	}
	if len(page.Article.Attachments) != 1 {
		t.Fatalf("attachments: %+v", page.Article.Attachments)
	}
}

func TestNewsDetailPrefersArticleContentOverLayoutJustify(t *testing.T) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<title>BAAK Online</title><body>
	<section><div class="cell-md-8 text-left"><h3>Pengumuman</h3>
	<div style="display:flex;justify-content:space-between"><span>Layout label</span></div>
	<div class="article-content"><p>Isi resmi berita.</p></div></div></section></body>`))
	page, err := ParsePublicPage(doc, "https://baak.example/beritabaak/760")
	if err != nil {
		t.Fatal(err)
	}
	if page.Article == nil || page.Article.Content != "Isi resmi berita." {
		t.Fatalf("article content = %#v", page.Article)
	}
}
