package utils

import (
	"bytes"
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
	Cmd        string `json:"cmd"`
	URL        string `json:"url"`
	MaxTimeout int    `json:"maxTimeout"`
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
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Expires  int64  `json:"expires"`
	HttpOnly bool   `json:"httpOnly"`
	Secure   bool   `json:"secure"`
}

// FlareSolverr handles requests through FlareSolverr proxy
type FlareSolverr struct {
	url    string
	client *http.Client
}

var globalFlareSolverr *FlareSolverr

// GetFlareSolverr returns the FlareSolverr instance if configured
func GetFlareSolverr() *FlareSolverr {
	if globalFlareSolverr == nil {
		url := os.Getenv("FLARESOLVERR_URL")
		if url != "" {
			globalFlareSolverr = &FlareSolverr{
				url: url,
				client: &http.Client{
					Timeout: 120 * time.Second, // FlareSolverr can take a while
				},
			}
		}
	}
	return globalFlareSolverr
}

// IsConfigured returns true if FlareSolverr is configured
func (fs *FlareSolverr) IsConfigured() bool {
	return fs != nil && fs.url != ""
}

// Fetch fetches a URL through FlareSolverr
func (fs *FlareSolverr) Fetch(targetURL string) (string, error) {
	if fs == nil || fs.url == "" {
		return "", fmt.Errorf("FlareSolverr not configured")
	}

	reqBody := FlareSolverrRequest{
		Cmd:        "request.get",
		URL:        targetURL,
		MaxTimeout: 60000, // 60 seconds
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", fs.url+"/v1", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := fs.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	var fsResp FlareSolverrResponse
	if err := json.Unmarshal(body, &fsResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %v", err)
	}

	if fsResp.Status != "ok" {
		return "", fmt.Errorf("FlareSolverr error: %s", fsResp.Message)
	}

	return fsResp.Solution.Response, nil
}

// CheckHealth checks if FlareSolverr is reachable and working
func (fs *FlareSolverr) CheckHealth() error {
	if fs == nil || fs.url == "" {
		return fmt.Errorf("FlareSolverr not configured")
	}

	resp, err := fs.client.Get(fs.url + "/health")
	if err != nil {
		return fmt.Errorf("failed to reach FlareSolverr: %v", err)
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
