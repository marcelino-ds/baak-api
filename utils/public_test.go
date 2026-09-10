package utils

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestParsePublicPageExtractsReadableContent(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<html><head><title>Daftar Dosen</title></head><body>
		<a href="/download/file.pdf"> Unduh </a>
		<table><tr><th>No</th><th>Nama</th></tr><tr><td>1</td><td>Ada</td></tr></table>
		<table><tr><td>Jam</td><td>09.00</td></tr></table>
		</body></html>`))
	if err != nil {
		t.Fatal(err)
	}

	page, err := ParsePublicPage(doc, "https://baak.example/beritabaak")
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "Daftar Dosen" {
		t.Fatalf("title = %q", page.Title)
	}
	if len(page.Links) != 1 || page.Links[0].URL != "https://baak.example/download/file.pdf" {
		t.Fatalf("links = %+v", page.Links)
	}
	if len(page.Tables) != 2 || len(page.Tables[0].Rows) != 1 || page.Tables[0].Rows[0][1] != "Ada" {
		t.Fatalf("tables = %+v", page.Tables)
	}
	if page.Text == "" || strings.Contains(page.Text, "\n\n\n") {
		t.Fatalf("unexpected page text = %q", page.Text)
	}
}

func TestPublicPageSelectsRequestedPanel(t *testing.T) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<title>BAAK Online</title><body>
	<header>Navigation</header><div class="resp-tabs-container">
	<div class="resp-tab-content"><h4>Daftar Ulang</h4><p>Wrong panel</p></div>
	<div class="resp-tab-content"><h4>Cuti Akademik</h4><p>First paragraph.</p><p>Second paragraph.</p>
	<a href="/file/cuti.pdf">Formulir</a><a href="javascript:alert(1)">Unsafe</a>
	<a href="https://user:secret@evil.example/file.pdf">Unsafe credential</a></div>
	<div class="resp-tab-content"><h4>Tidak Aktif</h4><p>Private hidden panel</p></div></div><footer>Footer</footer></body>`))
	page, err := ParsePublicPage(doc, "https://baak.example/adminAkademik/2")
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "Cuti Akademik" || strings.Contains(page.Text, "Wrong") || strings.Contains(page.Text, "hidden") {
		t.Fatalf("wrong panel: %+v", page)
	}
	if !strings.Contains(page.Text, "First paragraph.\nSecond paragraph.") {
		t.Fatalf("paragraphs lost: %q", page.Text)
	}
	if len(page.Links) != 1 || page.Links[0].Kind != "document" {
		t.Fatalf("unsafe links: %+v", page.Links)
	}
}

func TestPublicPagePaginationOptionsAndRecords(t *testing.T) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(publicTabs(4, `<main>
	<h4>Koordinator Mata Kuliah</h4><select name="jurusan"><option value="">Pilih</option>
	<option value="S1  IF">Informatika</option></select><table><tr><th>No</th><th>Mata Kuliah</th><th>Kelas</th><th>Dosen</th></tr>
	<tr><td>21</td><td>Algoritma</td><td>2IA</td><td>Dr. Ada</td></tr></table>
	<div>Total Records : 637</div><a rel="prev" href="?page=1">Previous</a><a rel="next" href="?page=3">Next</a>
	</main>`)))
	page, err := ParsePublicPage(doc, "https://baak.example/kuliahUjian/4?page=2")
	if err != nil {
		t.Fatal(err)
	}
	if page.Pagination.Page != 2 || page.Pagination.Next != 3 || page.Pagination.Total == nil || *page.Pagination.Total != 637 {
		t.Fatalf("pagination = %+v", page.Pagination)
	}
	if page.Tables[0].Records[0]["mata_kuliah"] != "Algoritma" {
		t.Fatalf("records = %+v", page.Tables)
	}
	if page.Options["jurusan"][0].Value != "S1  IF" {
		t.Fatalf("option value was modified: %+v", page.Options)
	}
}

func publicTabs(selected int, content string) string {
	var result strings.Builder
	result.WriteString(`<title>BAAK Online</title><body><div class="resp-tabs-container">`)
	for index := 1; index <= 9; index++ {
		result.WriteString(`<div class="resp-tab-content">`)
		if index == selected {
			result.WriteString(content)
		} else {
			fmt.Fprintf(&result, "<h4>Other tab %d</h4><p>Unrelated content</p>", index)
		}
		result.WriteString(`</div>`)
	}
	result.WriteString(`</div></body>`)
	return result.String()
}

func TestPublicPageDoesNotExposeHiddenPanelsAndNavigation(t *testing.T) {
	content := `<h4>Daftar Dosen Wali Kelas</h4><table><tr><th>No</th><th>Kelas</th><th>Dosen</th></tr>
	<tr><td>1</td><td>2IA01</td><td>Dr. Ada</td></tr></table>`
	for _, rendered := range []bool{false, true} {
		html := publicTabs(3, content)
		if !rendered {
			html = strings.ReplaceAll(html, ` class="resp-tab-content"`, "")
		}
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
		page, err := ParsePublicPage(doc, "https://baak.example/kuliahUjian/3/2?page=2")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(page.Text, "Unrelated") || len(page.Tables) != 1 {
			t.Fatalf("hidden content leaked: %+v", page)
		}
	}
}

func TestPublicPageNewsAndDocuments(t *testing.T) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<title>BAAK Online</title><body><main><h3>Berita</h3>
	<article class="post-news"><h6><a href="https://baak.example/beritabaak/759">Pengumuman</a></h6>
	<div class="post-news-meta">21/08/2026</div></article><a href="/file/FRS.doc">FRS</a>
	<a href="/downloadAkademik/13?_token=secret">Kalender</a>
	<iframe src="https://example.com/track"></iframe></main></body>`))
	page, err := ParsePublicPage(doc, "https://baak.example/beritabaak?page=1")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.News) != 1 || page.News[0].ID != "759" || page.News[0].URL != "https://baak.example/beritabaak/759" {
		t.Fatalf("news = %+v", page.News)
	}
	if page.Links[1].Kind != "document" || strings.Contains(page.Links[2].URL, "secret") {
		t.Fatalf("documents = %+v", page.Links)
	}
}

func TestPublicPageParsesStacktableRecords(t *testing.T) {
	html := publicTabs(3, `<h4>Dosen Wali</h4><table class="stacktable large-only stacktable small-only">
	<tr><th class="st-head-row st-head-row-main" colspan="2">KELAS</th></tr><tr><th class="st-head-row" colspan="2">2IA01</th></tr>
	<tr><td class="st-key">DOSEN</td><td class="st-val">Dr. Ada</td></tr>
	<tr><td class="st-key">SEMESTER</td><td class="st-val">Genap</td></tr></table>`)
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	page, err := ParsePublicPage(doc, "https://baak.example/kuliahUjian/3")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tables) != 1 || len(page.Tables[0].Records) != 1 {
		t.Fatalf("stacktable records = %+v", page.Tables)
	}
	record := page.Tables[0].Records[0]
	if record["kelas"] != "2IA01" || record["dosen"] != "Dr. Ada" || record["semester"] != "Genap" {
		t.Fatalf("stacktable record = %+v", record)
	}
}

func TestPublicStacktableKeepsAllRowsAndDeduplicatesDesktopCopy(t *testing.T) {
	content := `<h4>Dosen Wali</h4><table class="large-only small-only">
	<tr><th class="st-head-row-main">KELAS</th></tr>
	<tr><th class="st-head-row">1IA01</th></tr><tr><td class="st-key">DOSEN</td><td class="st-val">Ada</td></tr>
	<tr><th class="st-head-row">1IA02</th></tr><tr><td class="st-key">DOSEN</td><td class="st-val">Budi</td></tr></table>
	<table class="large-only"><tr><th>KELAS</th><th>DOSEN</th></tr>
	<tr><td>1IA01</td><td>Ada</td></tr><tr><td>1IA02</td><td>Budi</td></tr></table>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(publicTabs(3, content)))
	page, err := ParsePublicPage(doc, "https://baak.example/kuliahUjian/3")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tables) != 1 || len(page.Tables[0].Records) != 2 {
		t.Fatalf("duplicate/lost rows: %+v", page.Tables)
	}
	if page.Tables[0].Records[0]["dosen"] != "Ada" || page.Tables[0].Records[1]["dosen"] != "Budi" {
		t.Fatalf("wrong groups: %+v", page.Tables)
	}
}

func TestPublicPageRejectsErrorAndMissingPanels(t *testing.T) {
	for _, html := range []string{
		`<title>Server Error</title><body>Server Error</body>`,
		`<title>BAAK Online</title><body><nav>Home</nav></body>`,
		`<title>BAAK Online</title><body><div class="resp-tabs-container"><div>Only the wrong tab</div></div></body>`,
	} {
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
		_, err := ParsePublicPage(doc, "https://baak.example/kuliahUjian/5")
		if !errors.Is(err, ErrUnexpectedPage) {
			t.Fatalf("error page accepted: %v", err)
		}
	}
}

func TestPublicPathUsesVerifiedAdministrationIDs(t *testing.T) {
	for section, expected := range map[string]string{"pengecekan-nilai": "/adminAkademik/4", "pindah-lokasi": "/adminAkademik/5", "pindah-jurusan": "/adminAkademik/6", "berita": "/beritabaak"} {
		path, ok := PublicPath(section, "")
		if !ok || path != expected {
			t.Errorf("%s: got %q, want %q", section, path, expected)
		}
	}
	for _, parameter := range []string{"../5", "%2f5", "0", "6", "1?search_pi=x"} {
		if path, ok := PublicPath("dosen-wali", parameter); ok {
			t.Errorf("unsafe parameter accepted: %q", path)
		}
	}
}

func TestPublicPathAllowlistRejectsUnknownSections(t *testing.T) {
	if _, ok := PublicPath("not-a-section", ""); ok {
		t.Fatal("unknown public section was accepted")
	}
	if path, ok := PublicPath("berita", ""); !ok || path != "/beritabaak" {
		t.Fatalf("news path = %q, %t", path, ok)
	}
	if path, ok := PublicPath("dosen-wali", "2"); !ok || path != "/kuliahUjian/3/2" {
		t.Fatalf("teacher path = %q, %t", path, ok)
	}
}
