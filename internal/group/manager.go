package group

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/proto"
	"github.com/petervdpas/goop2/internal/storage"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// Event is emitted to local SSE listeners.
type Event struct {
	Type    string      `json:"type"`
	Group   string      `json:"group"`
	From    string      `json:"from,omitempty"`
	Payload any `json:"payload,omitempty"`
}

// Manager handles the group protocol, both host-side (relay) and client-side (connection).
type Manager struct {
	host   host.Host
	db     *storage.DB
	mu     sync.RWMutex
	selfID string

	// Host-side: groupID -> *hostedGroup
	groups map[string]*hostedGroup

	// Client-side: current outbound connection (nil if not connected)
	activeConn *clientConn

	// Local SSE listeners
	listeners []chan *Event
}

type hostedGroup struct {
	info         storage.GroupRow
	members      map[string]*memberConn // peerID -> connection
	hostJoined   bool
	hostJoinedAt int64
	mu           sync.RWMutex
}

type memberConn struct {
	peerID   string
	joinedAt int64
	stream   network.Stream
	encoder  *json.Encoder
	cancel   context.CancelFunc
	sendCh   chan Message // buffered outbound queue for non-blocking broadcast
}

type clientConn struct {
	hostPeerID string
	groupID    string
	stream     network.Stream
	encoder    *json.Encoder
	cancel     context.CancelFunc
}

// New creates a new group manager and registers the stream handler.
func New(h host.Host, db *storage.DB) *Manager {
	m := &Manager{
		host:      h,
		db:        db,
		selfID:    h.ID().String(),
		groups:    make(map[string]*hostedGroup),
		listeners: make([]chan *Event, 0),
	}

	// Load existing groups from DB into memory (restore host-joined state)
	if groups, err := db.ListGroups(); err == nil {
		for _, g := range groups {
			m.groups[g.ID] = &hostedGroup{
				info:       g,
				members:    make(map[string]*memberConn),
				hostJoined: g.HostJoined,
			}
		}
	}

	h.SetStreamHandler(protocol.ID(proto.GroupProtoID), m.handleIncomingStream)
	h.SetStreamHandler(protocol.ID(proto.GroupInviteProtoID), m.handleInviteStream)

	// Auto-reconnect to subscribed groups in the background
	go m.reconnectSubscriptions()

	return m
}

// ─── Host-side: stream handler ───────────────────────────────────────────────

func (m *Manager) handleIncomingStream(s network.Stream) {
	remotePeer := s.Conn().RemotePeer().String()
	dec := json.NewDecoder(s)
	enc := json.NewEncoder(s)

	// First message must be a join
	var joinMsg Message
	if err := dec.Decode(&joinMsg); err != nil {
		log.Printf("GROUP: Failed to decode join from %s: %v", remotePeer, err)
		s.Reset()
		return
	}
	if joinMsg.Type != TypeJoin {
		log.Printf("GROUP: Expected join from %s, got %s", remotePeer, joinMsg.Type)
		enc.Encode(Message{Type: TypeError, Payload: ErrorPayload{Code: "bad_first_msg", Message: "first message must be join"}})
		s.Reset()
		return
	}

	groupID := joinMsg.Group
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()

	if !exists {
		enc.Encode(Message{Type: TypeError, Group: groupID, Payload: ErrorPayload{Code: "not_found", Message: "group not found"}})
		s.Reset()
		return
	}

	// Check max_members
	hg.mu.Lock()
	if hg.info.MaxMembers > 0 && len(hg.members) >= hg.info.MaxMembers {
		hg.mu.Unlock()
		enc.Encode(Message{Type: TypeError, Group: groupID, Payload: ErrorPayload{Code: "full", Message: "group is full"}})
		s.Reset()
		return
	}

	// Create member connection with buffered send channel
	ctx, cancel := context.WithCancel(context.Background())
	mc := &memberConn{
		peerID:   remotePeer,
		joinedAt: nowMillis(),
		stream:   s,
		encoder:  enc,
		cancel:   cancel,
		sendCh:   make(chan Message, 64),
	}
	hg.members[remotePeer] = mc
	memberList := hg.memberList(m.selfID)
	hg.mu.Unlock()

	// Start per-member drain goroutine: writes from sendCh to the stream
	// with a deadline so one slow peer cannot block the others.
	go mc.drainLoop(ctx)

	log.Printf("GROUP: %s joined group %s", remotePeer, groupID)

	// Send welcome to the new member
	enc.Encode(Message{
		Type:  TypeWelcome,
		Group: groupID,
		Payload: WelcomePayload{
			GroupName: hg.info.Name,
			AppType:   hg.info.AppType,
			Members:   memberList,
		},
	})

	// Broadcast updated member list to all other members
	hg.broadcast(Message{
		Type:    TypeMembers,
		Group:   groupID,
		Payload: MembersPayload{Members: memberList},
	}, remotePeer)

	// Notify local listeners
	m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, Payload: MembersPayload{Members: memberList}})

	// Read loop: relay messages from this member to others
	m.readLoop(ctx, dec, hg, mc, groupID)

	// Cleanup on disconnect
	cancel()
	hg.mu.Lock()
	delete(hg.members, remotePeer)
	updatedMembers := hg.memberList(m.selfID)
	hg.mu.Unlock()

	s.Close()

	log.Printf("GROUP: %s left group %s", remotePeer, groupID)

	// Broadcast updated member list
	hg.broadcast(Message{
		Type:    TypeMembers,
		Group:   groupID,
		Payload: MembersPayload{Members: updatedMembers},
	}, "")

	m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, From: remotePeer, Payload: MembersPayload{Members: updatedMembers}})
}

func (m *Manager) readLoop(ctx context.Context, dec *json.Decoder, hg *hostedGroup, mc *memberConn, groupID string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var msg Message
		if err := dec.Decode(&msg); err != nil {
			return // disconnect
		}

		// Server-side: enforce sender identity
		msg.From = mc.peerID
		msg.Group = groupID

		switch msg.Type {
		case TypeLeave:
			return
		case TypeMsg, TypeState:
			// Relay to all other members
			hg.broadcast(msg, mc.peerID)
			// Also notify local listeners (host sees messages too)
			m.notifyListeners(&Event{Type: msg.Type, Group: groupID, From: mc.peerID, Payload: msg.Payload})
		}
	}
}

// ─── Host-side: group management ─────────────────────────────────────────────

// CreateGroup creates a new hosted group.
func (m *Manager) CreateGroup(id, name, appType string, maxMembers int) error {
	if err := m.db.CreateGroup(id, name, appType, maxMembers); err != nil {
		return err
	}

	g, err := m.db.GetGroup(id)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.groups[id] = &hostedGroup{
		info:    g,
		members: make(map[string]*memberConn),
	}
	m.mu.Unlock()

	log.Printf("GROUP: Created group %s (%s)", id, name)
	return nil
}

// CloseGroup closes a hosted group, notifying all members.
func (m *Manager) CloseGroup(groupID string) error {
	m.mu.Lock()
	hg, exists := m.groups[groupID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("group not found: %s", groupID)
	}
	delete(m.groups, groupID)
	m.mu.Unlock()

	// Send close to all members
	closeMsg := Message{Type: TypeClose, Group: groupID}
	hg.mu.Lock()
	for _, mc := range hg.members {
		mc.encoder.Encode(closeMsg)
		mc.cancel()
		mc.stream.Close()
	}
	hg.members = nil
	hg.mu.Unlock()

	// Remove from DB
	if err := m.db.DeleteGroup(groupID); err != nil {
		log.Printf("GROUP: Failed to delete group %s from DB: %v", groupID, err)
	}

	m.notifyListeners(&Event{Type: TypeClose, Group: groupID})

	log.Printf("GROUP: Closed group %s", groupID)
	return nil
}

// ListHostedGroups returns all hosted groups.
func (m *Manager) ListHostedGroups() ([]storage.GroupRow, error) {
	return m.db.ListGroups()
}

// HostedGroupMembers returns the current members of a hosted group.
func (m *Manager) HostedGroupMembers(groupID string) []MemberInfo {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()

	if !exists {
		return nil
	}

	hg.mu.RLock()
	defer hg.mu.RUnlock()
	return hg.memberList(m.selfID)
}

// JoinOwnGroup adds the host as a member of their own hosted group.
func (m *Manager) JoinOwnGroup(groupID string) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	hg.mu.Lock()
	if hg.hostJoined {
		hg.mu.Unlock()
		return fmt.Errorf("host already in group")
	}
	hg.hostJoined = true
	hg.hostJoinedAt = nowMillis()
	memberList := hg.memberList(m.selfID)
	hg.mu.Unlock()

	_ = m.db.SetHostJoined(groupID, true)

	// Broadcast updated member list to all connected peers
	hg.broadcast(Message{
		Type:    TypeMembers,
		Group:   groupID,
		Payload: MembersPayload{Members: memberList},
	}, "")

	m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, Payload: MembersPayload{Members: memberList}})

	log.Printf("GROUP: Host joined own group %s", groupID)
	return nil
}

// LeaveOwnGroup removes the host from their own hosted group.
func (m *Manager) LeaveOwnGroup(groupID string) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	hg.mu.Lock()
	if !hg.hostJoined {
		hg.mu.Unlock()
		return fmt.Errorf("host not in group")
	}
	hg.hostJoined = false
	hg.hostJoinedAt = 0
	memberList := hg.memberList(m.selfID)
	hg.mu.Unlock()

	_ = m.db.SetHostJoined(groupID, false)

	// Broadcast updated member list
	hg.broadcast(Message{
		Type:    TypeMembers,
		Group:   groupID,
		Payload: MembersPayload{Members: memberList},
	}, "")

	m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, Payload: MembersPayload{Members: memberList}})

	log.Printf("GROUP: Host left own group %s", groupID)
	return nil
}

// HostInGroup returns whether the host has joined the given hosted group.
func (m *Manager) HostInGroup(groupID string) bool {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()
	if !exists {
		return false
	}
	hg.mu.RLock()
	defer hg.mu.RUnlock()
	return hg.hostJoined
}

// ─── Client-side: connecting to remote groups ────────────────────────────────

// JoinRemoteGroup opens a stream to a remote host and joins a group.
func (m *Manager) JoinRemoteGroup(ctx context.Context, hostPeerID, groupID string) error {
	// Auto-leave any previous connection
	m.mu.Lock()
	old := m.activeConn
	m.activeConn = nil
	m.mu.Unlock()
	if old != nil {
		old.encoder.Encode(Message{Type: TypeLeave, Group: old.groupID})
		old.cancel()
		old.stream.Close()
	}

	pid, err := peer.Decode(hostPeerID)
	if err != nil {
		return fmt.Errorf("invalid host peer ID: %w", err)
	}

	stream, err := m.host.NewStream(ctx, pid, protocol.ID(proto.GroupProtoID))
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}

	enc := json.NewEncoder(stream)
	dec := json.NewDecoder(stream)

	// Send join message
	if err := enc.Encode(Message{Type: TypeJoin, Group: groupID}); err != nil {
		stream.Close()
		return fmt.Errorf("failed to send join: %w", err)
	}

	// Read welcome
	var welcome Message
	if err := dec.Decode(&welcome); err != nil {
		stream.Close()
		return fmt.Errorf("failed to read welcome: %w", err)
	}

	if welcome.Type == TypeError {
		stream.Close()
		return fmt.Errorf("join rejected: %v", welcome.Payload)
	}

	if welcome.Type != TypeWelcome {
		stream.Close()
		return fmt.Errorf("unexpected response type: %s", welcome.Type)
	}

	connCtx, cancel := context.WithCancel(context.Background())
	cc := &clientConn{
		hostPeerID: hostPeerID,
		groupID:    groupID,
		stream:     stream,
		encoder:    enc,
		cancel:     cancel,
	}

	m.mu.Lock()
	m.activeConn = cc
	m.mu.Unlock()

	// Extract group name from welcome payload for subscription storage
	groupName := ""
	appType := ""
	if wp, ok := welcome.Payload.(map[string]any); ok {
		if n, ok := wp["group_name"].(string); ok {
			groupName = n
		}
		if a, ok := wp["app_type"].(string); ok {
			appType = a
		}
	}

	// Store subscription
	m.db.AddSubscription(hostPeerID, groupID, groupName, appType, "member")

	m.notifyListeners(&Event{Type: TypeWelcome, Group: groupID, Payload: welcome.Payload})

	log.Printf("GROUP: Joined group %s on host %s", groupID, hostPeerID)

	// Spawn read goroutine for incoming messages from host
	go m.clientReadLoop(connCtx, dec, cc)

	return nil
}

func (m *Manager) clientReadLoop(ctx context.Context, dec *json.Decoder, cc *clientConn) {
	defer func() {
		m.mu.Lock()
		if m.activeConn == cc {
			m.activeConn = nil
		}
		m.mu.Unlock()
		cc.stream.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var msg Message
		if err := dec.Decode(&msg); err != nil {
			log.Printf("GROUP: Client connection lost: %v", err)
			m.notifyListeners(&Event{Type: TypeClose, Group: cc.groupID})
			return
		}

		switch msg.Type {
		case TypeClose:
			log.Printf("GROUP: Group %s closed by host", cc.groupID)
			m.db.RemoveSubscription(cc.hostPeerID, cc.groupID)
			m.notifyListeners(&Event{Type: TypeClose, Group: cc.groupID})
			cc.cancel()
			return
		case TypeMembers, TypeMsg, TypeState, TypeError:
			m.notifyListeners(&Event{Type: msg.Type, Group: msg.Group, From: msg.From, Payload: msg.Payload})
		}
	}
}

// SendToGroup sends a message through the active client connection.
func (m *Manager) SendToGroup(payload any) error {
	m.mu.RLock()
	cc := m.activeConn
	m.mu.RUnlock()

	if cc == nil {
		return fmt.Errorf("not connected to any group")
	}

	return cc.encoder.Encode(Message{
		Type:    TypeMsg,
		Group:   cc.groupID,
		Payload: payload,
	})
}

// LeaveGroup disconnects from the current remote group.
func (m *Manager) LeaveGroup() error {
	m.mu.Lock()
	cc := m.activeConn
	m.activeConn = nil
	m.mu.Unlock()

	if cc == nil {
		return fmt.Errorf("not connected to any group")
	}

	// Send leave message
	cc.encoder.Encode(Message{Type: TypeLeave, Group: cc.groupID})
	cc.cancel()
	cc.stream.Close()

	m.db.RemoveSubscription(cc.hostPeerID, cc.groupID)
	m.notifyListeners(&Event{Type: TypeLeave, Group: cc.groupID})

	log.Printf("GROUP: Left group %s", cc.groupID)
	return nil
}

// ActiveGroup returns info about the current client connection, if any.
func (m *Manager) ActiveGroup() (hostPeerID, groupID string, connected bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.activeConn != nil {
		return m.activeConn.hostPeerID, m.activeConn.groupID, true
	}
	return "", "", false
}

// ─── SSE event subscription ─────────────────────────────────────────────────

// Subscribe returns a channel that receives group events.
func (m *Manager) Subscribe() <-chan *Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan *Event, 10)
	m.listeners = append(m.listeners, ch)
	return ch
}

// Unsubscribe removes a listener channel.
func (m *Manager) Unsubscribe(ch <-chan *Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, listener := range m.listeners {
		if listener == ch {
			close(listener)
			m.listeners = append(m.listeners[:i], m.listeners[i+1:]...)
			return
		}
	}
}

func (m *Manager) notifyListeners(evt *Event) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, listener := range m.listeners {
		select {
		case listener <- evt:
		default:
			// Listener buffer full, skip
		}
	}
}

// ─── Subscriptions (client-side persistence) ─────────────────────────────────

// ListSubscriptions returns all stored group subscriptions.
func (m *Manager) ListSubscriptions() ([]storage.SubscriptionRow, error) {
	return m.db.ListSubscriptions()
}

// RejoinSubscription attempts to reconnect to a previously subscribed group.
func (m *Manager) RejoinSubscription(ctx context.Context, hostPeerID, groupID string) error {
	// Best-effort connect first (peer might be discovered via mDNS)
	pid, err := peer.Decode(hostPeerID)
	if err != nil {
		return fmt.Errorf("invalid host peer ID: %w", err)
	}
	_ = m.host.Connect(ctx, peer.AddrInfo{ID: pid})

	return m.JoinRemoteGroup(ctx, hostPeerID, groupID)
}

// RemoveSubscription removes a stale subscription from the database.
func (m *Manager) RemoveSubscription(hostPeerID, groupID string) error {
	return m.db.RemoveSubscription(hostPeerID, groupID)
}

// reconnectSubscriptions attempts to rejoin subscribed groups on startup.
// Waits for peer discovery before attempting connections.
func (m *Manager) reconnectSubscriptions() {
	// Wait for mDNS / rendezvous peer discovery
	time.Sleep(6 * time.Second)

	subs, err := m.db.ListSubscriptions()
	if err != nil || len(subs) == 0 {
		return
	}

	for _, sub := range subs {
		// Only one active connection at a time
		m.mu.RLock()
		hasActive := m.activeConn != nil
		m.mu.RUnlock()
		if hasActive {
			break
		}

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		err := m.RejoinSubscription(ctx, sub.HostPeerID, sub.GroupID)
		cancel()

		if err != nil {
			// Shorten verbose libp2p dial errors to first line.
			msg := err.Error()
			if i := strings.Index(msg, "\n"); i > 0 {
				msg = msg[:i]
			}
			log.Printf("GROUP: Auto-reconnect to %s failed: %s", sub.GroupID, msg)
		} else {
			log.Printf("GROUP: Auto-reconnected to group %s on host %s", sub.GroupID, sub.HostPeerID)
			break
		}
	}
}

// ─── Hosted group helpers ────────────────────────────────────────────────────

func (g *hostedGroup) memberList(hostID string) []MemberInfo {
	members := make([]MemberInfo, 0, len(g.members)+1)
	if g.hostJoined {
		members = append(members, MemberInfo{
			PeerID:   hostID,
			JoinedAt: g.hostJoinedAt,
		})
	}
	for _, mc := range g.members {
		members = append(members, MemberInfo{
			PeerID:   mc.peerID,
			JoinedAt: mc.joinedAt,
		})
	}
	return members
}

func (g *hostedGroup) broadcast(msg Message, excludePeerID string) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for pid, mc := range g.members {
		if pid == excludePeerID {
			continue
		}
		select {
		case mc.sendCh <- msg:
		default:
			// Slow peer; drop message rather than blocking others.
			log.Printf("GROUP: Send buffer full for %s, dropping message", pid)
		}
	}
}

const memberWriteTimeout = 5 * time.Second

// drainLoop writes queued messages from sendCh to the stream with a deadline.
// If a write times out or fails, the member is disconnected.
func (mc *memberConn) drainLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-mc.sendCh:
			if !ok {
				return
			}
			if dl, ok := mc.stream.(interface{ SetWriteDeadline(time.Time) error }); ok {
				_ = dl.SetWriteDeadline(time.Now().Add(memberWriteTimeout))
			}
			if err := mc.encoder.Encode(msg); err != nil {
				log.Printf("GROUP: Write to %s failed: %v (disconnecting)", mc.peerID, err)
				mc.cancel()
				return
			}
			if dl, ok := mc.stream.(interface{ SetWriteDeadline(time.Time) error }); ok {
				_ = dl.SetWriteDeadline(time.Time{}) // clear deadline
			}
		}
	}
}

// ─── Invitations ─────────────────────────────────────────────────────────────

// inviteMsg is the wire format for a group invitation.
type inviteMsg struct {
	GroupID    string `json:"group_id"`
	GroupName  string `json:"group_name"`
	HostPeerID string `json:"host_peer_id"`
}

// InvitePeer sends a group invitation to a remote peer.
// The peer's invite handler will auto-join the group.
func (m *Manager) InvitePeer(ctx context.Context, peerID, groupID string) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	pid, err := peer.Decode(peerID)
	if err != nil {
		return fmt.Errorf("invalid peer ID: %w", err)
	}

	// Best-effort connect
	_ = m.host.Connect(ctx, peer.AddrInfo{ID: pid})

	s, err := m.host.NewStream(ctx, pid, protocol.ID(proto.GroupInviteProtoID))
	if err != nil {
		return fmt.Errorf("failed to open invite stream: %w", err)
	}
	defer s.Close()

	inv := inviteMsg{
		GroupID:    groupID,
		GroupName:  hg.info.Name,
		HostPeerID: m.selfID,
	}
	if err := json.NewEncoder(s).Encode(inv); err != nil {
		return fmt.Errorf("failed to send invite: %w", err)
	}

	log.Printf("GROUP: Sent invite for group %s to peer %s", groupID, peerID)
	return nil
}

// handleInviteStream processes incoming group invitations from a host.
// It stores the subscription immediately so the invited peer can always see
// it, then attempts to auto-join in the background.
func (m *Manager) handleInviteStream(s network.Stream) {
	defer s.Close()

	var inv inviteMsg
	if err := json.NewDecoder(s).Decode(&inv); err != nil {
		log.Printf("GROUP: Failed to decode invite: %v", err)
		return
	}

	log.Printf("GROUP: Received invite for group %s from host %s", inv.GroupID, inv.HostPeerID)

	// Store the subscription immediately so the invited peer can see the group
	// in their list even if the auto-join fails due to transient connectivity.
	// JoinRemoteGroup will upsert it again with the full app_type from the welcome.
	_ = m.db.AddSubscription(inv.HostPeerID, inv.GroupID, inv.GroupName, "", "member")

	// Notify local listeners so the groups page refreshes immediately.
	m.notifyListeners(&Event{
		Type:  "invite",
		Group: inv.GroupID,
		From:  inv.HostPeerID,
		Payload: map[string]any{
			"group_id":   inv.GroupID,
			"group_name": inv.GroupName,
			"host":       inv.HostPeerID,
		},
	})

	// Auto-join in a goroutine so we don't block the stream handler.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := m.JoinRemoteGroup(ctx, inv.HostPeerID, inv.GroupID); err != nil {
			log.Printf("GROUP: Auto-join after invite failed: %v", err)
		}
	}()
}

// ─── Shutdown ────────────────────────────────────────────────────────────────

// SendToGroupAsHost sends a message to all members of a hosted group from the host.
func (m *Manager) SendToGroupAsHost(groupID string, payload any) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	msg := Message{
		Type:    TypeMsg,
		Group:   groupID,
		From:    m.selfID,
		Payload: payload,
	}

	hg.broadcast(msg, "")
	m.notifyListeners(&Event{Type: TypeMsg, Group: groupID, From: m.selfID, Payload: payload})
	return nil
}

// Close shuts down the group manager, closing all streams and listeners.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close all hosted groups
	for _, hg := range m.groups {
		hg.mu.Lock()
		for _, mc := range hg.members {
			mc.cancel()
			mc.stream.Close()
		}
		hg.members = nil
		hg.mu.Unlock()
	}

	// Close client connection
	if m.activeConn != nil {
		m.activeConn.cancel()
		m.activeConn.stream.Close()
		m.activeConn = nil
	}

	// Close all listener channels
	for _, listener := range m.listeners {
		close(listener)
	}
	m.listeners = nil

	return nil
}

// SelfID returns the local peer ID.
func (m *Manager) SelfID() string {
	return m.selfID
}

// IsPeerInGroup returns true if the given peer is a current member of a hosted group.
// Checks the members map and whether the host has joined.
func (m *Manager) IsPeerInGroup(peerID, groupID string) bool {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	hg.mu.RLock()
	defer hg.mu.RUnlock()

	if peerID == m.selfID && hg.hostJoined {
		return true
	}

	_, isMember := hg.members[peerID]
	return isMember
}

// IsGroupHost returns true if this peer hosts the given group.
func (m *Manager) IsGroupHost(groupID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.groups[groupID]
	return exists
}

// HostedGroupInfo returns the info for a hosted group.
func (m *Manager) HostedGroupInfo(groupID string) (storage.GroupRow, bool) {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()
	if !exists {
		return storage.GroupRow{}, false
	}
	return hg.info, true
}
