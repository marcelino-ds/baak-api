package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/models"
	"github.com/yafyx/baak-api/utils"
)

func TestHandlerUASUsesPublicPOSTContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/jadwal/cariUas" {
				t.Fatalf("GET path = %s", r.URL.Path)
			}
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "fixture", Path: "/"})
			_, _ = fmt.Fprint(w, `<html><title>Jadwal UAS</title><body><form><input name="teks"></form></body></html>`)
		case http.MethodPost:
			if err := r.ParseForm(); err != nil || r.Form.Get("teks") != "1IA01" {
				t.Fatalf("form = %#v, err = %v", r.Form, err)
			}
			if _, err := r.Cookie("session"); err != nil {
				t.Fatal("session cookie was not preserved")
			}
			_, _ = fmt.Fprint(w, `<html><title>Jadwal UAS</title><body><h3>Jadwal UAS</h3><table>`+
				`<tr><th>Hari</th><th>Tanggal</th><th>Mata Kuliah</th><th>Waktu</th></tr>`+
				`<tr><td>Selasa</td><td>28 Juli 2026</td><td>Algoritma</td><td>13.00 - 14.00</td></tr></table></body></html>`)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	previous := config.AppConfig
	config.AppConfig = config.Config{BaseURL: server.URL, CacheEnabled: false}
	t.Cleanup(func() { config.AppConfig = previous })

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/uas/1IA01", nil)
	HandlerPublicAlias("uas")(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Algoritma") {
		t.Fatalf("UAS response = %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data models.PublicPage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	metadata := envelope.Data.Metadata
	if metadata == nil || metadata.FetchedAt.IsZero() || metadata.ExpiresAt.Sub(metadata.FetchedAt) != 10*time.Minute {
		t.Fatalf("UAS freshness = %+v", metadata)
	}
}

func TestPublicRootAliasesReturnSeparateContent(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `<title>BAAK Online</title><header><a href="https://rps.gunadarma.ac.id/">Situs RPS</a>
		<a href="https://fti.gunadarma.ac.id/informatika">Informatika</a><a href="https://evil.example">Unrelated</a></header>
		<section><h3>Kalender Akademik</h3><table><tr><th>Kegiatan</th><th>Tanggal</th></tr><tr><td>Kuliah</td><td>2 Maret</td></tr></table>
		<a href="/downloadAkademik/13">Kalender Genap 2025</a>
		<h6>Pelayanan di Loket BAAK 1-8</h6><table><tr><th>Hari</th><th>Waktu</th></tr><tr><td>Senin</td><td>10.00-15.00</td></tr></table></section>`)
	}))
	defer server.Close()
	configureCacheTest(t, server.URL)
	for _, section := range []string{"ringkasan", "situs", "loket", "arsip-kalender"} {
		response := httptest.NewRecorder()
		HandlerPublic(response, httptest.NewRequest(http.MethodGet, "/public/"+section, nil))
		var envelope struct {
			Data models.PublicPage `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if response.Code != 200 || envelope.Data.Section != section {
			t.Fatalf("%s: %d %s", section, response.Code, response.Body.String())
		}
		switch section {
		case "situs":
			if len(envelope.Data.Links) != 2 || len(envelope.Data.Tables) != 0 || strings.Contains(envelope.Data.Text, "Kuliah") {
				t.Fatalf("situs = %+v", envelope.Data)
			}
		case "loket":
			if len(envelope.Data.Tables) != 1 || envelope.Data.Tables[0].Records[0]["hari"] != "Senin" || strings.Contains(envelope.Data.Text, "Kalender") {
				t.Fatalf("loket = %+v", envelope.Data)
			}
		case "arsip-kalender":
			if len(envelope.Data.Links) != 1 || len(envelope.Data.Tables) != 0 {
				t.Fatalf("arsip = %+v", envelope.Data)
			}
		}
	}
	if calls.Load() != 4 {
		t.Fatalf("section caches overlapped: %d source calls", calls.Load())
	}
}

func TestPublicLoketKeepsTheOfficialSidebarServiceTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<title>BAAK Online</title><main><div><h3>Kalender Akademik</h3><table>`+
			`<tr><th>Kegiatan</th><th>Tanggal</th></tr><tr><td>Kuliah</td><td>2 Maret</td></tr></table></div>`+
			`<aside><h6>Pelayanan di Loket BAAK 1-8</h6><table class="large-only">`+
			`<tr><th>Hari</th><th>Waktu</th></tr><tr><td>Senin-Kamis</td><td>10.00-15.00 WIB</td></tr>`+
			`</table></aside></main>`)
	}))
	defer server.Close()
	configureCacheTest(t, server.URL)
	response := httptest.NewRecorder()
	HandlerPublic(response, httptest.NewRequest(http.MethodGet, "/public/loket", nil))
	var result struct {
		Data models.PublicPage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(result.Data.Tables) != 1 || result.Data.Tables[0].Records[0]["hari"] != "Senin-Kamis" {
		t.Fatalf("loket sidebar table = %d %s", response.Code, response.Body.String())
	}
}

func TestPublicDocumentCategoryListsDownloads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/buku_pedoman" {
			t.Errorf("unexpected document resource: %s", r.URL.Path)
		}
		fmt.Fprint(w, `<title>BAAK Online</title><main><h3>Buku Pedoman</h3><a href="/file/Buku/pedoman.pdf">Pedoman</a>
		<a href="/beritabaak/1">Other news</a></main>`)
	}))
	defer server.Close()
	configureCacheTest(t, server.URL)
	response := httptest.NewRecorder()
	HandlerPublic(response, httptest.NewRequest(http.MethodGet, "/public/dokumen-buku-pedoman", nil))
	var result struct {
		Data models.PublicPage `json:"data"`
	}
	json.Unmarshal(response.Body.Bytes(), &result)
	if response.Code != 200 || len(result.Data.Links) != 1 || result.Data.Links[0].Kind != "document" {
		t.Fatalf("documents: %d %s", response.Code, response.Body.String())
	}
}

func TestPublicUnexpectedPageIsNotCached(t *testing.T) {
	var broken atomic.Bool
	broken.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if broken.Load() {
			fmt.Fprint(w, `<title>Server Error</title><p>Server Error</p>`)
			return
		}
		fmt.Fprint(w, `<title>Buku Pedoman</title><main><h3>Buku Pedoman</h3><a href="/file/pedoman.pdf">Pedoman</a></main>`)
	}))
	defer server.Close()
	configureCacheTest(t, server.URL)
	response := httptest.NewRecorder()
	HandlerPublic(response, httptest.NewRequest(http.MethodGet, "/public/buku-pedoman", nil))
	if response.Code != 502 || utils.GetCache().Stats().TotalItems != 0 {
		t.Fatalf("bad page cached: %d %s", response.Code, response.Body.String())
	}
	broken.Store(false)
	response = httptest.NewRecorder()
	HandlerPublic(response, httptest.NewRequest(http.MethodGet, "/public/buku-pedoman", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "pedoman.pdf") {
		t.Fatalf("recovery: %d %s", response.Code, response.Body.String())
	}
}

func TestHandlerPublicRejectsUnknownSection(t *testing.T) {
	previous := config.AppConfig
	config.AppConfig = config.Config{BaseURL: "https://baak.example"}
	t.Cleanup(func() { config.AppConfig = previous })

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/public/not-a-section", nil)
	HandlerPublic(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "NOT_FOUND") {
		t.Fatalf("unknown section = %d %s", response.Code, response.Body.String())
	}
}

func TestUASRequiresSearchAndRejectsFormOnlyResult(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `<title>BAAK Online</title><main><h3>Jadwal UAS</h3><form><input name="_token" value="fixture"></form></main>`)
	}))
	defer server.Close()
	configureCacheTest(t, server.URL)
	response := httptest.NewRecorder()
	HandlerPublicAlias("uas")(response, httptest.NewRequest(http.MethodGet, "/uas", nil))
	if response.Code != 400 || calls.Load() != 0 {
		t.Fatalf("missing class status=%d requests=%d", response.Code, calls.Load())
	}
	response = httptest.NewRecorder()
	HandlerPublicAlias("uas")(response, httptest.NewRequest(http.MethodGet, "/uas/1IA01", nil))
	if response.Code != 502 || utils.GetCache().Stats().TotalItems != 0 {
		t.Fatalf("form accepted as schedule: %d %s", response.Code, response.Body.String())
	}
}
