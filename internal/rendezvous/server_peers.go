package rendezvous

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/petervdpas/goop2/internal/proto"
)

// pairKey returns a canonical sorted key for a peer pair.
func pairKey(a, b string) [2]string {
	if a > b {
		a, b = b, a
	}
	return [2]string{a, b}
}

// emitPunchHints sends targeted address hints to peer pairs.
// When a peer arrives or its addresses change, we tell each existing peer about
// the arriving peer's addresses and vice versa. Both sides then add addresses
// to their libp2p peerstores and probe — optimal for DCUtR hole-punching on WAN,
// and a fast supplement to mDNS on LAN.
// addrsChanged indicates whether the peer's addresses differ from what the server
// had stored before this update. On TypeOnline, always true.
func (s *Server) emitPunchHints(arriving proto.PresenceMsg, addrsChanged bool) {
	if !addrsChanged {
		return
	}
	if len(arriving.Addrs) == 0 {
		return
	}

	now := time.Now()

	s.mu.Lock()
	type hint struct {
		targetPeerID string
		msg          proto.PresenceMsg
	}
	var hints []hint

	for peerID, peer := range s.peers {
		if peerID == arriving.PeerID {
			continue
		}
		if len(peer.Addrs) == 0 {
			continue
		}

		key := pairKey(arriving.PeerID, peerID)
		if last, ok := s.punchCooldowns[key]; ok && now.Sub(last) < 60*time.Second {
			continue
		}
		s.punchCooldowns[key] = now

		// Tell existing peer about arriving peer
		hints = append(hints, hint{
			targetPeerID: peerID,
			msg: proto.PresenceMsg{
				Type:   proto.TypePunch,
				PeerID: arriving.PeerID,
				Target: peerID,
				Addrs:  arriving.Addrs,
				TS:     proto.NowMillis(),
			},
		})
		// Tell arriving peer about existing peer
		hints = append(hints, hint{
			targetPeerID: arriving.PeerID,
			msg: proto.PresenceMsg{
				Type:   proto.TypePunch,
				PeerID: peerID,
				Target: arriving.PeerID,
				Addrs:  peer.Addrs,
				TS:     proto.NowMillis(),
			},
		})
	}
	s.mu.Unlock()

	for _, h := range hints {
		b, err := json.Marshal(h.msg)
		if err != nil {
			continue
		}
		if !s.sendToPeer(h.targetPeerID, b) {
			s.broadcast(b)
		}
	}
}

// upsertPeer updates the in-memory peer map and persists to peerDB.
// Returns true if the peer is new or its addresses changed (used to gate punch hints).
func (s *Server) upsertPeer(pm proto.PresenceMsg, msgSize int64, verified bool, verificationToken string) bool {
	now := time.Now().UnixMilli()

	s.mu.Lock()
	defer s.mu.Unlock()

	// If peer sends offline, remove them immediately
	if pm.Type == proto.TypeOffline {
		delete(s.peers, pm.PeerID)
		s.peersDirty = true
		s.addLog(fmt.Sprintf("Peer went offline and removed: %s", pm.PeerID))
		if s.peerDB != nil {
			go s.peerDB.remove(pm.PeerID)
		}
		return false
	}

	// Preserve existing byte counts and detect address changes
	existing, exists := s.peers[pm.PeerID]
	addrsChanged := !exists || !addrsEqual(existing.Addrs, pm.Addrs)
	bytesSent := msgSize
	bytesReceived := int64(0)
	if exists {
		bytesSent += existing.BytesSent
		bytesReceived = existing.BytesReceived
	}

	row := peerRow{
		PeerID:              pm.PeerID,
		Type:                pm.Type,
		Content:             pm.Content,
		Email:               pm.Email,
		AvatarHash:          pm.AvatarHash,
		ActiveTemplate:      pm.ActiveTemplate,
		PublicKey:            pm.PublicKey,
		EncryptionSupported: pm.EncryptionSupported,
		Addrs:               pm.Addrs,
		TS:                  pm.TS,
		LastSeen:            now,
		BytesSent:           bytesSent,
		BytesReceived:       bytesReceived,
		Verified:            verified,
		verificationToken:   verificationToken,
	}
	s.peers[pm.PeerID] = row
	s.peersDirty = true

	if s.peerDB != nil {
		go s.peerDB.upsert(row)
	}
	return addrsChanged
}

// addrsEqual compares two address slices for equality (order-insensitive).
func addrsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, addr := range a {
		set[addr] = struct{}{}
	}
	for _, addr := range b {
		if _, ok := set[addr]; !ok {
			return false
		}
	}
	return true
}

func (s *Server) snapshotPeers() []peerRow {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.peersDirty && s.cachedPeers != nil {
		return s.cachedPeers
	}

	out := make([]peerRow, 0, len(s.peers))
	for _, v := range s.peers {
		out = append(out, v)
	}

	rank := func(t string) int {
		switch t {
		case proto.TypeOnline:
			return 0
		case proto.TypeUpdate:
			return 1
		case proto.TypeOffline:
			return 2
		default:
			return 9
		}
	}

	sort.Slice(out, func(i, j int) bool {
		ri, rj := rank(out[i].Type), rank(out[j].Type)
		if ri != rj {
			return ri < rj
		}
		return out[i].LastSeen > out[j].LastSeen
	})

	s.cachedPeers = out
	s.peersDirty = false
	return out
}

// Topology returns the network topology from the rendezvous server's perspective.
// The rendezvous is the center node; all registered peers are shown around it.
func (s *Server) Topology() map[string]any {
	peers := s.snapshotPeers()

	selfLabel := "Rendezvous"
	if s.externalURL != "" {
		selfLabel = s.externalURL
	}

	self := map[string]any{
		"id":          "rendezvous",
		"label":       selfLabel,
		"has_circuit": s.relayHost != nil,
	}

	var peerList []map[string]any
	for _, p := range peers {
		conn := "none"
		if p.Type == proto.TypeOnline || p.Type == proto.TypeUpdate {
			conn = "direct"
			// Check if any address contains /p2p-circuit
			for _, a := range p.Addrs {
				if strings.Contains(a, "/p2p-circuit") {
					conn = "relay"
					break
				}
			}
		}

		addr := ""
		if len(p.Addrs) > 0 {
			addr = p.Addrs[0]
		}

		peerList = append(peerList, map[string]any{
			"id":         p.PeerID,
			"label":      p.Content,
			"reachable":  p.Type == proto.TypeOnline || p.Type == proto.TypeUpdate,
			"connection": conn,
			"addr":       addr,
		})
	}

	// Include virtual peers from the bridge service
	if s.bridge != nil {
		if vpeers := s.bridge.FetchVirtualPeers(); len(vpeers) > 0 {
			peerList = append(peerList, vpeers...)
		}
	}

	result := map[string]any{
		"self":  self,
		"peers": peerList,
	}

	if s.relayInfo != nil {
		result["relay"] = map[string]any{
			"id":    s.relayInfo.PeerID,
			"label": "Relay",
		}
	}

	return result
}

// cleanupStalePeers removes peers that haven't been seen in 30+ seconds
func (s *Server) cleanupStalePeers(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now().UnixMilli()
			staleThreshold := now - (30 * 1000) // 30 seconds

			var pruned []string
			for peerID, peer := range s.peers {
				if peer.LastSeen < staleThreshold {
					delete(s.peers, peerID)
					pruned = append(pruned, peerID)
					s.addLog(fmt.Sprintf("Removed stale peer: %s (last seen: %v)", peerID, time.UnixMilli(peer.LastSeen).Format("15:04:05")))
				}
			}
			if len(pruned) > 0 {
				s.peersDirty = true
			}
			s.mu.Unlock()

			// Broadcast TypeOffline for each pruned peer so SSE subscribers
			// learn immediately instead of waiting for their own TTL expiry.
			for _, peerID := range pruned {
				offMsg := proto.PresenceMsg{
					Type:   proto.TypeOffline,
					PeerID: peerID,
					TS:     proto.NowMillis(),
				}
				if b, err := json.Marshal(offMsg); err == nil {
					s.broadcast(b)
				}
			}

			if s.peerDB != nil {
				go s.peerDB.cleanupStale(staleThreshold)
			}

			// Clean up stale punch cooldown entries (older than 5 minutes)
			punchCutoff := time.Now().Add(-5 * time.Minute)
			s.mu.Lock()
			for key, ts := range s.punchCooldowns {
				if ts.Before(punchCutoff) {
					delete(s.punchCooldowns, key)
				}
			}
			s.mu.Unlock()

			// Clean up stale rate limiter entries
			s.cleanupRateLimiter()
		}
	}
}

// loadPeersFromDB restores peer state from SQLite on startup.
func (s *Server) loadPeersFromDB() {
	rows, err := s.peerDB.loadAll()
	if err != nil {
		log.Printf("peerdb: load error: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range rows {
		s.peers[r.PeerID] = r
	}
	if len(rows) > 0 {
		s.peersDirty = true
		log.Printf("peerdb: loaded %d peers from database", len(rows))
	}
}

// syncFromDB periodically merges peer state from SQLite so that peers
// registered by other instances become visible.
func (s *Server) syncFromDB(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	var lastKnownMax int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Quick check: skip full load if DB hasn't changed
			dbMax, dbCount, err := s.peerDB.maxLastSeenAndCount()
			if err != nil {
				continue
			}
			s.mu.Lock()
			memCount := len(s.peers)
			s.mu.Unlock()
			if dbMax == lastKnownMax && dbCount == memCount {
				continue
			}
			lastKnownMax = dbMax

			rows, err := s.peerDB.loadAll()
			if err != nil {
				continue
			}

			s.mu.Lock()
			changed := false
			dbPeers := make(map[string]struct{}, len(rows))
			for _, r := range rows {
				dbPeers[r.PeerID] = struct{}{}
				existing, ok := s.peers[r.PeerID]
				if !ok || r.LastSeen > existing.LastSeen {
					s.peers[r.PeerID] = r
					changed = true
				}
			}
			// Remove peers that were cleaned up by another instance
			for peerID := range s.peers {
				if _, inDB := dbPeers[peerID]; !inDB {
					delete(s.peers, peerID)
					changed = true
				}
			}
			if changed {
				s.peersDirty = true
			}
			s.mu.Unlock()
		}
	}
}
