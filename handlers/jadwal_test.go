package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yafyx/baak-api/config"
)

func TestHandlerJadwalUsesOneSessionForTokenAndSchedule(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "fixture", Path: "/"})
			_, _ = w.Write([]byte(`<html><input type="hidden" name="_token" value="fixture-token"></html>`))
		case "/jadwal/cariJadKul":
			sessionCookie, _ := r.Cookie("session")
			if sessionCookie == nil || r.URL.Query().Get("_token") != "fixture-token" || r.URL.Query().Get("teks") != "1IA01" || sessionCookie.Value != "fixture" {
				http.Error(w, "bad search parameters", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`<table><tr><th>KELAS</th><th>HARI</th><th>MATA KULIAH</th>` +
				`<th>WAKTU</th><th>RUANG</th><th>DOSEN</th></tr>` +
				`<tr><td>1</td><td>Senin</td><td>Algoritma</td><td>1</td><td>D201</td><td>Dr. Ada</td></tr></table>`))
		case "/kuliahUjian/6":
			_, _ = w.Write([]byte(`<table class="cell-xs-6"><tr><td>1</td><td>07.30 - 08.20</td></tr></table>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	previousConfig := config.AppConfig
	config.AppConfig = config.Config{BaseURL: upstream.URL}
	defer func() { config.AppConfig = previousConfig }()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/jadwal/1IA01", nil)
	response := httptest.NewRecorder()
	HandlerJadwal(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("HandlerJadwal status = %d, body = %s", response.Code, response.Body.String())
	}

	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Jadwal struct {
				Senin []struct {
					Nama string `json:"nama"`
					Jam  string `json:"jam"`
				} `json:"senin"`
			} `json:"jadwal"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.Success || len(envelope.Data.Jadwal.Senin) != 1 {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
	if got := envelope.Data.Jadwal.Senin[0].Jam; got != "07:30 - 08:20" {
		t.Fatalf("schedule time = %q, want %q", got, "07:30 - 08:20")
	}
}
