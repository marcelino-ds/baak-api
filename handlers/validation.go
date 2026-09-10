package handlers

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxSearchLength = 128

func pathSearch(path, prefix string) (string, error) {
	// net/http has already decoded URL.Path; decoding again changes literal '%'.
	value, err := querySearch(strings.TrimPrefix(path, prefix))
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(value, "/\\") {
		return "", fmt.Errorf("search term contains an invalid character")
	}
	return value, nil
}

func querySearch(value string) (string, error) {
	if !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("search term must contain valid text without control characters")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("missing search query parameter 'q'")
	}
	if utf8.RuneCountInString(value) > maxSearchLength {
		return "", fmt.Errorf("search query must not exceed %d characters", maxSearchLength)
	}
	return value, nil
}
