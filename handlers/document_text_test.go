package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/models"
	"github.com/yafyx/baak-api/utils"
)

func TestDocumentTextEndpointCatalogDownloadExtractAndCache(t *testing.T) {
	var catalogRequests, pdfRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/buku_pedoman":
			catalogRequests.Add(1)
			fmt.Fprint(w, `<title>Buku Pedoman</title><main><h3>Buku Pedoman</h3><a href="/file/pedoman.pdf">Pedoman</a></main>`)
		case "/file/pedoman.pdf":
			pdfRequests.Add(1)
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(testTextPDF("Isi pedoman resmi"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureCacheTest(t, server.URL)

	catalogResponse := httptest.NewRecorder()
	HandlerDocuments(catalogResponse, httptest.NewRequest(http.MethodGet, "/dokumen/buku-pedoman", nil))
	var catalog struct {
		Success bool              `json:"success"`
		Data    models.PublicPage `json:"data"`
	}
	if err := json.Unmarshal(catalogResponse.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	if catalogResponse.Code != http.StatusOK || !catalog.Success || len(catalog.Data.Links) != 1 {
		t.Fatalf("catalog response = %d %s", catalogResponse.Code, catalogResponse.Body.String())
	}
	id := catalog.Data.Links[0].ID
	wantID := fmt.Sprintf("%x", sha256.Sum256([]byte(server.URL+"/file/pedoman.pdf")))
	if id != wantID || catalog.Data.Metadata == nil {
		t.Fatalf("catalog ID/metadata = %+v", catalog.Data)
	}
	if catalog.Data.Metadata.ExpiresAt.Sub(catalog.Data.Metadata.FetchedAt) != time.Hour {
		t.Fatalf("catalog freshness = %+v", catalog.Data.Metadata)
	}
	var firstBody string
	for range 2 {
		response := httptest.NewRecorder()
		HandlerDocuments(response, httptest.NewRequest(http.MethodGet, "/dokumen/buku-pedoman/"+id+"/teks", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("response = %d %s", response.Code, response.Body.String())
		}
		var envelope struct {
			Success bool               `json:"success"`
			Data    models.PDFDocument `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if !envelope.Success || envelope.Data.ID != id || envelope.Data.PageCount != 1 || len(envelope.Data.Pages) != 1 {
			t.Fatalf("document = %#v", envelope.Data)
		}
		if envelope.Data.Pages[0].Number != 1 || envelope.Data.Pages[0].Text != "Isi pedoman resmi" {
			t.Fatalf("page = %+v", envelope.Data.Pages[0])
		}
		if envelope.Data.Title != "Pedoman" || envelope.Data.Source != server.URL+"/file/pedoman.pdf" {
			t.Fatalf("source/title = %+v", envelope.Data)
		}
		metadata := envelope.Data.Metadata
		wantHash := fmt.Sprintf("%x", sha256.Sum256(testTextPDF("Isi pedoman resmi")))
		if metadata.FetchedAt.IsZero() || metadata.ExpiresAt.Sub(metadata.FetchedAt) != 24*time.Hour || metadata.ContentHash != wantHash {
			t.Fatalf("PDF freshness = %+v", metadata)
		}
		if response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("response headers = %v", response.Header())
		}
		if firstBody != "" && response.Body.String() != firstBody {
			t.Fatal("cached document or metadata changed between requests")
		}
		firstBody = response.Body.String()
	}
	if catalogRequests.Load() != 1 || pdfRequests.Load() != 1 {
		t.Fatalf("requests catalog=%d pdf=%d", catalogRequests.Load(), pdfRequests.Load())
	}
}

func TestDocumentTextEndpointRejectsUnknownAndExternalDocuments(t *testing.T) {
	var externalRequests, unexpectedRequests atomic.Int32
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		externalRequests.Add(1)
	}))
	defer external.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/buku_pedoman" {
			unexpectedRequests.Add(1)
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `<title>Buku Pedoman</title><main><h3>Buku Pedoman</h3><a href="%s/file/private.pdf">External</a>
		<a href="/adminAkademik/private.pdf">Private</a></main>`, external.URL)
	}))
	defer server.Close()
	configureCacheTest(t, server.URL)

	cases := []struct {
		name, path string
		status     int
	}{
		{name: "malformed id", path: "/dokumen/buku-pedoman/not-a-sha/teks", status: http.StatusBadRequest},
		{name: "not in catalog", path: "/dokumen/buku-pedoman/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/teks", status: http.StatusNotFound},
		{name: "unlisted document", path: "/dokumen/buku-pedoman/" + utils.DocumentID(server.URL+"/file/unlisted.pdf") + "/teks?url=" + server.URL + "/file/unlisted.pdf", status: http.StatusNotFound},
		{name: "external catalog link", path: "/dokumen/buku-pedoman/" + utils.DocumentID(external.URL+"/file/private.pdf") + "/teks", status: http.StatusUnprocessableEntity},
		{name: "private catalog path", path: "/dokumen/buku-pedoman/" + utils.DocumentID(server.URL+"/adminAkademik/private.pdf") + "/teks", status: http.StatusUnprocessableEntity},
		{name: "unsupported category", path: "/dokumen/pribadi/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/teks", status: http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			HandlerDocuments(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if response.Code != tc.status {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
	if externalRequests.Load() != 0 || unexpectedRequests.Load() != 0 {
		t.Fatal("a document outside the catalog/origin/path allowlist received a request")
	}
}

func TestDocumentTextEndpointDoesNotCacheExtractionErrors(t *testing.T) {
	for _, scenario := range []struct {
		name, body, code string
		upstreamStatus   int
		status           int
	}{
		{name: "HTML", body: "<html>Cloudflare</html>", status: 422, code: "PDF_UNSUPPORTED"},
		{name: "broken PDF", body: "%PDF-1.4\nbroken", status: 422, code: "PDF_UNSUPPORTED"},
		{name: "upstream error", upstreamStatus: 503, status: 502, code: "UPSTREAM_ERROR"},
		{name: "blocked download", upstreamStatus: 403, status: 503, code: "CLOUDFLARE_BLOCKED"},
		{name: "missing runtime", status: 503, code: "PDF_UNAVAILABLE"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			python := os.Getenv("PDF_PYTHON")
			if scenario.name == "missing runtime" {
				t.Setenv("PDF_PYTHON", filepath.Join(t.TempDir(), "missing-python"))
			}
			var valid atomic.Bool
			var pdfRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/buku_pedoman":
					fmt.Fprint(w, `<title>Buku Pedoman</title><main><h3>Buku Pedoman</h3><a href="/file/pedoman.pdf">Pedoman</a></main>`)
				case "/file/pedoman.pdf":
					pdfRequests.Add(1)
					if !valid.Load() {
						if scenario.upstreamStatus != 0 {
							w.WriteHeader(scenario.upstreamStatus)
							return
						}
						if scenario.body != "" {
							fmt.Fprint(w, scenario.body)
							return
						}
					}
					_, _ = w.Write(testTextPDF("Recovered"))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			configureCacheTest(t, server.URL)
			id := utils.DocumentID(server.URL + "/file/pedoman.pdf")
			path := "/dokumen/buku-pedoman/" + id + "/teks"
			response := httptest.NewRecorder()
			HandlerDocuments(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != scenario.status || !strings.Contains(response.Body.String(), scenario.code) {
				t.Fatalf("error status = %d %s", response.Code, response.Body.String())
			}
			valid.Store(true)
			if scenario.name == "missing runtime" {
				t.Setenv("PDF_PYTHON", python)
			}
			response = httptest.NewRecorder()
			HandlerDocuments(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Recovered") {
				t.Fatalf("recovery response = %d %s", response.Code, response.Body.String())
			}
			if pdfRequests.Load() != 2 {
				t.Fatalf("PDF error was cached, requests=%d", pdfRequests.Load())
			}
		})
	}
}

func TestDocumentTextEndpointRejectsNonGET(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		response := httptest.NewRecorder()
		HandlerDocuments(response, httptest.NewRequest(method, "/dokumen/buku-pedoman/id/teks", nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, OPTIONS" {
			t.Fatalf("response = %d %s allow=%q", response.Code, response.Body.String(), response.Header().Get("Allow"))
		}
	}
}

func TestDocumentTextEndpointUsesCategoryCatalog(t *testing.T) {
	for _, scenario := range []struct {
		category, resource, html string
	}{
		{category: "mata-kuliah", resource: "/kuliahUjian/2",
			html: `<div class="resp-tabs-container"><div>Kalender</div><div><h3>Mata Kuliah</h3><a href="/file/pedoman.pdf">Dokumen</a></div></div>`},
		{category: "frs", resource: "/kuliahUjian/9",
			html: `<div class="resp-tabs-container">` + strings.Repeat(`<div>Panel lain</div>`, 8) +
				`<div><h3>FRS</h3><a href="/file/pedoman.pdf">Dokumen</a></div></div>`},
		{category: "kalender", resource: "/",
			html: `<main><h3>Kalender</h3><a href="/file/pedoman.pdf">Kalender</a></main>`},
	} {
		t.Run(scenario.category, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("upstream method = %s", r.Method)
				}
				switch r.URL.Path {
				case scenario.resource:
					fmt.Fprint(w, `<title>BAAK Online</title>`+scenario.html)
				case "/file/pedoman.pdf":
					_, _ = w.Write(testTextPDF("Dokumen resmi"))
				default:
					t.Errorf("wrong category source: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			configureCacheTest(t, server.URL)
			id := fmt.Sprintf("%x", sha256.Sum256([]byte(server.URL+"/file/pedoman.pdf")))
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/dokumen/"+scenario.category+"/"+id+"/teks", nil)
			HandlerDocuments(response, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Dokumen resmi") {
				t.Fatalf("category response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestDocumentTextEndpointHonorsDisabledCache(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/buku_pedoman" {
			fmt.Fprint(w, `<main><h3>Pedoman</h3><a href="/file/pedoman.pdf">Pedoman</a></main>`)
			return
		}
		_, _ = w.Write(testTextPDF(fmt.Sprintf("Revisi %d", calls.Add(1))))
	}))
	defer server.Close()
	configureCacheTest(t, server.URL)
	config.AppConfig.CacheEnabled = false
	id := utils.DocumentID(server.URL + "/file/pedoman.pdf")
	for _, want := range []string{"Revisi 1", "Revisi 2"} {
		response := httptest.NewRecorder()
		HandlerDocuments(response, httptest.NewRequest(http.MethodGet, "/dokumen/buku-pedoman/"+id+"/teks", nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), want) {
			t.Fatalf("disabled cache response = %d %s", response.Code, response.Body.String())
		}
	}
	if utils.GetCache().Stats().TotalItems != 0 {
		t.Fatal("disabled cache stored a catalog or PDF")
	}
}

func testTextPDF(text string) []byte {
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	stream := "BT /F1 12 Tf 50 750 Td (" + text + ") Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
	}
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return []byte(b.String())
}
