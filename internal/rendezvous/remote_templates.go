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

// RemoteTemplatesProvider proxies template endpoints to a standalone
// templates service and provides status/version info.
type RemoteTemplatesProvider struct {
	baseURL    string
	adminToken string
	client     *http.Client
	proxy      *httputil.ReverseProxy

	// cached status from /api/templates/status
	tplVersion       string
	tplAPIVersion    int
	tplDummyMode     bool
	tplTemplateCount int
	tplCachedAt      time.Time
	tplCacheMu       sync.RWMutex
}

// NewRemoteTemplatesProvider creates a provider that talks to the templates service.
func NewRemoteTemplatesProvider(baseURL, adminToken string) *RemoteTemplatesProvider {
	base := strings.TrimRight(baseURL, "/")
	target, _ := url.Parse(base)
	return &RemoteTemplatesProvider{
		baseURL:    base,
		adminToken: adminToken,
		client:     &http.Client{Timeout: 5 * time.Second},
		proxy:      httputil.NewSingleHostReverseProxy(target),
	}
}

// RegisterRoutes registers /api/templates/prices handlers that inject the
// admin token for POST requests before proxying to the templates service.
func (p *RemoteTemplatesProvider) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/templates/prices", p.handlePrices)
}

func (p *RemoteTemplatesProvider) handlePrices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Read prices — no auth needed, proxy directly
		p.proxy.ServeHTTP(w, r)

	case http.MethodPost:
		// Write prices — inject admin token
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		req, err := http.NewRequest(http.MethodPost, p.baseURL+"/api/templates/prices", strings.NewReader(string(body)))
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if p.adminToken != "" {
			req.Header.Set("Authorization", "Bearer "+p.adminToken)
		}
		resp, err := p.client.Do(req)
		if err != nil {
			log.Printf("templates: price save error: %v", err)
			http.Error(w, "templates service error", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		forwardResponse(w, resp)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// Proxy returns the reverse proxy for forwarding requests to the templates service.
func (p *RemoteTemplatesProvider) Proxy() *httputil.ReverseProxy {
	return p.proxy
}

// FetchTemplates fetches the template list from the remote service.
func (p *RemoteTemplatesProvider) FetchTemplates() ([]StoreMeta, error) {
	resp, err := p.client.Get(p.baseURL + "/api/templates")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("templates service: status %s", resp.Status)
	}
	var out []StoreMeta
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// fetchStatus fetches and caches /api/templates/status.
func (p *RemoteTemplatesProvider) fetchStatus() {
	const cacheTTL = 30 * time.Second

	p.tplCacheMu.RLock()
	if time.Since(p.tplCachedAt) < cacheTTL {
		p.tplCacheMu.RUnlock()
		return
	}
	p.tplCacheMu.RUnlock()

	resp, err := p.client.Get(p.baseURL + "/api/templates/status")
	if err != nil {
		log.Printf("templates: status check error: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Version       string `json:"version"`
		APIVersion    int    `json:"api_version"`
		DummyMode     bool   `json:"dummy_mode"`
		TemplateCount int    `json:"template_count"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return
	}

	p.tplCacheMu.Lock()
	p.tplVersion = result.Version
	p.tplAPIVersion = result.APIVersion
	p.tplDummyMode = result.DummyMode
	p.tplTemplateCount = result.TemplateCount
	p.tplCachedAt = time.Now()
	p.tplCacheMu.Unlock()
}

// Version returns the cached version string from the templates service.
func (p *RemoteTemplatesProvider) Version() string {
	p.fetchStatus()
	p.tplCacheMu.RLock()
	defer p.tplCacheMu.RUnlock()
	return p.tplVersion
}

// APIVersion returns the cached API version from the templates service.
func (p *RemoteTemplatesProvider) APIVersion() int {
	p.fetchStatus()
	p.tplCacheMu.RLock()
	defer p.tplCacheMu.RUnlock()
	return p.tplAPIVersion
}

// DummyMode returns the cached dummy_mode flag from the templates service.
func (p *RemoteTemplatesProvider) DummyMode() bool {
	p.fetchStatus()
	p.tplCacheMu.RLock()
	defer p.tplCacheMu.RUnlock()
	return p.tplDummyMode
}

// TemplateCount returns the cached template count from the templates service.
func (p *RemoteTemplatesProvider) TemplateCount() int {
	p.fetchStatus()
	p.tplCacheMu.RLock()
	defer p.tplCacheMu.RUnlock()
	return p.tplTemplateCount
}
