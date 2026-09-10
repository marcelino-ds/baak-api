package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPathSearchDecodesAndValidatesInput(t *testing.T) {
	request := httptest.NewRequest("GET", "/jadwal/kelas%20satu", nil)
	value, err := pathSearch(request.URL.Path, "/jadwal/")
	if err != nil || value != "kelas satu" {
		t.Fatalf("decoded search = %q, %v", value, err)
	}
	for _, path := range []string{"/jadwal/", "/jadwal/a/b", "/jadwal/invalid\x00"} {
		if _, err := pathSearch(path, "/jadwal/"); err == nil {
			t.Errorf("pathSearch accepted %q", path)
		}
	}
	request = httptest.NewRequest("GET", "/jadwal/100%2520", nil)
	if value, err := pathSearch(request.URL.Path, "/jadwal/"); err != nil || value != "100%20" {
		t.Fatalf("literal percent was decoded twice: %q, %v", value, err)
	}
}

func TestSearchValidationBoundsInput(t *testing.T) {
	if _, err := querySearch(strings.Repeat("a", maxSearchLength+1)); err == nil {
		t.Fatal("querySearch accepted an oversized query")
	}
	if value, err := querySearch("  1IA01  "); err != nil || value != "1IA01" {
		t.Fatalf("querySearch = %q, %v", value, err)
	}
}
