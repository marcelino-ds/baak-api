package handlers

import (
	"net/url"
	"testing"
)

func TestPublicQueryForwardsSupportedSearchAndPagination(t *testing.T) {
	input := url.Values{
		"page": {"2"}, "search_koor": {"algoritma"}, "ignored": {"secret"},
	}
	query, err := publicQuery("koordinator", input)
	if err != nil {
		t.Fatal(err)
	}
	if query.Get("page") != "2" || query.Get("search_koor") != "algoritma" || query.Get("ignored") != "" {
		t.Fatalf("forwarded query = %v", query)
	}
}

func TestPublicQueryRejectsUnsafePagination(t *testing.T) {
	for _, input := range []url.Values{
		{"page": {"0"}}, {"page": {"10001"}}, {"page": {"1\n2"}},
		{"search_pi": {"a\r\nb"}}, {"search_pi": {"a", "b"}},
	} {
		if _, err := publicQuery("pembimbing-pi", input); err == nil {
			t.Fatalf("unsafe query accepted: %v", input)
		}
	}
}

func TestPublicQueryReachesEntirePIList(t *testing.T) {
	query, err := publicQuery("pembimbing-pi", url.Values{"page": {"222"}, "q": {"3IA01"}})
	if err != nil || query.Get("page") != "222" || query.Get("search_pi") != "3IA01" {
		t.Fatalf("PI query = %v, %v", query, err)
	}
	if _, err := publicQuery("pembimbing-pi", url.Values{"q": {"A"}, "search_pi": {"B"}}); err == nil {
		t.Fatal("conflicting searches were accepted")
	}
}
