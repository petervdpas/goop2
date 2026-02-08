package rendezvous

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// RemoteEmailProvider proxies email endpoints to a standalone
// email service and provides status/version info.
type RemoteEmailProvider struct {
	baseURL string
	client  *http.Client

	// cached status from /api/email/status
	emailVersion    string
	emailAPIVersion int
	emailDummyMode  bool
	emailCachedAt   time.Time
	emailCacheMu    sync.RWMutex
}

// NewRemoteEmailProvider creates a provider that talks to the email service.
func NewRemoteEmailProvider(baseURL string) *RemoteEmailProvider {
	return &RemoteEmailProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// RegisterRoutes sets up a reverse proxy for /api/email/* to the email service.
func (p *RemoteEmailProvider) RegisterRoutes(mux *http.ServeMux) {
	target, err := url.Parse(p.baseURL)
	if err != nil {
		log.Printf("WARNING: invalid email URL %q: %v", p.baseURL, err)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	mux.HandleFunc("/api/email/", func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})
}

// fetchStatus fetches and caches /api/email/status.
func (p *RemoteEmailProvider) fetchStatus() {
	const cacheTTL = 30 * time.Second

	p.emailCacheMu.RLock()
	if time.Since(p.emailCachedAt) < cacheTTL {
		p.emailCacheMu.RUnlock()
		return
	}
	p.emailCacheMu.RUnlock()

	resp, err := p.client.Get(p.baseURL + "/api/email/status")
	if err != nil {
		log.Printf("email: status check error: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Version    string `json:"version"`
		APIVersion int    `json:"api_version"`
		DummyMode  bool   `json:"dummy_mode"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return
	}

	p.emailCacheMu.Lock()
	p.emailVersion = result.Version
	p.emailAPIVersion = result.APIVersion
	p.emailDummyMode = result.DummyMode
	p.emailCachedAt = time.Now()
	p.emailCacheMu.Unlock()
}

// Version returns the cached version string from the email service.
func (p *RemoteEmailProvider) Version() string {
	p.fetchStatus()
	p.emailCacheMu.RLock()
	defer p.emailCacheMu.RUnlock()
	return p.emailVersion
}

// APIVersion returns the cached API version from the email service.
func (p *RemoteEmailProvider) APIVersion() int {
	p.fetchStatus()
	p.emailCacheMu.RLock()
	defer p.emailCacheMu.RUnlock()
	return p.emailAPIVersion
}

// DummyMode returns the cached dummy_mode flag from the email service.
func (p *RemoteEmailProvider) DummyMode() bool {
	p.fetchStatus()
	p.emailCacheMu.RLock()
	defer p.emailCacheMu.RUnlock()
	return p.emailDummyMode
}
