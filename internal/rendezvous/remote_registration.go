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
)

// RemoteRegistrationProvider proxies registration endpoints to a standalone
// registration service and provides email verification checks.
type RemoteRegistrationProvider struct {
	remoteBase

	// extra cached status fields
	regRequired    bool
	regGrantAmount int
}

// NewRemoteRegistrationProvider creates a provider that talks to the registration service.
func NewRemoteRegistrationProvider(baseURL, adminToken string) *RemoteRegistrationProvider {
	p := &RemoteRegistrationProvider{remoteBase: newRemoteBase(baseURL, adminToken)}
	p.fetchFn = func() {
		var result struct {
			RegistrationRequired bool   `json:"registration_required"`
			Version              string `json:"version"`
			APIVersion           int    `json:"api_version"`
			GrantAmount          int    `json:"grant_amount"`
		}
		fetchCachedStatus(&p.cacheMu, &p.cachedAt,
			p.client, p.baseURL+"/api/reg/status", "registration", &result, func() {
				p.regRequired = result.RegistrationRequired
				p.version = result.Version
				p.apiVersion = result.APIVersion
				p.regGrantAmount = result.GrantAmount
			})
	}
	return p
}

// RegisterRoutes sets up reverse proxies for registration endpoints.
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

// IsEmailTokenValid queries the registration service to check if an email+token pair is valid.
func (p *RemoteRegistrationProvider) IsEmailTokenValid(email, token string) bool {
	body, _ := json.Marshal(map[string]string{"email": email, "token": token})
	req, err := http.NewRequest("POST", p.baseURL+"/api/reg/validate", strings.NewReader(string(body)))
	if err != nil {
		log.Printf("registration: token validate error: %v", err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		log.Printf("registration: token validate error: %v", err)
		return false
	}
	defer resp.Body.Close()

	var result struct {
		Valid bool `json:"valid"`
	}
	if err := readJSON(resp, &result); err != nil {
		return false
	}
	return result.Valid
}

// RegistrationRequired queries the registration service to check if
// registration is required. The result is cached for 30 seconds.
func (p *RemoteRegistrationProvider) RegistrationRequired() bool {
	p.fetchStatus()
	p.cacheMu.RLock()
	defer p.cacheMu.RUnlock()
	return p.regRequired
}

// GrantAmount returns the cached grant_amount from the registration service.
func (p *RemoteRegistrationProvider) GrantAmount() int {
	p.fetchStatus()
	p.cacheMu.RLock()
	defer p.cacheMu.RUnlock()
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
