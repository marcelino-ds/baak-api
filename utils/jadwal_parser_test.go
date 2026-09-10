package utils

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJadwalFindsTableByColumnHeaders(t *testing.T) {
	const navigation = `<table><tr><td>Portal navigation</td></tr></table>`
	const headers = `<tr><th>KELAS</th><th>HARI</th><th>MATA KULIAH</th>` +
		`<th>WAKTU</th><th>RUANG</th><th>DOSEN</th></tr>`
	const course = `<tr><td>1IA01</td><td>Senin</td><td>Algoritma</td>` +
		`<td>1</td><td>D201</td><td>Dr. Ada</td></tr>`
	tests := []struct {
		name string
		html string
	}{
		{name: "table after navigation", html: navigation + `<table>` + headers + course + `</table>`},
		{
			name: "table before navigation",
			html: `<table>` + headers + course + `</table>` + navigation,
		},
		{
			name: "mixed case whitespace and td headers",
			html: navigation + `<table><tr><td>Kelas</td><td> Hari </td><td>Mata&nbsp; Kuliah</td>` +
				`<td>Waktu</td><td>ruang</td><td>DOSEN</td></tr>` + course + `</table>`,
		},
		{
			name: "reordered columns with an extra column",
			html: `<table><thead><tr><th>DOSEN</th><th>KELAS</th><th>RUANG</th><th>HARI</th>` +
				`<th>WAKTU</th><th>MATA KULIAH</th><th>CATATAN</th></tr></thead>` +
				`<tbody><tr><td>Dr. Ada</td><td>1IA01</td><td>D201</td><td>Senin</td>` +
				`<td>1</td><td>Algoritma</td><td>Catatan</td></tr></tbody></table>`,
		},
		{
			name: "schedule nested in layout table",
			html: `<table><tr><td>` + navigation + `<table>` + headers + course + `</table></td></tr></table>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scraper := newJadwalParserScraper(t, tt.html)
			jadwal, err := scraper.GetJadwal(context.Background(), scraper.BaseURL)
			if err != nil {
				t.Fatal(err)
			}
			if len(jadwal.Senin) != 1 {
				t.Fatalf("Monday course count = %d, want 1", len(jadwal.Senin))
			}
			got := jadwal.Senin[0]
			if got.Nama != "Algoritma" || got.Dosen != "Dr. Ada" || got.Ruang != "D201" {
				t.Errorf("incorrect course columns: %+v", got)
			}
			if got.Waktu != "1" || got.Jam != "07:30 - 08:20" {
				t.Errorf("incorrect schedule time: %+v", got)
			}
		})
	}
}

func TestJadwalRejectsUnrecognizedOrMalformedTables(t *testing.T) {
	const headers = `<tr><th>KELAS</th><th>HARI</th><th>MATA KULIAH</th>` +
		`<th>WAKTU</th><th>RUANG</th><th>DOSEN</th></tr>`
	tests := []struct {
		name string
		html string
	}{
		{name: "unrelated table", html: `<table><tr><td>Portal navigation</td></tr></table>`},
		{name: "missing columns", html: `<table><tr><th>HARI</th><th>MATA KULIAH</th></tr></table>`},
		{
			name: "truncated course row",
			html: `<table>` + headers + `<tr><td>1IA01</td><td>Senin</td><td>Algoritma</td></tr></table>`,
		},
		{
			name: "unrecognized day is not an empty result",
			html: `<table>` + headers + `<tr><td>1IA01</td><td>UnexpectedDay</td><td>Algoritma</td>` +
				`<td>1</td><td>D201</td><td>Dr. Ada</td></tr></table>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scraper := newJadwalParserScraper(t, tt.html)
			_, err := scraper.GetJadwal(context.Background(), scraper.BaseURL)
			if !errors.Is(err, ErrUnexpectedPage) {
				t.Fatalf("malformed schedule error = %v, want ErrUnexpectedPage", err)
			}
		})
	}
}

func newJadwalParserScraper(t *testing.T, html string) *Scraper {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/kuliahUjian/6" {
			_, _ = io.WriteString(w, `<table class="cell-xs-6"><tr><td>1</td><td>07.30 - 08.20</td></tr></table>`)
			return
		}
		_, _ = io.WriteString(w, html)
	}))
	t.Cleanup(server.Close)
	return &Scraper{BaseURL: server.URL, client: server.Client(), breaker: NewCircuitBreaker()}
}
