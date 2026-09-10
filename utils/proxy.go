package utils

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"

	"github.com/yafyx/baak-api/config"
	"golang.org/x/net/publicsuffix"
)

// Default timeout for HTTP requests
var httpTimeout = 15 * time.Second

// ProxyManager handles proxy rotation for HTTP requests
type ProxyManager struct {
	proxies    []*url.URL
	mutex      sync.RWMutex
	transports map[string]*http.Transport
}

var (
	globalProxyManager *ProxyManager
	proxyOnce          sync.Once
)

// GetProxyManager returns the singleton proxy manager
func GetProxyManager() *ProxyManager {
	proxyOnce.Do(func() {
		globalProxyManager = &ProxyManager{
			proxies: make([]*url.URL, 0),
		}

		// Load proxies from environment variable if available
		for _, p := range config.AppConfig.HTTPProxies {
			if proxyURL, err := url.Parse(p); err == nil {
				globalProxyManager.AddProxy(proxyURL)
			}
		}
	})

	return globalProxyManager
}

// GetCookieJar returns a cookie jar for HTTP requests
func GetCookieJar() (http.CookieJar, error) {
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %v", err)
	}
	return jar, nil
}

// AddProxy adds a proxy to the manager
func (pm *ProxyManager) AddProxy(proxy *url.URL) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	pm.proxies = append(pm.proxies, proxy)
}

// GetTransport returns an http.Transport with the next proxy
func (pm *ProxyManager) GetTransport() *http.Transport {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// If no proxies, return default transport
	if len(pm.proxies) == 0 {
		return directTransport
	}

	// Use a random proxy
	proxyIndex := rand.Intn(len(pm.proxies))
	proxy := pm.proxies[proxyIndex]

	if pm.transports == nil {
		pm.transports = make(map[string]*http.Transport)
	}
	key := proxy.String()
	if transport := pm.transports[key]; transport != nil {
		return transport
	}
	transport := newTransport()
	transport.Proxy = http.ProxyURL(proxy)
	pm.transports[key] = transport
	return transport
}

var directTransport = newTransport()

// Connection pools are shared across requests; clients keep separate cookie jars.
func newTransport() *http.Transport {
	return &http.Transport{
		DialContext:  (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns: 100, MaxIdleConnsPerHost: 10, MaxConnsPerHost: 20,
		IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 10 * time.Second,
		ResponseHeaderTimeout: httpTimeout, ForceAttemptHTTP2: true,
	}
}

// GetClient returns an http.Client with a proxy-enabled transport
func (pm *ProxyManager) GetClient() (*http.Client, error) {
	// Create a cookie jar for the client
	jar, err := GetCookieJar()
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %v", err)
	}

	// Create a client with the proxy transport
	client := &http.Client{
		Transport: pm.GetTransport(),
		Timeout:   httpTimeout,
		Jar:       jar,
	}

	return client, nil
}

// HasProxies returns true if proxies are configured
func (pm *ProxyManager) HasProxies() bool {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	return len(pm.proxies) > 0
}
