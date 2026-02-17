package rendezvous

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// RemoteEmailProvider proxies email endpoints to a standalone
// email service and provides status/version info.
type RemoteEmailProvider struct {
	remoteBase
}

// NewRemoteEmailProvider creates a provider that talks to the email service.
func NewRemoteEmailProvider(baseURL string) *RemoteEmailProvider {
	p := &RemoteEmailProvider{remoteBase: newRemoteBase(baseURL, "")}
	p.fetchFn = func() {
		var result struct {
			Version    string `json:"version"`
			APIVersion int    `json:"api_version"`
			DummyMode  bool   `json:"dummy_mode"`
		}
		fetchCachedStatus(&p.cacheMu, &p.cachedAt,
			p.client, p.baseURL+"/api/email/status", "email", &result, func() {
				p.version = result.Version
				p.apiVersion = result.APIVersion
				p.dummyMode = result.DummyMode
			})
	}
	return p
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
