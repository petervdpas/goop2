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

	// cached registration_required status
	regRequired      bool
	regRequiredAt    time.Time
	regRequiredMu    sync.RWMutex
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

	// Proxy /register → /api/reg/register
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/api/reg/register"
		proxy.ServeHTTP(w, r)
	})

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

// RegistrationRequired queries the registration service to check if
// registration is required. The result is cached for 30 seconds.
func (p *RemoteRegistrationProvider) RegistrationRequired() bool {
	const cacheTTL = 30 * time.Second

	p.regRequiredMu.RLock()
	if time.Since(p.regRequiredAt) < cacheTTL {
		val := p.regRequired
		p.regRequiredMu.RUnlock()
		return val
	}
	p.regRequiredMu.RUnlock()

	reqURL := p.baseURL + "/api/reg/status"
	resp, err := p.client.Get(reqURL)
	if err != nil {
		log.Printf("registration: status check error: %v", err)
		// Return last known value on error
		p.regRequiredMu.RLock()
		val := p.regRequired
		p.regRequiredMu.RUnlock()
		return val
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.regRequired
	}

	var result struct {
		RegistrationRequired bool `json:"registration_required"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return p.regRequired
	}

	p.regRequiredMu.Lock()
	p.regRequired = result.RegistrationRequired
	p.regRequiredAt = time.Now()
	p.regRequiredMu.Unlock()

	return result.RegistrationRequired
}

