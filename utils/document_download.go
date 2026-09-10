package utils

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

func DocumentID(source string) string {
	hash := sha256.Sum256([]byte(source))
	return hex.EncodeToString(hash[:])
}

func allowedDocument(base, target *url.URL) bool {
	if target.User != nil || target.Scheme != base.Scheme || target.Host != base.Host {
		return false
	}
	// Reject residual escapes so a second upstream decode cannot introduce traversal.
	if target.RawQuery != "" || target.ForceQuery || target.Fragment != "" || strings.ContainsAny(target.Path, "%\\\x00\r\n") {
		return false
	}
	if path.Clean(target.Path) != target.Path {
		return false
	}
	if strings.HasPrefix(target.Path, "/file/") {
		return strings.EqualFold(path.Ext(target.Path), ".pdf")
	}
	if strings.HasPrefix(target.Path, "/downloadAkademik/") {
		return publicIDPattern.MatchString(strings.TrimPrefix(target.Path, "/downloadAkademik/"))
	}
	return false
}

// DownloadPDF restricts both the initial URL and every redirect to public document paths on BAAK.
func DownloadPDF(ctx context.Context, baseURL, source string) ([]byte, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, ErrPDFInvalid
	}
	target, err := url.Parse(source)
	if err != nil || !allowedDocument(base, target) {
		return nil, ErrPDFInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client := &http.Client{Transport: directTransport, Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !allowedDocument(base, req.URL) {
			return ErrPDFInvalid
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, ErrPDFInvalid
	}
	req.Header.Set("User-Agent", userAgents[0])
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUpstream, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, upstreamStatusError{code: response.StatusCode}
	}
	if response.ContentLength > 16<<20 {
		return nil, ErrPDFInvalid
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (16<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUpstream, err)
	}
	if len(data) > 16<<20 || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, ErrPDFInvalid
	}
	return data, nil
}
