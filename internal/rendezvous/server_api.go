package rendezvous

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/petervdpas/goop2/internal/util"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

func (s *Server) handlePeersJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(s.snapshotPeers())
}

func (s *Server) handleLogsJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	s.logMu.Lock()
	logs := make([]string, len(s.logs))
	copy(logs, s.logs)
	s.logMu.Unlock()

	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(logs)
}

type relayPeerJSON struct {
	PeerID string `json:"peer_id"`
	Name   string `json:"name,omitempty"`
	Addr   string `json:"addr"`
	Dir    string `json:"dir"` // "inbound" or "outbound"
}

func dirString(d network.Direction) string {
	switch d {
	case network.DirInbound:
		return "inbound"
	case network.DirOutbound:
		return "outbound"
	default:
		return "unknown"
	}
}

type relayStatusJSON struct {
	Peers []relayPeerJSON `json:"peers"`
	Logs  []string        `json:"logs"`
}

func (s *Server) handleRelayStatusJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	// Build peer ID → name map from rendezvous peers
	s.mu.Lock()
	peerNames := make(map[string]string, len(s.peers))
	for _, p := range s.peers {
		if p.Content != "" {
			peerNames[p.PeerID] = p.Content
		}
	}
	s.mu.Unlock()

	var result relayStatusJSON

	if s.relayHost != nil {
		for _, pid := range s.relayHost.Network().Peers() {
			conns := s.relayHost.Network().ConnsToPeer(pid)
			for _, c := range conns {
				result.Peers = append(result.Peers, relayPeerJSON{
					PeerID: pid.String(),
					Name:   peerNames[pid.String()],
					Addr:   c.RemoteMultiaddr().String(),
					Dir:    dirString(c.Stat().Direction),
				})
			}
		}
	}

	s.relayLogMu.Lock()
	result.Logs = make([]string, len(s.relayLogs))
	copy(result.Logs, s.relayLogs)
	s.relayLogMu.Unlock()

	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(result)
}

// handleDiagPeer queries a peer's diagnostic info via the relay host connection.
// Admin-only. The relay host opens a /goop/diag/1.0.0 stream to the peer and
// returns the diagnostic snapshot.
func (s *Server) handleDiagPeer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	if s.relayHost == nil {
		http.Error(w, "relay not enabled", http.StatusServiceUnavailable)
		return
	}

	peerIDStr := r.URL.Query().Get("peer")
	if peerIDStr == "" {
		http.Error(w, "missing ?peer= parameter", http.StatusBadRequest)
		return
	}
	pid, err := peer.Decode(peerIDStr)
	if err != nil {
		http.Error(w, "invalid peer ID", http.StatusBadRequest)
		return
	}

	// Check if the relay host has a connection to this peer.
	conns := s.relayHost.Network().ConnsToPeer(pid)
	if len(conns) == 0 {
		http.Error(w, "peer not connected to relay", http.StatusNotFound)
		return
	}

	// Gather relay-side info about this peer.
	now := time.Now()
	var relayConns []map[string]any
	for _, c := range conns {
		age := now.Sub(c.Stat().Opened)
		relayConns = append(relayConns, map[string]any{
			"addr":    c.RemoteMultiaddr().String(),
			"dir":     dirString(c.Stat().Direction),
			"age":     age.Truncate(time.Second).String(),
			"streams": len(c.GetStreams()),
		})
	}

	// Resolve peer name from heartbeat data.
	s.mu.Lock()
	peerName := ""
	if p, ok := s.peers[peerIDStr]; ok {
		peerName = p.Content
	}
	s.mu.Unlock()

	// Open a diagnostic stream to the peer via the relay connection.
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stream, err := s.relayHost.NewStream(ctx, pid, protocol.ID("/goop/diag/1.0.0"))
	if err != nil {
		// Peer doesn't support the protocol or stream failed.
		// Return what we know from the relay side.
		result := map[string]any{
			"peer_id":      peerIDStr,
			"name":         peerName,
			"relay_view":   relayConns,
			"stream_error": err.Error(),
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	defer stream.Close()

	// Read the peer's diagnostic snapshot.
	var peerDiag map[string]any
	if err := json.NewDecoder(io.LimitReader(stream, 64*1024)).Decode(&peerDiag); err != nil {
		peerDiag = map[string]any{"decode_error": err.Error()}
	}

	// Merge relay-side view into the response.
	peerDiag["name"] = peerName
	peerDiag["relay_view"] = relayConns

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(peerDiag)
}

// handlePulse tells a target peer to refresh its relay reservation.
// Any peer can request this — when they can't reach a target peer, they
// call POST /api/pulse?peer=<id> and the rendezvous opens a relay-refresh
// stream to the target via the relay host.
func (s *Server) handlePulse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.relayHost == nil {
		http.Error(w, "relay not enabled", http.StatusServiceUnavailable)
		return
	}

	ip := extractIP(r.RemoteAddr)
	if !s.allowPublish(ip) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}

	peerIDStr := r.URL.Query().Get("peer")
	if peerIDStr == "" {
		http.Error(w, "missing ?peer= parameter", http.StatusBadRequest)
		return
	}
	pid, err := peer.Decode(peerIDStr)
	if err != nil {
		http.Error(w, "invalid peer ID", http.StatusBadRequest)
		return
	}

	// Check if the relay host has a connection to this peer.
	conns := s.relayHost.Network().ConnsToPeer(pid)
	if len(conns) == 0 {
		http.Error(w, "peer not connected to relay", http.StatusNotFound)
		return
	}

	s.relayAddLog(fmt.Sprintf("pulse: refreshing relay for %s (requested by %s)", pid.String()[:16]+"...", extractIP(r.RemoteAddr)))

	// Open a relay-refresh stream to the target peer.
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	stream, err := s.relayHost.NewStream(ctx, pid, protocol.ID("/goop/relay-refresh/1.0.0"))
	if err != nil {
		s.relayAddLog(fmt.Sprintf("pulse: stream to %s failed: %v", pid.String()[:16]+"...", err))
		http.Error(w, "pulse stream failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer stream.Close()

	// Read the result from the peer.
	var result map[string]any
	if err := json.NewDecoder(io.LimitReader(stream, 4096)).Decode(&result); err != nil {
		result = map[string]any{"ok": false, "error": err.Error()}
	}

	s.relayAddLog(fmt.Sprintf("pulse: %s responded: %v", pid.String()[:16]+"...", result))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(result)
}

// serviceLogEntry is a single log line from a microservice.
type serviceLogEntry struct {
	Service string `json:"service"`
	Message string `json:"message"`
}

func (s *Server) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	type svcDef struct {
		name string
		url  string
	}
	var svcs []svcDef
	if s.registration != nil {
		svcs = append(svcs, svcDef{"registration", s.registration.baseURL})
	}
	if cp, ok := s.credits.(*RemoteCreditProvider); ok {
		svcs = append(svcs, svcDef{"credits", cp.baseURL})
	}
	if s.email != nil {
		svcs = append(svcs, svcDef{"email", s.email.baseURL})
	}
	if s.templates != nil {
		svcs = append(svcs, svcDef{"templates", s.templates.baseURL})
	}

	type result struct {
		name    string
		entries []string
	}
	ch := make(chan result, len(svcs))
	client := &http.Client{Timeout: HealthCheckTimeout}

	for _, svc := range svcs {
		go func(name, baseURL string) {
			resp, err := client.Get(baseURL + "/api/logs")
			if err != nil {
				ch <- result{name, nil}
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				ch <- result{name, nil}
				return
			}
			var lines []string
			json.NewDecoder(resp.Body).Decode(&lines)
			ch <- result{name, lines}
		}(svc.name, svc.url)
	}

	var all []serviceLogEntry
	for range svcs {
		res := <-ch
		for _, line := range res.entries {
			all = append(all, serviceLogEntry{Service: res.name, Message: line})
		}
	}

	// Sort by message (which starts with timestamp from Go's log package)
	sort.Slice(all, func(i, j int) bool {
		return all[i].Message < all[j].Message
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(all)
}

// checkServiceHealth pings a service's /healthz endpoint.
func checkServiceHealth(baseURL string) bool {
	client := &http.Client{Timeout: PulseTimeout}
	resp, err := client.Get(util.NormalizeURL(baseURL) + "/healthz")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// topologyPath returns the topology endpoint path for a given service name.
func topologyPath(name string) string {
	switch strings.ToLower(name) {
	case "credits":
		return "/api/credits/topology"
	case "registration":
		return "/api/reg/topology"
	case "email":
		return "/api/email/topology"
	case "templates":
		return "/api/templates/topology"
	default:
		return "/api/" + strings.ToLower(name) + "/topology"
	}
}

// fetchTopology calls a service's topology endpoint and decodes the response.
func fetchTopology(baseURL, serviceName string) (topologyInfo, error) {
	client := &http.Client{Timeout: PulseTimeout}
	resp, err := client.Get(util.NormalizeURL(baseURL) + topologyPath(serviceName))
	if err != nil {
		return topologyInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return topologyInfo{}, fmt.Errorf("status %d", resp.StatusCode)
	}
	var topo topologyInfo
	if err := json.NewDecoder(resp.Body).Decode(&topo); err != nil {
		return topologyInfo{}, err
	}
	return topo, nil
}

// validateChain checks the topology data for common misconfiguration issues.
func validateChain(topologies []topologyInfo, services []serviceStatus) []string {
	var issues []string

	// Build a map of service name → URL (as configured in the rendezvous)
	rvURLs := make(map[string]string)
	for _, svc := range services {
		rvURLs[strings.ToLower(svc.Name)] = svc.URL
	}

	for _, topo := range topologies {
		for _, dep := range topo.Dependencies {
			if dep.URL == "" {
				// Check if the rendezvous has this service configured
				if rvURL, ok := rvURLs[dep.Name]; ok && rvURL != "" {
					issues = append(issues, fmt.Sprintf(
						"%s service has no %s URL configured — but the rendezvous connects to %s at %s",
						topo.Service, dep.Name, dep.Name, rvURL))
				} else {
					issues = append(issues, fmt.Sprintf(
						"%s service has no %s URL configured", topo.Service, dep.Name))
				}
			} else if !dep.OK {
				issues = append(issues, fmt.Sprintf(
					"%s → %s (%s): %s", topo.Service, dep.Name, dep.URL, dep.Error))
			}
		}
	}

	// Specific critical check: credits must know about templates for pricing to work
	for _, topo := range topologies {
		if topo.Service == "credits" {
			for _, dep := range topo.Dependencies {
				if dep.Name == "templates" && dep.URL == "" {
					issues = append(issues, "Credits service has no templates_url — all templates will appear free (price=0)")
				}
			}
		}
	}

	return issues
}
