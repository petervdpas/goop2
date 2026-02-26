// Package entangle maintains a persistent bidirectional heartbeat stream
// between each pair of connected peers.
//
// One long-lived stream per peer keeps the underlying libp2p connection (and
// any relay circuit) warm, prevents the connection manager from pruning idle
// paths, and gives immediate disconnect detection: when the stream closes, the
// peer is gone — no polling, no timeout guessing.
//
// Protocol: /goop/entangle/1.0.0
//
//	→ {"type":"ping"} every 30 s
//	← {"type":"pong"}  reply
//
// Stream EOF / write error → peer marked unreachable via the PeerTable callback.
package entangle

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const (
	ProtoID      = "/goop/entangle/1.0.0"
	pingInterval = 30 * time.Second
	dialTimeout  = 15 * time.Second
)

type msg struct {
	Type string `json:"type"` // "ping" | "pong"
}

type entConn struct {
	peerID string
	stream network.Stream
	cancel context.CancelFunc
}

// Manager maintains one persistent entangle stream per connected peer.
type Manager struct {
	host   host.Host
	selfID string

	mu    sync.Mutex
	conns map[string]*entConn // peerID → active conn

	// onConnect is called when an entangle stream is successfully established.
	onConnect func(peerID string)
	// onDisconnect is called when a peer's stream dies.
	onDisconnect func(peerID string)
}

// New registers the /goop/entangle/1.0.0 stream handler and returns a Manager.
// onConnect is called when a stream is established; onDisconnect when it dies.
func New(h host.Host, onConnect, onDisconnect func(peerID string)) *Manager {
	m := &Manager{
		host:         h,
		selfID:       h.ID().String(),
		conns:        make(map[string]*entConn),
		onConnect:    onConnect,
		onDisconnect: onDisconnect,
	}
	h.SetStreamHandler(protocol.ID(ProtoID), m.handleIncoming)
	log.Printf("🔗 Entangle: registered handler for %s", ProtoID)
	return m
}

// Connect opens an entangle stream to peerID if one is not already open.
// Safe to call concurrently and repeatedly — duplicate calls are no-ops.
//
// Only the peer with the lexicographically lower ID initiates the stream.
// When both peers discover each other simultaneously and both call Connect,
// having both sides dial causes each handleIncoming to reset the other's
// outbound stream → runLoop dies → onDisconnect fires → infinite reset loop.
// With this rule exactly one side dials; the other just waits for the
// incoming stream (handleIncoming).
func (m *Manager) Connect(ctx context.Context, peerID string) {
	if peerID == m.selfID {
		return
	}
	if m.selfID > peerID {
		// The remote peer has the lower ID and will dial us.
		return
	}
	m.mu.Lock()
	if _, exists := m.conns[peerID]; exists {
		m.mu.Unlock()
		return
	}
	// Reserve slot immediately to prevent concurrent duplicate dials.
	m.conns[peerID] = nil
	m.mu.Unlock()

	go func() {
		pid, err := peer.Decode(peerID)
		if err != nil {
			m.mu.Lock()
			delete(m.conns, peerID)
			m.mu.Unlock()
			return
		}

		dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
		defer cancel()

		s, err := m.host.NewStream(dialCtx, pid, protocol.ID(ProtoID))
		if err != nil {
			m.mu.Lock()
			delete(m.conns, peerID)
			m.mu.Unlock()
			log.Printf("entangle: → %s dial failed: %v", peerID[:8], err)
			return
		}

		connCtx, connCancel := context.WithCancel(context.Background())
		c := &entConn{peerID: peerID, stream: s, cancel: connCancel}

		m.mu.Lock()
		m.conns[peerID] = c
		m.mu.Unlock()

		log.Printf("entangle: → %s entangled", peerID[:8])
		if m.onConnect != nil {
			go m.onConnect(peerID)
		}
		m.runLoop(connCtx, c)
	}()
}

// IsConnected reports whether an active entangle stream exists for peerID.
func (m *Manager) IsConnected(peerID string) bool {
	m.mu.Lock()
	c, ok := m.conns[peerID]
	m.mu.Unlock()
	return ok && c != nil
}

// handleIncoming is the stream handler for inbound entangle connections.
func (m *Manager) handleIncoming(s network.Stream) {
	peerID := s.Conn().RemotePeer().String()

	m.mu.Lock()
	if existing, exists := m.conns[peerID]; exists && existing != nil {
		// Already entangled — close the duplicate (lower peer ID wins, but
		// simpler to just keep the first one we got).
		m.mu.Unlock()
		s.Reset()
		return
	}
	connCtx, connCancel := context.WithCancel(context.Background())
	c := &entConn{peerID: peerID, stream: s, cancel: connCancel}
	m.conns[peerID] = c
	m.mu.Unlock()

	log.Printf("entangle: ← %s entangled", peerID[:8])
	if m.onConnect != nil {
		go m.onConnect(peerID)
	}
	go m.runLoop(connCtx, c)
}

// runLoop drives the ping/pong heartbeat on an established entangle stream.
// It exits when the stream closes or the context is cancelled.
func (m *Manager) runLoop(ctx context.Context, c *entConn) {
	defer func() {
		c.stream.Close()
		c.cancel()

		m.mu.Lock()
		if existing, ok := m.conns[c.peerID]; ok && existing == c {
			delete(m.conns, c.peerID)
		}
		m.mu.Unlock()

		log.Printf("entangle: %s disentangled", c.peerID[:8])
		if m.onDisconnect != nil {
			go m.onDisconnect(c.peerID)
		}
	}()

	enc := json.NewEncoder(c.stream)
	dec := json.NewDecoder(c.stream)

	readErr := make(chan error, 1)
	go func() {
		for {
			var in msg
			if err := dec.Decode(&in); err != nil {
				readErr <- err
				return
			}
			if in.Type == "ping" {
				_ = c.stream.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := enc.Encode(msg{Type: "pong"}); err != nil {
					readErr <- err
					return
				}
				_ = c.stream.SetWriteDeadline(time.Time{})
			}
		}
	}()

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-readErr:
			if err != nil {
				log.Printf("entangle: %s read error: %v", c.peerID[:8], err)
			}
			return
		case <-ticker.C:
			_ = c.stream.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := enc.Encode(msg{Type: "ping"}); err != nil {
				log.Printf("entangle: %s ping failed: %v", c.peerID[:8], err)
				return
			}
			_ = c.stream.SetWriteDeadline(time.Time{})
		}
	}
}

// Close shuts down all active entangle streams.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.conns {
		if c != nil {
			c.stream.Reset()
			c.cancel()
		}
	}
}
