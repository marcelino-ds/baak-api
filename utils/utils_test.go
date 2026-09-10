package utils

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestScraperHonorsCanceledContext(t *testing.T) {
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	scraper := &Scraper{
		BaseURL: "https://baak.example",
		client:  &http.Client{Transport: transport},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := scraper.FetchDocument(ctx, "https://baak.example/jadwal")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("FetchDocument error = %v, want context deadline exceeded", err)
	}
}

func TestNewScraperNormalizesBaseURLAndRejectsCredentials(t *testing.T) {
	scraper, err := NewScraper("https://baak.example///")
	if err != nil || scraper.BaseURL != "https://baak.example" {
		t.Fatalf("normalized URL = %q, %v", scraper.BaseURL, err)
	}
	if _, err := NewScraper("https://user:secret@baak.example"); err == nil {
		t.Fatal("accepted credentials in base URL")
	}
}

func TestScraperPreservesCookiesWhenFallingBackToFlareSolverr(t *testing.T) {
	var flareRequest string
	flare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		flareRequest = string(body)
		_, _ = w.Write([]byte(`{"status":"ok","solution":{"status":200,"response":"<html>solved</html>"}}`))
	}))
	defer flare.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "fixture", Path: "/"})
		w.WriteHeader(http.StatusForbidden)
	}))
	defer upstream.Close()

	scraper := &Scraper{
		BaseURL:      upstream.URL,
		client:       upstream.Client(),
		flareSolverr: &FlareSolverr{url: flare.URL, client: flare.Client()},
	}

	if _, err := scraper.FetchDocument(context.Background(), upstream.URL+"/jadwal"); err != nil {
		t.Fatalf("FetchDocument: %v", err)
	}
	if !strings.Contains(flareRequest, `"name":"session"`) || !strings.Contains(flareRequest, `"value":"fixture"`) {
		t.Fatalf("FlareSolverr request did not preserve upstream cookie: %s", flareRequest)
	}
}

func TestConvertWaktuToJam(t *testing.T) {
	periods := [][]string{
		{"07:30", "08:20"},
		{"08:30", "09:20"},
		{"09:30", "10:20"},
		{"10:30", "11:20"},
	}
	tests := []struct {
		name  string
		waktu string
		want  string
	}{
		{name: "single period", waktu: "2", want: "08:30 - 09:20"},
		{name: "range", waktu: "1-3", want: "07:30 - 10:20"},
		{name: "four periods", waktu: "1/2/3/4", want: "07:30 - 11:20"},
		{name: "reversed range", waktu: "3-1", want: ""},
		{name: "out of bounds", waktu: "1-5", want: ""},
		{name: "unavailable", waktu: "-", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convertWaktuToJam(tt.waktu, periods); got != tt.want {
				t.Errorf("convertWaktuToJam(%q) = %q, want %q", tt.waktu, got, tt.want)
			}
		})
	}
}

func TestParseTanggalHandlesBAAKDateRanges(t *testing.T) {
	tests := []struct {
		text  string
		start string
		end   string
	}{
		{text: "2 Maret \u2013 14 Maret 2026", start: "2 Maret", end: "14 Maret 2026"},
		{text: "27 Juli \u2013 8 Agustus 2026", start: "27 Juli", end: "8 Agustus 2026"},
		{text: "2 Maret - 14 Maret 2026", start: "2 Maret", end: "14 Maret 2026"},
		{text: "2 Maret \u2014 14 Maret 2026", start: "2 Maret", end: "14 Maret 2026"},
		{text: "16 Mei 2026", start: "16 Mei 2026", end: "16 Mei 2026"},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			start, end := parseTanggal(tt.text)
			if start != tt.start || end != tt.end {
				t.Errorf("date range = (%q, %q), want (%q, %q)", start, end, tt.start, tt.end)
			}
		})
	}
}

func TestFetchRejectsCloudflareChallengeWithOKStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><title>Just a moment...</title><div id="cf-challenge"></div></html>`))
	}))
	defer upstream.Close()

	if _, err := FetchDocumentWithRetry(upstream.URL, upstream.URL, 1); err == nil {
		t.Fatal("Cloudflare challenge was accepted as a successful document")
	}
}

func TestFetchRejectsUntrustedTLSCertificate(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><title>Untrusted</title></html>"))
	}))
	defer upstream.Close()

	if _, err := FetchDocumentWithRetry(upstream.URL, upstream.URL, 1); err == nil {
		t.Fatal("document fetched using an untrusted TLS certificate")
	}
}

func TestParsersRejectPagesWithoutExpectedTables(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><title>Server Error</title><p>Something went wrong</p></html>`))
	}))
	defer upstream.Close()
	scraper := &Scraper{BaseURL: upstream.URL, client: upstream.Client()}

	parsers := []struct {
		name string
		load func() error
	}{
		{name: "UTS", load: func() error { _, err := scraper.GetUTS(context.Background(), upstream.URL); return err }},
		{name: "jadwal", load: func() error { _, err := scraper.GetJadwal(context.Background(), upstream.URL); return err }},
		{name: "kalender", load: func() error { _, err := scraper.GetKegiatan(context.Background()); return err }},
		{name: "time slots", load: func() error { _, err := scraper.GetTimeStampLUT(context.Background()); return err }},
		{name: "kelas baru", load: func() error { _, err := scraper.GetKelasbaru(context.Background(), upstream.URL+"?teks=x"); return err }},
		{name: "mahasiswa baru", load: func() error {
			_, err := scraper.GetMahasiswaBaru(context.Background(), upstream.URL+"?teks=x")
			return err
		}},
	}
	for _, parser := range parsers {
		t.Run(parser.name, func(t *testing.T) {
			err := parser.load()
			if err == nil {
				t.Fatal("parser accepted a page without a result table")
			}
			response := httptest.NewRecorder()
			WriteHTTPError(response, err)
			if response.Code != http.StatusBadGateway {
				t.Errorf("unexpected page status = %d, want 502: %v", response.Code, err)
			}
		})
	}
}

func TestStudentParsersAcceptExplicitEmptyResults(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><title>BAAK Online</title>` +
			`<h5>Input <b>missing</b> Berdasarkan Nama Tidak Ada Dalam Database!</h5></html>`))
	}))
	defer upstream.Close()
	scraper := &Scraper{BaseURL: upstream.URL, client: upstream.Client()}
	classes, err := scraper.GetKelasbaru(context.Background(), upstream.URL+"?teks=missing")
	if err != nil || len(classes) != 0 {
		t.Fatalf("empty class search: %v, %v", classes, err)
	}
	students, err := scraper.GetMahasiswaBaru(context.Background(), upstream.URL+"?teks=missing")
	if err != nil || len(students) != 0 {
		t.Fatalf("empty student search: %v, %v", students, err)
	}
}

func TestJadwalParserAcceptsEmptyTable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/kuliahUjian/6" {
			_, _ = w.Write([]byte(`<table class="cell-xs-6"><tr><td>Jam ke - 1</td><td>07.30 - 08.30</td></tr></table>`))
			return
		}
		_, _ = w.Write([]byte(`<table><tr><th>KELAS</th><th>HARI</th><th>MATA KULIAH</th>` +
			`<th>WAKTU</th><th>RUANG</th><th>DOSEN</th></tr></table>`))
	}))
	defer upstream.Close()
	scraper := &Scraper{BaseURL: upstream.URL, client: upstream.Client()}
	jadwal, err := scraper.GetJadwal(context.Background(), upstream.URL)
	if err != nil {
		t.Fatalf("empty schedule: %v", err)
	}
	count := len(jadwal.Senin) + len(jadwal.Selasa) + len(jadwal.Rabu) + len(jadwal.Kamis) + len(jadwal.Jumat) + len(jadwal.Sabtu)
	if count != 0 {
		t.Fatalf("empty schedule has %d rows", count)
	}
}
