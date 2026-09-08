package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/yafyx/baak-api/models"
	"golang.org/x/net/publicsuffix"
	"golang.org/x/time/rate"
)

const (
	BaseURL = "https://baak.gunadarma.ac.id"
	BaseIP  = "103.23.40.57"
)

var Limiter = rate.NewLimiter(rate.Limit(5), 10)

const maxHTMLBodyBytes int64 = 8 << 20

// Scraper owns the HTTP session for one API request. Keeping the client and
// cookie jar together makes CSRF and FlareSolverr fallbacks use one session.
type Scraper struct {
	BaseURL      string
	client       *http.Client
	flareSolverr *FlareSolverr
	breaker      *CircuitBreaker
	referrer     string
}

// NewScraper creates a request-scoped scraper with a secure TLS client.
func NewScraper(baseURL string) (*Scraper, error) {
	if baseURL == "" {
		baseURL = BaseURL
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid BAAK base URL %q", baseURL)
	}
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}
	client := &http.Client{
		Timeout: httpTimeout,
		Jar:     jar,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     20,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}
	if proxyManager := GetProxyManager(); proxyManager.HasProxies() {
		proxyClient, err := proxyManager.GetClient()
		if err != nil {
			return nil, fmt.Errorf("failed to create proxy client: %w", err)
		}
		client = proxyClient
	}
	return &Scraper{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		client:       client,
		flareSolverr: GetFlareSolverr(),
		breaker:      GetCircuitBreaker(),
		referrer:     strings.TrimRight(baseURL, "/"),
	}, nil
}

type upstreamStatusError struct {
	code int
}

func (e upstreamStatusError) Error() string {
	return fmt.Sprintf("unexpected status code: %d", e.code)
}

func (s *Scraper) ensureClient() error {
	if s.client == nil {
		return errors.New("scraper client is not configured")
	}
	if s.client.Jar == nil {
		jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
		if err != nil {
			return fmt.Errorf("failed to create cookie jar: %w", err)
		}
		s.client.Jar = jar
	}
	return nil
}

func (s *Scraper) fetchOnce(ctx context.Context, targetURL, referrer string) (*goquery.Document, error) {
	if err := s.ensureClient(); err != nil {
		return nil, err
	}
	parsedURL, err := url.Parse(targetURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid target URL %q", targetURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgents[0])
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "id-ID,id;q=0.9,en-US;q=0.8")
	req.Header.Set("Referer", referrer)
	response, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, upstreamStatusError{code: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxHTMLBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read HTML: %w", err)
	}
	if int64(len(body)) > maxHTMLBodyBytes {
		return nil, errors.New("HTML response exceeds size limit")
	}
	if IsCloudflareChallenge(string(body)) {
		return nil, errors.New("access forbidden (403): Cloudflare challenge")
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}
	return doc, nil
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Scraper) fetchWithRetry(ctx context.Context, targetURL string, maxRetries int) (*goquery.Document, error) {
	if maxRetries < 1 {
		return nil, errors.New("max retries must be positive")
	}
	referrer := s.referrer
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		doc, err := s.fetchOnce(ctx, targetURL, referrer)
		if err == nil {
			s.referrer = targetURL
			return doc, nil
		}
		lastErr = err
		if isCloudflareError(err) {
			break
		}
		var statusErr upstreamStatusError
		if errors.As(err, &statusErr) && (statusErr.code == http.StatusForbidden || statusErr.code == http.StatusTooManyRequests) {
			break
		}
		if attempt+1 < maxRetries {
			if err := waitForRetry(ctx, time.Duration(250*(1<<attempt))*time.Millisecond); err != nil {
				return nil, err
			}
		}
	}
	return nil, fmt.Errorf("all retry attempts failed: %w", lastErr)
}

func isCloudflareError(err error) bool {
	if err == nil {
		return false
	}
	var statusErr upstreamStatusError
	return errors.As(err, &statusErr) && statusErr.code == http.StatusForbidden || strings.Contains(err.Error(), "Cloudflare challenge")
}

// FetchDocument fetches HTML, retries transient failures, and falls back to FlareSolverr.
func (s *Scraper) FetchDocument(ctx context.Context, targetURL string) (*goquery.Document, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.breaker == nil {
		s.breaker = GetCircuitBreaker()
	}
	if err := s.ensureClient(); err != nil {
		return nil, err
	}
	if err := s.breaker.Allow(); err != nil {
		if s.flareSolverr == nil {
			return nil, fmt.Errorf("service temporarily unavailable: %w", err)
		}
		return s.fetchFlareSolverr(ctx, targetURL)
	}
	doc, err := s.fetchWithRetry(ctx, targetURL, 3)
	if err == nil {
		s.breaker.RecordSuccess()
		return doc, nil
	}
	if isCloudflareError(err) && s.flareSolverr != nil {
		doc, flareErr := s.fetchFlareSolverr(ctx, targetURL)
		if flareErr == nil {
			s.breaker.RecordSuccess()
			return doc, nil
		}
		err = flareErr
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	s.breaker.RecordFailure()
	return nil, err
}

func (s *Scraper) fetchFlareSolverr(ctx context.Context, targetURL string) (*goquery.Document, error) {
	if s.flareSolverr == nil {
		return nil, errors.New("FlareSolverr not configured")
	}
	html, err := s.flareSolverr.FetchContext(ctx, targetURL, s.client.Jar)
	if err != nil {
		return nil, fmt.Errorf("FlareSolverr fetch failed: %w", err)
	}
	if IsCloudflareChallenge(html) {
		return nil, errors.New("FlareSolverr returned a Cloudflare challenge")
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML from FlareSolverr: %w", err)
	}
	return doc, nil
}

// GetCSRFToken fetches a page using this scraper and extracts its CSRF token.
func (s *Scraper) GetCSRFToken(ctx context.Context, targetURL string) (string, error) {
	doc, err := s.FetchDocument(ctx, targetURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch document for CSRF token: %w", err)
	}
	token, exists := doc.Find(`input[name="_token"]`).First().Attr("value")
	if !exists {
		return "", fmt.Errorf("CSRF token input field not found on page: %s", targetURL)
	}
	return token, nil
}

func (s *Scraper) GetTimeStampLUT(ctx context.Context) ([][]string, error) {
	doc, err := s.FetchDocument(ctx, s.BaseURL+"/kuliahUjian/6")
	if err != nil {
		return nil, err
	}
	result := make([][]string, 0)
	doc.Find("table.cell-xs-6 tr").Each(func(_ int, row *goquery.Selection) {
		cells := row.Find("td")
		if cells.Length() < 2 {
			return
		}
		timeRange := strings.NewReplacer(" ", "", ".", ":").Replace(strings.TrimSpace(cells.Eq(1).Text()))
		times := strings.Split(timeRange, "-")
		if len(times) == 2 {
			result = append(result, times)
		}
	})
	return result, nil
}

func (s *Scraper) GetJadwal(ctx context.Context, targetURL string) (models.Jadwal, error) {
	doc, err := s.FetchDocument(ctx, targetURL)
	if err != nil {
		return models.Jadwal{}, err
	}
	lut, err := s.GetTimeStampLUT(ctx)
	if err != nil {
		return models.Jadwal{}, err
	}
	jadwal := models.Jadwal{}
	hariMap := map[string]*[]models.MataKuliah{
		"Senin": &jadwal.Senin, "Selasa": &jadwal.Selasa, "Rabu": &jadwal.Rabu,
		"Kamis": &jadwal.Kamis, "Jum'at": &jadwal.Jumat, "Sabtu": &jadwal.Sabtu,
	}
	doc.Find("table").First().Find("tr").Each(func(_ int, row *goquery.Selection) {
		cells := row.Find("td")
		if cells.Length() < 6 {
			return
		}
		course := models.MataKuliah{
			Nama: strings.TrimSpace(cells.Eq(2).Text()), Waktu: strings.TrimSpace(cells.Eq(3).Text()),
			Jam:   convertWaktuToJam(strings.TrimSpace(cells.Eq(3).Text()), lut),
			Ruang: strings.TrimSpace(cells.Eq(4).Text()), Dosen: strings.TrimSpace(cells.Eq(5).Text()),
		}
		if day, ok := hariMap[strings.TrimSpace(cells.Eq(1).Text())]; ok {
			*day = append(*day, course)
		}
	})
	return jadwal, nil
}

func (s *Scraper) GetKegiatan(ctx context.Context) ([]models.Kegiatan, error) {
	doc, err := s.FetchDocument(ctx, s.BaseURL)
	if err != nil {
		return nil, err
	}
	activities := make([]models.Kegiatan, 0)
	parent := ""
	doc.Find("table").First().Find("tr").Each(func(_ int, row *goquery.Selection) {
		cells := row.Find("td")
		if cells.Length() != 2 {
			parent = ""
			return
		}
		name, date := strings.TrimSpace(cells.Eq(0).Text()), strings.TrimSpace(cells.Eq(1).Text())
		if date == "" {
			parent = name
			return
		}
		start, end := parseTanggal(date)
		if parent != "" && isSubItem(name) {
			name = parent + " " + name
		} else {
			parent = ""
		}
		activities = append(activities, models.Kegiatan{Kegiatan: name, Tanggal: date, Start: start, End: end})
	})
	return activities, nil
}

func (s *Scraper) GetKelasbaru(ctx context.Context, targetURL string) ([]models.KelasBaru, error) {
	items := make([]models.KelasBaru, 0)
	for page := 1; ; page++ {
		doc, err := s.FetchDocument(ctx, fmt.Sprintf("%s&page=%d", targetURL, page))
		if err != nil {
			return nil, err
		}
		doc.Find("table").First().Find("tr").Each(func(_ int, row *goquery.Selection) {
			cells := row.Find("td")
			if cells.Length() == 5 {
				items = append(items, models.KelasBaru{NPM: strings.TrimSpace(cells.Eq(1).Text()), Nama: strings.TrimSpace(cells.Eq(2).Text()), KelasLama: strings.TrimSpace(cells.Eq(3).Text()), KelasBaru: strings.TrimSpace(cells.Eq(4).Text())})
			}
		})
		if doc.Find(`a[rel="next"]`).Length() == 0 {
			return items, nil
		}
	}
}

func (s *Scraper) GetMahasiswaBaru(ctx context.Context, targetURL string) ([]models.MahasiswaBaru, error) {
	items := make([]models.MahasiswaBaru, 0)
	for page := 1; ; page++ {
		doc, err := s.FetchDocument(ctx, fmt.Sprintf("%s&page=%d", targetURL, page))
		if err != nil {
			return nil, err
		}
		doc.Find("table").First().Find("tr").Each(func(_ int, row *goquery.Selection) {
			cells := row.Find("td")
			if cells.Length() == 6 {
				items = append(items, models.MahasiswaBaru{NoPend: strings.TrimSpace(cells.Eq(1).Text()), Nama: strings.TrimSpace(cells.Eq(2).Text()), NPM: strings.TrimSpace(cells.Eq(3).Text()), Kelas: strings.TrimSpace(cells.Eq(4).Text()), Keterangan: strings.TrimSpace(cells.Eq(5).Text())})
			}
		})
		if doc.Find(`a[rel="next"]`).Length() == 0 {
			return items, nil
		}
	}
}

func (s *Scraper) GetUTS(ctx context.Context, targetURL string) ([]models.UTS, error) {
	doc, err := s.FetchDocument(ctx, targetURL)
	if err != nil {
		return nil, err
	}
	items := make([]models.UTS, 0)
	doc.Find("table").First().Find("tr").Each(func(_ int, row *goquery.Selection) {
		cells := row.Find("td")
		if cells.Length() == 5 {
			items = append(items, models.UTS{Nama: strings.TrimSpace(cells.Eq(1).Text()), Waktu: strings.TrimSpace(cells.Eq(2).Text()), Ruang: strings.TrimSpace(cells.Eq(3).Text()), Dosen: strings.TrimSpace(cells.Eq(4).Text())})
		}
	})
	return items, nil
}

// List of common user agents to rotate through
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.5 Safari/605.1.15",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/115.0",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36",
	"Mozilla/5.0 (iPad; CPU OS 16_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.5 Mobile/15E148 Safari/604.1",
}

func convertWaktuToJam(waktu string, timeStampLUT [][]string) string {
	re := regexp.MustCompile(`(\d+)`)
	matches := re.FindAllString(waktu, -1)

	if len(matches) == 0 || len(timeStampLUT) == 0 {
		return ""
	}

	start, _ := strconv.Atoi(matches[0])
	end, _ := strconv.Atoi(matches[len(matches)-1])

	if start < 1 || start > len(timeStampLUT) || end < 1 || end > len(timeStampLUT) {
		return ""
	}
	if start > end {
		return ""
	}

	return timeStampLUT[start-1][0] + " - " + timeStampLUT[end-1][1]
}

func isSubItem(text string) bool {
	return regexp.MustCompile(`^[a-z]\..+`).MatchString(text)
}

func parseTanggal(tanggal string) (start, end string) {
	parts := strings.Split(tanggal, "-")
	if len(parts) == 2 {
		start = strings.TrimSpace(parts[0])
		end = strings.TrimSpace(parts[1])
	} else if len(parts) == 1 {
		start = strings.TrimSpace(parts[0])
		end = start
	}

	return start, end
}

// FetchDocumentWithRetry keeps the original utility API for callers that do
// not need a request context or FlareSolverr fallback.
func FetchDocumentWithRetry(targetURL, referrer string, maxRetries int) (*goquery.Document, error) {
	scraper, err := NewScraper(BaseURL)
	if err != nil {
		return nil, err
	}
	if referrer != "" {
		scraper.referrer = referrer
	}
	return scraper.fetchWithRetry(context.Background(), targetURL, maxRetries)
}

func FetchDocument(targetURL string) (*goquery.Document, error) {
	scraper, err := NewScraper(BaseURL)
	if err != nil {
		return nil, err
	}
	return scraper.FetchDocument(context.Background(), targetURL)
}

func GetCSRFToken(targetURL string) (string, error) {
	scraper, err := NewScraper(BaseURL)
	if err != nil {
		return "", err
	}
	return scraper.GetCSRFToken(context.Background(), targetURL)
}

func GetJadwal(targetURL string) (models.Jadwal, error) {
	scraper, err := NewScraper(BaseURL)
	if err != nil {
		return models.Jadwal{}, err
	}
	return scraper.GetJadwal(context.Background(), targetURL)
}

func GetTimeStampLUT() ([][]string, error) {
	scraper, err := NewScraper(BaseURL)
	if err != nil {
		return nil, err
	}
	return scraper.GetTimeStampLUT(context.Background())
}

func GetKegiatan(targetURL string) ([]models.Kegiatan, error) {
	scraper, err := NewScraper(BaseURL)
	if err != nil {
		return nil, err
	}
	scraper.BaseURL = strings.TrimRight(targetURL, "/")
	return scraper.GetKegiatan(context.Background())
}

func GetKelasbaru(targetURL string) ([]models.KelasBaru, error) {
	scraper, err := NewScraper(BaseURL)
	if err != nil {
		return nil, err
	}
	return scraper.GetKelasbaru(context.Background(), targetURL)
}

func GetMahasiswaBaru(targetURL string) ([]models.MahasiswaBaru, error) {
	scraper, err := NewScraper(BaseURL)
	if err != nil {
		return nil, err
	}
	return scraper.GetMahasiswaBaru(context.Background(), targetURL)
}

func GetUTS(targetURL string) ([]models.UTS, error) {
	scraper, err := NewScraper(BaseURL)
	if err != nil {
		return nil, err
	}
	return scraper.GetUTS(context.Background(), targetURL)
}

// EnsureSessionPublic is retained for the diagnostic command.
func EnsureSessionPublic() error {
	scraper, err := NewScraper(BaseURL)
	if err != nil {
		return err
	}
	_, err = scraper.FetchDocument(context.Background(), BaseURL)
	return err
}
