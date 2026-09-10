package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/yafyx/baak-api/config"
)

// FlareSolverrRequest represents a request to FlareSolverr
type FlareSolverrRequest struct {
	Cmd        string        `json:"cmd"`
	URL        string        `json:"url"`
	MaxTimeout int           `json:"maxTimeout"`
	PostData   string        `json:"postData,omitempty"`
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

const flareSolverrTimeout = 65 * time.Second

var ErrFlareSolverrBusy = errors.New("FlareSolverr concurrency limit reached")

var flareConcurrency struct {
	sync.Mutex
	active int
}

func acquireFlareSolverrSlot(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := config.AppConfig.FlareSolverrMaxConcurrent
	if limit <= 0 {
		limit = config.DefaultFlareSolverrMaxConcurrent
	}
	flareConcurrency.Lock()
	defer flareConcurrency.Unlock()
	if flareConcurrency.active >= limit {
		return nil, ErrFlareSolverrBusy
	}
	flareConcurrency.active++
	return func() {
		flareConcurrency.Lock()
		flareConcurrency.active--
		flareConcurrency.Unlock()
	}, nil
}

// GetFlareSolverr returns the FlareSolverr instance if configured
func GetFlareSolverr() *FlareSolverr {
	url := config.AppConfig.FlareSolverrURL
	if url == "" {
		return nil
	}
	return &FlareSolverr{
		url:    url,
		client: flareClient,
	}
}

var flareClient = func() *http.Client {
	transport := newTransport()
	// The shared request limiter also covers HTTP/2; avoid a second transport queue.
	transport.MaxConnsPerHost = 0
	transport.MaxIdleConnsPerHost = 2
	transport.ResponseHeaderTimeout = flareSolverrTimeout
	return &http.Client{Timeout: flareSolverrTimeout, Transport: transport}
}()

var flareHealthClient = &http.Client{Timeout: config.HealthTimeout, Transport: directTransport}

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
	return fs.fetchContext(ctx, targetURL, jar, "request.get", "")
}

// FetchFormContext submits a form through FlareSolverr while honoring cancellation.
func (fs *FlareSolverr) FetchFormContext(ctx context.Context, targetURL string, jar http.CookieJar, values url.Values) (string, error) {
	return fs.fetchContext(ctx, targetURL, jar, "request.post", values.Encode())
}

func (fs *FlareSolverr) fetchContext(ctx context.Context, targetURL string, jar http.CookieJar, command, postData string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if fs == nil || fs.url == "" {
		return "", fmt.Errorf("FlareSolverr not configured")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	client := fs.client
	if client == nil {
		client = flareClient
	}

	reqBody := FlareSolverrRequest{
		Cmd:        command,
		URL:        targetURL,
		MaxTimeout: 60000, // 60 seconds
		PostData:   postData,
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline).Milliseconds()
		if remaining < 1 {
			return "", context.DeadlineExceeded
		}
		if remaining < int64(reqBody.MaxTimeout) {
			reqBody.MaxTimeout = int(remaining)
		}
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

	// A caller disconnect does not stop FlareSolverr's browser. Keep observing
	// the job with its own timeout so cancellation cannot free capacity early.
	solverCtx, cancelSolver := context.WithTimeout(context.WithoutCancel(ctx), flareSolverrTimeout)
	req, err := http.NewRequestWithContext(
		solverCtx,
		http.MethodPost,
		fs.url+"/v1",
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		cancelSolver()
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	release, err := acquireFlareSolverrSlot(ctx)
	if err != nil {
		cancelSolver()
		return "", err
	}
	if err := ctx.Err(); err != nil {
		release()
		cancelSolver()
		return "", err
	}

	type result struct {
		response FlareSolverrResponse
		err      error
	}
	completed := make(chan result, 1)
	go func() {
		outcome := result{err: fmt.Errorf("%w: solver request did not complete", ErrFlareSolverr)}
		defer func() {
			if recover() != nil {
				outcome.err = fmt.Errorf("%w: unexpected solver request failure", ErrFlareSolverr)
			}
			cancelSolver()
			release()
			completed <- outcome
		}()
		outcome.response, outcome.err = fetchFlareSolverrResponse(client, req)
	}()

	var fsResp FlareSolverrResponse
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case outcome := <-completed:
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if outcome.err != nil {
			return "", outcome.err
		}
		fsResp = outcome.response
	}
	if fsResp.Solution.URL != "" {
		requested, requestErr := url.Parse(targetURL)
		resolved, resolveErr := url.Parse(fsResp.Solution.URL)
		if requestErr != nil || resolveErr != nil {
			return "", fmt.Errorf("%w: invalid solver result URL", ErrFlareSolverr)
		}
		if resolved.User != nil || resolved.Scheme != requested.Scheme || resolved.Host != requested.Host {
			return "", fmt.Errorf("%w: solver result left the source origin", ErrFlareSolverr)
		}
	}
	// Only an active caller may update its session; the worker never touches the jar.
	if jar != nil {
		cookieURL := targetURL
		if fsResp.Solution.URL != "" {
			cookieURL = fsResp.Solution.URL
		}
		if parsedURL, err := http.NewRequest(http.MethodGet, cookieURL, nil); err == nil {
			cookies := make([]*http.Cookie, 0, len(fsResp.Solution.Cookies))
			for _, cookie := range fsResp.Solution.Cookies {
				httpCookie := &http.Cookie{Name: cookie.Name, Value: cookie.Value, Path: cookie.Path, Domain: cookie.Domain, Secure: cookie.Secure, HttpOnly: cookie.HttpOnly}
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

func fetchFlareSolverrResponse(client *http.Client, req *http.Request) (FlareSolverrResponse, error) {
	var result FlareSolverrResponse
	resp, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("%w: HTTP status %d", ErrFlareSolverr, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTMLBodyBytes+1))
	if err != nil {
		return result, fmt.Errorf("failed to read response: %w", err)
	}
	if int64(len(body)) > maxHTMLBodyBytes {
		return result, fmt.Errorf("%w: response exceeds size limit", ErrFlareSolverr)
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if result.Status != "ok" {
		return result, fmt.Errorf("%w: solver rejected the request", ErrFlareSolverr)
	}
	if result.Solution.Status != http.StatusOK {
		return result, upstreamStatusError{code: result.Solution.Status}
	}
	return result, nil
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
	ctx, cancel := context.WithTimeout(ctx, config.HealthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fs.url+"/health", nil)
	if err != nil {
		return fmt.Errorf("failed to create FlareSolverr health request: %w", err)
	}
	resp, err := flareHealthClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach FlareSolverr: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))

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
