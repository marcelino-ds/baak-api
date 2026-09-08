package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// FlareSolverrRequest represents a request to FlareSolverr
type FlareSolverrRequest struct {
	Cmd        string        `json:"cmd"`
	URL        string        `json:"url"`
	MaxTimeout int           `json:"maxTimeout"`
	Cookies    []FlareCookie `json:"cookies,omitempty"`
}

// FlareSolverrResponse represents a response from FlareSolverr
type FlareSolverrResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution struct {
		URL       string            `json:"url"`
		Status    int               `json:"status"`
		Headers   map[string]string `json:"headers"`
		Response  string            `json:"response"`
		Cookies   []FlareCookie     `json:"cookies"`
		UserAgent string            `json:"userAgent"`
	} `json:"solution"`
}

// FlareCookie represents a cookie from FlareSolverr
type FlareCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain,omitempty"`
	Path     string  `json:"path,omitempty"`
	Expires  float64 `json:"expires,omitempty"`
	HttpOnly bool    `json:"httpOnly,omitempty"`
	Secure   bool    `json:"secure,omitempty"`
}

// FlareSolverr handles requests through FlareSolverr proxy
type FlareSolverr struct {
	url    string
	client *http.Client
}

// GetFlareSolverr returns the FlareSolverr instance if configured
func GetFlareSolverr() *FlareSolverr {
	url := strings.TrimRight(os.Getenv("FLARESOLVERR_URL"), "/")
	if url == "" {
		return nil
	}
	return &FlareSolverr{
		url:    url,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// IsConfigured returns true if FlareSolverr is configured
func (fs *FlareSolverr) IsConfigured() bool {
	return fs != nil && fs.url != ""
}

// Fetch fetches a URL through FlareSolverr
func (fs *FlareSolverr) Fetch(targetURL string) (string, error) {
	return fs.FetchContext(context.Background(), targetURL, nil)
}

// FetchContext fetches a URL through FlareSolverr while honoring cancellation.
func (fs *FlareSolverr) FetchContext(ctx context.Context, targetURL string, jar http.CookieJar) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if fs == nil || fs.url == "" {
		return "", fmt.Errorf("FlareSolverr not configured")
	}

	reqBody := FlareSolverrRequest{
		Cmd:        "request.get",
		URL:        targetURL,
		MaxTimeout: 60000, // 60 seconds
	}
	if jar != nil {
		if parsedURL, err := http.NewRequest(http.MethodGet, targetURL, nil); err == nil {
			for _, cookie := range jar.Cookies(parsedURL.URL) {
				reqBody.Cookies = append(reqBody.Cookies, FlareCookie{
					Name:  cookie.Name,
					Value: cookie.Value,
				})
			}
		}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fs.url+"/v1", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := fs.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTMLBodyBytes+1))
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	if int64(len(body)) > maxHTMLBodyBytes {
		return "", fmt.Errorf("FlareSolverr response exceeds size limit")
	}

	var fsResp FlareSolverrResponse
	if err := json.Unmarshal(body, &fsResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %v", err)
	}

	if fsResp.Status != "ok" {
		return "", fmt.Errorf("FlareSolverr error: %s", fsResp.Message)
	}
	if fsResp.Solution.Status != http.StatusOK {
		return "", fmt.Errorf("FlareSolverr unexpected status code: %d", fsResp.Solution.Status)
	}
	if jar != nil {
		cookieURL := targetURL
		if fsResp.Solution.URL != "" {
			cookieURL = fsResp.Solution.URL
		}
		if parsedURL, err := http.NewRequest(http.MethodGet, cookieURL, nil); err == nil {
			cookies := make([]*http.Cookie, 0, len(fsResp.Solution.Cookies))
			for _, cookie := range fsResp.Solution.Cookies {
				httpCookie := &http.Cookie{Name: cookie.Name, Value: cookie.Value, Path: cookie.Path}
				if cookie.Expires > 0 {
					httpCookie.Expires = time.Unix(int64(cookie.Expires), 0)
				}
				cookies = append(cookies, httpCookie)
			}
			if len(cookies) > 0 {
				jar.SetCookies(parsedURL.URL, cookies)
			}
		}
	}

	return fsResp.Solution.Response, nil
}

// CheckHealth checks if FlareSolverr is reachable and working
func (fs *FlareSolverr) CheckHealth() error {
	return fs.CheckHealthContext(context.Background())
}

// CheckHealthContext checks FlareSolverr while honoring request cancellation.
func (fs *FlareSolverr) CheckHealthContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if fs == nil || fs.url == "" {
		return fmt.Errorf("FlareSolverr not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fs.url+"/health", nil)
	if err != nil {
		return fmt.Errorf("failed to create FlareSolverr health request: %w", err)
	}
	resp, err := fs.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach FlareSolverr: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("FlareSolverr health check failed with status: %d", resp.StatusCode)
	}

	return nil
}

// IsCloudflareChallenge checks if the HTML response is a Cloudflare challenge
func IsCloudflareChallenge(html string) bool {
	cloudflareIndicators := []string{
		"Just a moment...",
		"cf-browser-verification",
		"cf-challenge",
		"Cloudflare Ray ID",
		"Enable JavaScript and cookies to continue",
		"_cf_chl_",
		"cRay",
		"cf_chl_prog",
	}

	for _, indicator := range cloudflareIndicators {
		if strings.Contains(html, indicator) {
			return true
		}
	}

	return false
}
