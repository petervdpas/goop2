//
// Realtime channels — thin wrapper around the group protocol for low-latency
// 2-peer communication (signaling, game moves, video chat, etc.).
// Each channel is a private group with max_members=2.
package realtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/group"
)

// Channel represents an active realtime channel between two peers.
type Channel struct {
	ID         string `json:"id"`
	RemotePeer string `json:"remote_peer"`
	Role       string `json:"role"` // "host" or "guest"
	CreatedAt  int64  `json:"created_at"`
}

// Envelope is a realtime message that flows through a channel.
type Envelope struct {
	Channel string `json:"channel"`
	From    string `json:"from"`
	Payload any    `json:"payload"`
}

// Manager manages realtime channels on top of the group protocol.
type Manager struct {
	grp    *group.Manager
	selfID string

	mu       sync.RWMutex
	channels map[string]*Channel // channelID -> Channel

	// Listeners for realtime events (filtered from group events)
	listenerMu sync.RWMutex
	listeners  map[chan *Envelope]struct{}
}

// New creates a new realtime manager.
// Any rt- groups left over from a previous session are purged immediately.
func New(grp *group.Manager, selfID string) *Manager {
	m := &Manager{
		grp:       grp,
		selfID:    selfID,
		channels:  make(map[string]*Channel),
		listeners: make(map[chan *Envelope]struct{}),
	}

	// Purge stale rt- groups that survived a crash or unclean shutdown.
	if rows, err := grp.ListHostedGroups(); err == nil {
		for _, g := range rows {
			if strings.HasPrefix(g.ID, "rt-") {
				_ = grp.CloseGroup(g.ID)
				log.Printf("REALTIME: Cleaned up stale channel %s on startup", g.ID)
			}
		}
	}

	// Subscribe to group events and filter for realtime channels
	go m.forwardGroupEvents()

	return m
}

// CreateChannel creates a new realtime channel and invites the remote peer.
func (m *Manager) CreateChannel(ctx context.Context, remotePeerID string) (*Channel, error) {
	id := generateChannelID()

	// Create a private group with max 2 members
	if err := m.grp.CreateGroup(id, "rt:"+id, "realtime", 2, false); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}

	// Host joins own group
	if err := m.grp.JoinOwnGroup(id); err != nil {
		m.grp.CloseGroup(id)
		return nil, fmt.Errorf("join own group: %w", err)
	}

	// Invite the remote peer
	if err := m.grp.InvitePeer(ctx, remotePeerID, id); err != nil {
		m.grp.CloseGroup(id)
		return nil, fmt.Errorf("invite peer: %w", err)
	}

	ch := &Channel{
		ID:         id,
		RemotePeer: remotePeerID,
		Role:       "host",
		CreatedAt:  time.Now().UnixMilli(),
	}

	m.mu.Lock()
	m.channels[id] = ch
	m.mu.Unlock()

	log.Printf("REALTIME: Created channel %s with peer %s", id, remotePeerID)
	return ch, nil
}

// AcceptChannel accepts an incoming channel invitation.
// The browser calls this after receiving a group invite event.
func (m *Manager) AcceptChannel(ctx context.Context, channelID, hostPeerID string) (*Channel, error) {
	// Join the remote group
	if err := m.grp.JoinRemoteGroup(ctx, hostPeerID, channelID); err != nil {
		return nil, fmt.Errorf("join remote group: %w", err)
	}

	ch := &Channel{
		ID:         channelID,
		RemotePeer: hostPeerID,
		Role:       "guest",
		CreatedAt:  time.Now().UnixMilli(),
	}

	m.mu.Lock()
	m.channels[channelID] = ch
	m.mu.Unlock()

	log.Printf("REALTIME: Accepted channel %s from host %s", channelID, hostPeerID)
	return ch, nil
}

// Send sends a payload on a realtime channel.
func (m *Manager) Send(channelID string, payload any) error {
	m.mu.RLock()
	ch, ok := m.channels[channelID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("channel not found: %s", channelID)
	}

	if ch.Role == "host" {
		return m.grp.SendToGroupAsHost(channelID, payload)
	}
	return m.grp.SendToGroup(channelID, payload)
}

// CloseChannel closes a realtime channel. Idempotent — returns nil if
// the channel was already closed (both peers call close on hangup).
func (m *Manager) CloseChannel(channelID string) error {
	m.mu.Lock()
	ch, ok := m.channels[channelID]
	if ok {
		delete(m.channels, channelID)
	}
	m.mu.Unlock()

	if !ok {
		return nil // already closed
	}

	if ch.Role == "host" {
		return m.grp.CloseGroup(channelID)
	}
	return m.grp.LeaveGroup(channelID)
}

// ListChannels returns all active channels.
func (m *Manager) ListChannels() []*Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]*Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		out = append(out, ch)
	}
	return out
}

// GetChannel returns a specific channel by ID.
func (m *Manager) GetChannel(id string) (*Channel, bool) {
	m.mu.RLock()
	ch, ok := m.channels[id]
	m.mu.RUnlock()
	return ch, ok
}

// Subscribe returns a channel that receives realtime envelopes.
func (m *Manager) Subscribe() (ch chan *Envelope, cancel func()) {
	ch = make(chan *Envelope, 64)

	m.listenerMu.Lock()
	m.listeners[ch] = struct{}{}
	m.listenerMu.Unlock()

	cancel = func() {
		m.listenerMu.Lock()
		if _, ok := m.listeners[ch]; ok {
			delete(m.listeners, ch)
			close(ch)
		}
		m.listenerMu.Unlock()
	}
	return ch, cancel
}

// forwardGroupEvents listens to all group events and forwards
// those belonging to realtime channels to our listeners.
func (m *Manager) forwardGroupEvents() {
	evtCh := m.grp.Subscribe()

	for evt := range evtCh {
		// Only forward message events for known realtime channels
		m.mu.RLock()
		_, isRT := m.channels[evt.Group]
		m.mu.RUnlock()

		if !isRT {
			// Check if this is a welcome for a realtime group (auto-detect app_type)
			if evt.Type == "welcome" {
				if wp, ok := evt.Payload.(map[string]any); ok {
					if appType, _ := wp["app_type"].(string); appType == "realtime" {
						// Auto-register as guest channel
						m.mu.Lock()
						if _, exists := m.channels[evt.Group]; !exists {
							m.channels[evt.Group] = &Channel{
								ID:         evt.Group,
								RemotePeer: evt.From,
								Role:       "guest",
								CreatedAt:  time.Now().UnixMilli(),
							}
						}
						m.mu.Unlock()
						isRT = true
					}
				}
			}
		}

		if !isRT {
			continue
		}

		// Handle channel close events
		if evt.Type == "close" || evt.Type == "leave" {
			m.mu.Lock()
			delete(m.channels, evt.Group)
			m.mu.Unlock()
		}

		// Skip own messages — the host receives its own sends via
		// notifyListeners in SendToGroupAsHost, which would echo
		// signaling messages (SDP offers, ICE candidates) back to the
		// sender and corrupt the WebRTC peer connection.
		if evt.From == m.selfID {
			continue
		}

		env := &Envelope{
			Channel: evt.Group,
			From:    evt.From,
			Payload: evt.Payload,
		}

		m.listenerMu.RLock()
		for ch := range m.listeners {
			select {
			case ch <- env:
			default:
			}
		}
		m.listenerMu.RUnlock()
	}
}

// Close shuts down the realtime manager.
func (m *Manager) Close() {
	m.listenerMu.Lock()
	for ch := range m.listeners {
		close(ch)
	}
	m.listeners = nil
	m.listenerMu.Unlock()

	m.mu.Lock()
	m.channels = nil
	m.mu.Unlock()
}

func generateChannelID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "rt-" + hex.EncodeToString(b)
}
