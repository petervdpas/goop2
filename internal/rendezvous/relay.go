package rendezvous

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
)

// RelayInfo describes a circuit relay v2 host that peers can use for NAT traversal.
type RelayInfo struct {
	PeerID string   `json:"peer_id"`
	Addrs  []string `json:"addrs"`
}

// StartRelay creates a libp2p host that acts as a circuit relay v2 server.
// externalURL, if set, is used to derive the public IP so WAN peers get a
// reachable address (e.g. /ip4/<public>/tcp/<port>/p2p/<id>).
func StartRelay(port int, keyFile string, externalURL string) (host.Host, *RelayInfo, error) {
	priv, err := loadOrCreateRelayKey(keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("relay key: %w", err)
	}

	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)),
		libp2p.EnableRelayService(),
		libp2p.DisableRelay(), // relay host itself doesn't need to be a relay client
	)
	if err != nil {
		return nil, nil, fmt.Errorf("relay host: %w", err)
	}

	info := &RelayInfo{
		PeerID: h.ID().String(),
	}

	// Collect listen addresses.
	for _, a := range h.Addrs() {
		info.Addrs = append(info.Addrs, a.String())
	}

	// If we have an external URL, resolve its hostname to an IP and prepend
	// a public multiaddr so WAN peers can reach this relay.
	if externalURL != "" {
		if pubAddr := buildPublicAddr(externalURL, port, h.ID().String()); pubAddr != "" {
			info.Addrs = append([]string{pubAddr}, info.Addrs...)
		}
	}

	log.Printf("relay: listening on port %d, peer ID %s (%d addrs)", port, info.PeerID, len(info.Addrs))
	return h, info, nil
}

// buildPublicAddr resolves the hostname from externalURL and returns a
// multiaddr string like /ip4/<ip>/tcp/<port>/p2p/<id>.
func buildPublicAddr(externalURL string, port int, peerID string) string {
	u, err := url.Parse(externalURL)
	if err != nil {
		return ""
	}
	hostname := u.Hostname()
	if hostname == "" {
		return ""
	}

	// Resolve to IP if it's a domain name.
	ip := net.ParseIP(hostname)
	if ip == nil {
		ips, err := net.LookupIP(hostname)
		if err != nil || len(ips) == 0 {
			log.Printf("relay: could not resolve %s: %v", hostname, err)
			return ""
		}
		// Prefer IPv4.
		for _, candidate := range ips {
			if candidate.To4() != nil {
				ip = candidate
				break
			}
		}
		if ip == nil {
			ip = ips[0]
		}
	}

	if ip.To4() != nil {
		return fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", ip.String(), port, peerID)
	}
	return fmt.Sprintf("/ip6/%s/tcp/%d/p2p/%s", ip.String(), port, peerID)
}

// loadOrCreateRelayKey loads an Ed25519 key from disk, or creates one.
func loadOrCreateRelayKey(keyFile string) (crypto.PrivKey, error) {
	data, err := os.ReadFile(keyFile)
	if err == nil {
		priv, err := crypto.UnmarshalPrivateKey(data)
		if err == nil {
			return priv, nil
		}
		log.Printf("WARNING: corrupt relay key at %s: %v (generating new key)", keyFile, err)
	}

	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		return nil, err
	}

	raw, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshal relay key: %w", err)
	}

	if dir := filepath.Dir(keyFile); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("create relay key directory: %w", err)
		}
	}

	if err := os.WriteFile(keyFile, raw, 0600); err != nil {
		return nil, fmt.Errorf("save relay key: %w", err)
	}

	log.Printf("relay: generated new identity key: %s", keyFile)
	return priv, nil
}

// handleRelayInfo writes the RelayInfo as JSON. Returns 404 if info is nil.
func handleRelayInfo(w http.ResponseWriter, r *http.Request, info *RelayInfo) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if info == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(info)
}
