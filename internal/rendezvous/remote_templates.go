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

// RemoteTemplatesProvider proxies template endpoints to a standalone
// templates service and provides status/version info.
type RemoteTemplatesProvider struct {
	remoteBase
	proxy *httputil.ReverseProxy

	// extra cached status field
	templateCount int
}

// NewRemoteTemplatesProvider creates a provider that talks to the templates service.
func NewRemoteTemplatesProvider(baseURL, adminToken string) *RemoteTemplatesProvider {
	p := &RemoteTemplatesProvider{
		remoteBase: newRemoteBase(baseURL, adminToken),
	}
	target, _ := url.Parse(p.baseURL)
	p.proxy = httputil.NewSingleHostReverseProxy(target)
	p.fetchFn = func() {
		var result struct {
			Version       string `json:"version"`
			APIVersion    int    `json:"api_version"`
			DummyMode     bool   `json:"dummy_mode"`
			TemplateCount int    `json:"template_count"`
		}
		fetchCachedStatus(&p.cacheMu, &p.cachedAt,
			p.client, p.baseURL+"/api/templates/status", "templates", &result, func() {
				p.version = result.Version
				p.apiVersion = result.APIVersion
				p.dummyMode = result.DummyMode
				p.templateCount = result.TemplateCount
			})
	}
	return p
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
		setAuthHeader(req, p.adminToken)
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

// TemplateCount returns the cached template count from the templates service.
func (p *RemoteTemplatesProvider) TemplateCount() int {
	p.fetchStatus()
	p.cacheMu.RLock()
	defer p.cacheMu.RUnlock()
	return p.templateCount
}
