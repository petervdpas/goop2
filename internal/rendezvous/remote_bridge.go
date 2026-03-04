package rendezvous

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

// RemoteBridgeProvider proxies bridge endpoints to a standalone
// bridge service and provides status/version info.
type RemoteBridgeProvider struct {
	remoteBase

	// extra cached status field
	virtualPeers int
}

// NewRemoteBridgeProvider creates a provider that talks to the bridge service.
func NewRemoteBridgeProvider(baseURL, adminToken string) *RemoteBridgeProvider {
	p := &RemoteBridgeProvider{
		remoteBase: newRemoteBase(baseURL, adminToken),
	}
	p.fetchFn = func() {
		var result struct {
			Version      string `json:"version"`
			APIVersion   int    `json:"api_version"`
			DummyMode    bool   `json:"dummy_mode"`
			VirtualPeers int    `json:"virtual_peers"`
		}
		fetchCachedStatus(&p.cacheMu, &p.cachedAt,
			p.client, p.baseURL+"/api/bridge/status", "bridge", &result, func() {
				p.version = result.Version
				p.apiVersion = result.APIVersion
				p.dummyMode = result.DummyMode
				p.virtualPeers = result.VirtualPeers
			})
	}
	return p
}

// RegisterRoutes registers the bridge token request proxy endpoint.
func (p *RemoteBridgeProvider) RegisterRoutes(mux *http.ServeMux, registration *RemoteRegistrationProvider) {
	mux.HandleFunc("/api/bridge/request-token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Email             string `json:"email"`
			VerificationToken string `json:"verification_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Email == "" || req.VerificationToken == "" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"error": "email and verification token required"})
			return
		}

		// Verify the peer is actually registered and verified
		if registration != nil {
			if !registration.IsEmailVerified(req.Email) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"error": "email not verified"})
				return
			}
		}

		// Proxy to bridge service
		body, _ := json.Marshal(map[string]string{"email": req.Email})
		proxyReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
			p.baseURL+"/api/bridge/token", strings.NewReader(string(body)))
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
			return
		}
		proxyReq.Header.Set("Content-Type", "application/json")
		setAuthHeader(proxyReq, p.adminToken)

		resp, err := p.client.Do(proxyReq)
		if err != nil {
			log.Printf("bridge: token proxy error: %v", err)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"error": "bridge service unreachable"})
			return
		}
		defer resp.Body.Close()

		// Forward the bridge response as-is
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
}

// FetchVirtualPeers queries the bridge service for virtual peers and returns
// them in the same map format used by the topology response.
func (p *RemoteBridgeProvider) FetchVirtualPeers() []map[string]any {
	resp, err := p.client.Get(p.baseURL + "/api/bridge/peers")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var peers []struct {
		PeerID              string `json:"peer_id"`
		Label               string `json:"label"`
		Email               string `json:"email"`
		Platform            string `json:"platform"`
		PublicKey            string `json:"public_key"`
		EncryptionSupported bool   `json:"encryption_supported"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return nil
	}
	var out []map[string]any
	for _, peer := range peers {
		m := map[string]any{
			"id":         peer.PeerID,
			"label":      peer.Label,
			"reachable":  true,
			"connection": "bridge",
			"virtual":    true,
			"platform":   peer.Platform,
		}
		if peer.Email != "" {
			m["email"] = peer.Email
		}
		if peer.PublicKey != "" {
			m["publicKey"] = peer.PublicKey
		}
		if peer.EncryptionSupported {
			m["encryptionSupported"] = true
		}
		out = append(out, m)
	}
	return out
}

// VirtualPeerCount returns the cached virtual peer count from the bridge service.
func (p *RemoteBridgeProvider) VirtualPeerCount() int {
	p.fetchStatus()
	p.cacheMu.RLock()
	defer p.cacheMu.RUnlock()
	return p.virtualPeers
}
