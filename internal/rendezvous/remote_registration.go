package rendezvous

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/util"
)

// RemoteRegistrationProvider proxies registration endpoints to a standalone
// registration service and provides email verification checks.
type RemoteRegistrationProvider struct {
	baseURL    string
	adminToken string
	client     *http.Client

	// cached status from /api/reg/status
	regRequired    bool
	regVersion     string
	regAPIVersion  int
	regGrantAmount int
	regCachedAt    time.Time
	regCacheMu     sync.RWMutex
}

// NewRemoteRegistrationProvider creates a provider that talks to the registration service.
func NewRemoteRegistrationProvider(baseURL, adminToken string) *RemoteRegistrationProvider {
	return &RemoteRegistrationProvider{
		baseURL:    util.NormalizeURL(baseURL),
		adminToken: adminToken,
		client:     &http.Client{Timeout: 5 * time.Second},
	}
}

// RegisterRoutes sets up reverse proxies for registration endpoints.
// The verifyRender callback is called to render a nice HTML page for /verify
// instead of returning raw JSON.
func (p *RemoteRegistrationProvider) RegisterRoutes(mux *http.ServeMux) {
	target, err := url.Parse(p.baseURL)
	if err != nil {
		log.Printf("WARNING: invalid registration URL %q: %v", p.baseURL, err)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Proxy all /api/reg/* directly
	mux.HandleFunc("/api/reg/", func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})
}

// HandleVerify calls the registration service's verify endpoint and returns
// the parsed result so the caller can render a proper HTML page.
func (p *RemoteRegistrationProvider) HandleVerify(token string) (email string, ok bool) {
	reqURL := fmt.Sprintf("%s/api/reg/verify?token=%s", p.baseURL, url.QueryEscape(token))
	resp, err := p.client.Get(reqURL)
	if err != nil {
		log.Printf("registration: verify error: %v", err)
		return "", false
	}
	defer resp.Body.Close()

	var result struct {
		Status   string `json:"status"`
		Email    string `json:"email"`
		Verified bool   `json:"verified"`
	}
	if err := readJSON(resp, &result); err != nil {
		return "", false
	}

	return result.Email, result.Verified
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

	var result struct {
		Verified bool `json:"verified"`
	}
	if err := readJSON(resp, &result); err != nil {
		return false
	}
	return result.Verified
}

// fetchStatus fetches and caches /api/reg/status (registration_required, version).
func (p *RemoteRegistrationProvider) fetchStatus() {
	var result struct {
		RegistrationRequired bool   `json:"registration_required"`
		Version              string `json:"version"`
		APIVersion           int    `json:"api_version"`
		GrantAmount          int    `json:"grant_amount"`
	}
	fetchCachedStatus(&p.regCacheMu, &p.regCachedAt,
		p.client, p.baseURL+"/api/reg/status", "registration", &result, func() {
			p.regRequired = result.RegistrationRequired
			p.regVersion = result.Version
			p.regAPIVersion = result.APIVersion
			p.regGrantAmount = result.GrantAmount
		})
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

// FetchRegistrations fetches all registrations from the registration service.
func (p *RemoteRegistrationProvider) FetchRegistrations() (json.RawMessage, error) {
	req, err := http.NewRequest("GET", p.baseURL+"/api/reg/registrations", nil)
	if err != nil {
		return nil, err
	}
	setAuthHeader(req, p.adminToken)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registration service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registration service returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}
