package rendezvous

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/petervdpas/goop2/internal/proto"
)

// wsClient wraps a WebSocket connection for a specific peer.
type wsClient struct {
	conn   *websocket.Conn
	send   chan []byte
	done   chan struct{}
	peerID string
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// sendToPeer sends a message directly to a WebSocket-connected peer.
// Returns true if the peer has an active WebSocket connection (message queued).
func (s *Server) sendToPeer(peerID string, msg []byte) bool {
	s.wsClientsMu.RLock()
	wsc, ok := s.wsClients[peerID]
	s.wsClientsMu.RUnlock()
	if !ok {
		return false
	}
	select {
	case wsc.send <- msg:
		return true
	default:
		return false // send buffer full, fall back to broadcast
	}
}

// handleWS upgrades an HTTP connection to WebSocket for a specific peer.
// The peer sends heartbeat/presence messages; the server pushes presence
// updates and punch hints through the same connection.
func (s *Server) handleWS(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("probe") == "1" {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "probe-ok"))
		conn.Close()
		return
	}

	peerID := r.URL.Query().Get("peer_id")
	if peerID == "" {
		http.Error(w, "peer_id required", http.StatusBadRequest)
		return
	}

	// Only allow peers that have published presence.
	// New peers hit /publish first (HTTP POST), then connect WS.
	// If a peer races and WS arrives before publish, the WS reconnect
	// loop retries after 250ms — by which time publish has completed.
	s.mu.Lock()
	_, knownPeer := s.peers[peerID]
	s.mu.Unlock()
	if !knownPeer {
		http.Error(w, "unknown peer — publish first", http.StatusTooEarly)
		return
	}

	// Per-IP WebSocket connection limit.
	remoteIP := extractIP(r.RemoteAddr)
	s.wsClientsMu.RLock()
	ipCount := 0
	for _, wsc := range s.wsClients {
		if extractIP(wsc.conn.RemoteAddr().String()) == remoteIP {
			ipCount++
		}
	}
	s.wsClientsMu.RUnlock()
	if ipCount >= maxWSClientsPerIP {
		http.Error(w, "too many WebSocket connections from this IP", http.StatusTooManyRequests)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws: upgrade failed for %s: %v", peerID[:min(8, len(peerID))], err)
		return
	}

	wsc := &wsClient{
		conn:   conn,
		send:   make(chan []byte, 128),
		done:   make(chan struct{}),
		peerID: peerID,
	}

	// Register this WebSocket client
	s.wsClientsMu.Lock()
	if old, exists := s.wsClients[peerID]; exists {
		// Signal previous write pump to exit (stale/reconnect)
		close(old.done)
		old.conn.Close()
	}
	s.wsClients[peerID] = wsc
	s.wsClientsMu.Unlock()

	s.addLog(fmt.Sprintf("WS connected: %s", peerID))

	// Combined write pump + ping ticker: single goroutine owns all conn writes
	go func() {
		ticker := time.NewTicker(WSPingInterval)
		defer ticker.Stop()
		defer conn.Close()
		for {
			select {
			case msg := <-wsc.send:
				conn.SetWriteDeadline(time.Now().Add(WSWriteDeadline))
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(WSWriteDeadline))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-wsc.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Read pump: receives presence messages from the peer
	conn.SetReadDeadline(time.Now().Add(WSReadDeadline))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(WSReadDeadline))
		return nil
	})

	defer func() {
		// Unregister — only if this is still the current client for this peer.
		// If a newer connection replaced us, it already closed our done channel.
		s.wsClientsMu.Lock()
		isCurrent := false
		if cur, ok := s.wsClients[peerID]; ok && cur == wsc {
			delete(s.wsClients, peerID)
			isCurrent = true
		}
		s.wsClientsMu.Unlock()

		if isCurrent {
			close(wsc.done)
		}

		s.addLog(fmt.Sprintf("WS disconnected: %s", peerID))

		// Instant disconnect detection: if the peer is still in s.peers,
		// broadcast TypeOffline immediately — same benefit as entangle.
		s.mu.Lock()
		_, stillOnline := s.peers[peerID]
		if stillOnline {
			delete(s.peers, peerID)
			s.peersDirty = true
		}
		s.mu.Unlock()

		if stillOnline && s.peerDB != nil {
			go s.peerDB.remove(peerID)
		}

		if stillOnline {
			offMsg := proto.PresenceMsg{
				Type:   proto.TypeOffline,
				PeerID: peerID,
				TS:     proto.NowMillis(),
			}
			if b, err := json.Marshal(offMsg); err == nil {
				s.broadcast(b)
			}
			s.addLog(fmt.Sprintf("WS: peer %s marked offline (connection lost)", peerID))
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		// Reset read deadline on any message
		conn.SetReadDeadline(time.Now().Add(WSReadDeadline))

		var pm proto.PresenceMsg
		if err := json.Unmarshal(message, &pm); err != nil {
			continue
		}

		if pm.PeerID == "" {
			pm.PeerID = peerID
		}

		if err := validatePresence(pm); err != nil {
			continue
		}

		// Same logic as /publish handler
		isRegistered := true
		if s.registration != nil && s.registration.RegistrationRequired() {
			if pm.Email == "" || pm.VerificationToken == "" {
				isRegistered = false
			} else {
				isRegistered = s.registration.IsEmailTokenValid(pm.Email, pm.VerificationToken)
			}
		}

		peerToken := pm.VerificationToken
		pm.VerificationToken = ""
		if pm.TS == 0 {
			pm.TS = proto.NowMillis()
		}
		pm.Verified = isRegistered

		b, _ := json.Marshal(pm)
		msgSize := int64(len(b))

		addrsChanged := s.upsertPeer(pm, msgSize, isRegistered, peerToken)
		s.broadcast(b)

		if pm.Type == proto.TypeOnline || pm.Type == proto.TypeUpdate {
			s.emitPunchHints(pm, addrsChanged)
		}
	}
}

func (s *Server) broadcast(b []byte) {
	s.mu.Lock()

	msgSize := int64(len(b))

	// Attribute received bytes to all online peers
	for peerID, peer := range s.peers {
		peer.BytesReceived += msgSize
		s.peers[peerID] = peer
	}
	s.peersDirty = true

	// Copy client channels so we can send outside the lock
	clients := make([]chan []byte, 0, len(s.clients))
	for ch := range s.clients {
		clients = append(clients, ch)
	}
	s.mu.Unlock()

	for _, ch := range clients {
		select {
		case ch <- b:
		default:
			// slow client; drop message rather than blocking server
		}
	}

	// Also fan out to WebSocket clients
	s.wsClientsMu.RLock()
	wsClients := make([]*wsClient, 0, len(s.wsClients))
	for _, wsc := range s.wsClients {
		wsClients = append(wsClients, wsc)
	}
	s.wsClientsMu.RUnlock()

	for _, wsc := range wsClients {
		select {
		case wsc.send <- b:
		default:
		}
	}
}
