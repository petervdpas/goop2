package rendezvous

import (
	"io"
	"log"
	"net/http"
	"strings"
)

// RemoteEncryptionProvider proxies encryption endpoints to a standalone
// encryption service and provides status/version info.
type RemoteEncryptionProvider struct {
	remoteBase

	// extra cached status fields
	keyCount            int
	broadcastKeyAgeSec  int
	rotationIntervalMin int
}

// NewRemoteEncryptionProvider creates a provider that talks to the encryption service.
func NewRemoteEncryptionProvider(baseURL, adminToken string) *RemoteEncryptionProvider {
	p := &RemoteEncryptionProvider{
		remoteBase: newRemoteBase(baseURL, adminToken),
	}
	p.fetchFn = func() {
		var result struct {
			Version             string `json:"version"`
			APIVersion          int    `json:"api_version"`
			DummyMode           bool   `json:"dummy_mode"`
			KeyCount            int    `json:"key_count"`
			BroadcastKeyAgeSec  int    `json:"broadcast_key_age_sec"`
			RotationIntervalMin int    `json:"rotation_interval_min"`
		}
		fetchCachedStatus(&p.cacheMu, &p.cachedAt,
			p.client, p.baseURL+"/api/encryption/status", "encryption", &result, func() {
				p.version = result.Version
				p.apiVersion = result.APIVersion
				p.dummyMode = result.DummyMode
				p.keyCount = result.KeyCount
				p.broadcastKeyAgeSec = result.BroadcastKeyAgeSec
				p.rotationIntervalMin = result.RotationIntervalMin
			})
	}
	return p
}

// RegisterRoutes registers the encryption proxy endpoints.
func (p *RemoteEncryptionProvider) RegisterRoutes(mux *http.ServeMux) {
	// Proxy POST /api/encryption/keys — peer uploads public key
	mux.HandleFunc("/api/encryption/keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		proxyReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
			p.baseURL+"/api/encryption/keys", r.Body)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		proxyReq.Header.Set("Content-Type", "application/json")
		if peerID := r.Header.Get("X-Goop-PeerID"); peerID != "" {
			proxyReq.Header.Set("X-Goop-PeerID", peerID)
		}
		setAuthHeader(proxyReq, p.adminToken)

		resp, err := p.client.Do(proxyReq)
		if err != nil {
			log.Printf("encryption: keys proxy error: %v", err)
			http.Error(w, "encryption service unreachable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})

	// Proxy GET /api/encryption/broadcast-key — peer fetches sealed broadcast key
	mux.HandleFunc("/api/encryption/broadcast-key", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		targetURL := p.baseURL + "/api/encryption/broadcast-key"
		if q := r.URL.RawQuery; q != "" {
			targetURL += "?" + q
		}

		proxyReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if peerID := r.Header.Get("X-Goop-PeerID"); peerID != "" {
			proxyReq.Header.Set("X-Goop-PeerID", peerID)
		}

		resp, err := p.client.Do(proxyReq)
		if err != nil {
			log.Printf("encryption: broadcast-key proxy error: %v", err)
			http.Error(w, "encryption service unreachable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})

	// Proxy GET /api/encryption/keys/ — peer fetches another peer's public key
	mux.HandleFunc("/api/encryption/keys/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		peerID := strings.TrimPrefix(r.URL.Path, "/api/encryption/keys/")
		if peerID == "" {
			http.Error(w, "missing peer_id", http.StatusBadRequest)
			return
		}

		resp, err := p.client.Get(p.baseURL + "/api/encryption/keys/" + peerID)
		if err != nil {
			log.Printf("encryption: get key proxy error: %v", err)
			http.Error(w, "encryption service unreachable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
}

// KeyCount returns the cached peer key count from the encryption service.
func (p *RemoteEncryptionProvider) KeyCount() int {
	p.fetchStatus()
	p.cacheMu.RLock()
	defer p.cacheMu.RUnlock()
	return p.keyCount
}
