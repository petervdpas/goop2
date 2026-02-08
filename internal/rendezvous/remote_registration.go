package rendezvous

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// RemoteRegistrationProvider proxies registration endpoints to a standalone
// registration service and provides email verification checks.
type RemoteRegistrationProvider struct {
	baseURL string
	client  *http.Client

	// cached status from /api/reg/status
	regRequired   bool
	regVersion    string
	regAPIVersion int
	regGrantAmount int
	regCachedAt   time.Time
	regCacheMu    sync.RWMutex
}

// NewRemoteRegistrationProvider creates a provider that talks to the registration service.
func NewRemoteRegistrationProvider(baseURL string) *RemoteRegistrationProvider {
	return &RemoteRegistrationProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// RegisterRoutes sets up reverse proxies for registration endpoints.
func (p *RemoteRegistrationProvider) RegisterRoutes(mux *http.ServeMux) {
	target, err := url.Parse(p.baseURL)
	if err != nil {
		log.Printf("WARNING: invalid registration URL %q: %v", p.baseURL, err)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Proxy /verify → /api/reg/verify
	mux.HandleFunc("/verify", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/api/reg/verify"
		proxy.ServeHTTP(w, r)
	})

	// Proxy all /api/reg/* directly
	mux.HandleFunc("/api/reg/", func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})
}

// IsEmailVerified queries the registration service to check if an email is verified.
func (p *RemoteRegistrationProvider) IsEmailVerified(email string) bool {
	reqURL := fmt.Sprintf("%s/api/reg/verified?email=%s", p.baseURL, url.QueryEscape(email))
	resp, err := p.client.Get(reqURL)
	if err != nil {
		log.Printf("registration: verify check error: %v", err)
		return false // fail closed for registration checks
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	var result struct {
		Verified bool `json:"verified"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return false
	}
	return result.Verified
}

// fetchStatus fetches and caches /api/reg/status (registration_required, version).
func (p *RemoteRegistrationProvider) fetchStatus() {
	const cacheTTL = 30 * time.Second

	p.regCacheMu.RLock()
	if time.Since(p.regCachedAt) < cacheTTL {
		p.regCacheMu.RUnlock()
		return
	}
	p.regCacheMu.RUnlock()

	resp, err := p.client.Get(p.baseURL + "/api/reg/status")
	if err != nil {
		log.Printf("registration: status check error: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		RegistrationRequired bool   `json:"registration_required"`
		Version              string `json:"version"`
		APIVersion           int    `json:"api_version"`
		GrantAmount          int    `json:"grant_amount"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return
	}

	p.regCacheMu.Lock()
	p.regRequired = result.RegistrationRequired
	p.regVersion = result.Version
	p.regAPIVersion = result.APIVersion
	p.regGrantAmount = result.GrantAmount
	p.regCachedAt = time.Now()
	p.regCacheMu.Unlock()
}

// RegistrationRequired queries the registration service to check if
// registration is required. The result is cached for 30 seconds.
func (p *RemoteRegistrationProvider) RegistrationRequired() bool {
	p.fetchStatus()
	p.regCacheMu.RLock()
	defer p.regCacheMu.RUnlock()
	return p.regRequired
}

// Version returns the cached version string from the registration service.
func (p *RemoteRegistrationProvider) Version() string {
	p.fetchStatus()
	p.regCacheMu.RLock()
	defer p.regCacheMu.RUnlock()
	return p.regVersion
}

// APIVersion returns the cached API version from the registration service.
func (p *RemoteRegistrationProvider) APIVersion() int {
	p.fetchStatus()
	p.regCacheMu.RLock()
	defer p.regCacheMu.RUnlock()
	return p.regAPIVersion
}

// GrantAmount returns the cached grant_amount from the registration service.
func (p *RemoteRegistrationProvider) GrantAmount() int {
	p.fetchStatus()
	p.regCacheMu.RLock()
	defer p.regCacheMu.RUnlock()
	return p.regGrantAmount
}

