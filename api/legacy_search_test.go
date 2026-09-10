package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yafyx/baak-api/config"
)

func TestLegacyPublicStudentSearchFormsRemainReadOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cariMhsBaru" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("_token") == "" {
			fmt.Fprint(w, `<form><input name="_token" value="fixture-token"></form>`)
			return
		}
		if r.URL.Query().Get("tipeMhsBaru") != "Kelas" || r.URL.Query().Get("teks") != "1IA01" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `<table><tr><th>No</th><th>No Pend</th><th>Nama</th><th>NPM</th><th>Kelas</th><th>Keterangan</th></tr>
		<tr><td>1</td><td>2025-1</td><td>Ani</td><td>123</td><td>1IA01</td><td>Aktif</td></tr></table>`)
	}))
	defer server.Close()
	previousConfig, previousError := config.AppConfig, configurationError
	config.AppConfig = config.Config{BaseURL: server.URL, CacheEnabled: false}
	configurationError = nil
	t.Cleanup(func() { config.AppConfig, configurationError = previousConfig, previousError })

	response := httptest.NewRecorder()
	handleRoutes(response, httptest.NewRequest(http.MethodGet, "/cariMhsBaru?tipeMhsBaru=Kelas&teks=1IA01", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"kelas":"1IA01"`) {
		t.Fatalf("legacy student search = %d %s", response.Code, response.Body.String())
	}
}

func TestLegacyPublicStudentSearchRejectsNonGET(t *testing.T) {
	response := httptest.NewRecorder()
	handleRoutes(response, httptest.NewRequest(http.MethodPost, "/cariMhsBaru?teks=1IA01", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("legacy method status = %d %s", response.Code, response.Body.String())
	}
}

func TestLegacyPublicClassSearchAcceptsOfficialQueryContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cariKelasBaru" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("_token") == "" {
			fmt.Fprint(w, `<form><input name="_token" value="fixture-token"></form>`)
			return
		}
		if r.URL.Query().Get("tipeKelasBaru") != "Kelas" || r.URL.Query().Get("teks") != "1IA01" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `<table><tr><th>No</th><th>NPM</th><th>Nama</th><th>Kelas Lama</th><th>Kelas Baru</th></tr>
		<tr><td>1</td><td>123</td><td>Ani</td><td>1IA01</td><td>1IA02</td></tr></table>`)
	}))
	defer server.Close()
	previousConfig, previousError := config.AppConfig, configurationError
	config.AppConfig = config.Config{BaseURL: server.URL, CacheEnabled: false}
	configurationError = nil
	t.Cleanup(func() { config.AppConfig, configurationError = previousConfig, previousError })

	response := httptest.NewRecorder()
	handleRoutes(response, httptest.NewRequest(http.MethodGet, "/cariKelasBaru?tipeKelasBaru=Kelas&teks=1IA01", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"kelas_baru":"1IA02"`) {
		t.Fatalf("legacy class search = %d %s", response.Code, response.Body.String())
	}
}
