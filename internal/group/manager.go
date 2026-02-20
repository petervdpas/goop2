package group

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
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

// Handler is implemented by subsystems that process events for a specific group app_type.
// HandleGroupEvent is called asynchronously for every event whose group has the matching app_type.
type Handler interface {
	HandleGroupEvent(evt *Event)
}

// ActiveGroupInfo holds info about one active client-side group connection.
type ActiveGroupInfo struct {
	HostPeerID string `json:"host_peer_id"`
	GroupID    string `json:"group_id"`
	AppType    string `json:"app_type"`
}

// Manager handles the group protocol, both host-side (relay) and client-side (connection).
type Manager struct {
	host   host.Host
	db     *storage.DB
	mu     sync.RWMutex
	selfID string

	// Host-side: groupID -> *hostedGroup
	groups map[string]*hostedGroup

	// Client-side: outbound connections keyed by groupID (one per group).
	activeConns map[string]*clientConn

	// Local SSE listeners
	listeners []chan *Event

	// Type-specific event handlers keyed by app_type.
	handlers map[string]Handler
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
	sendCh   chan Message    // buffered outbound queue for non-blocking broadcast
	lastPong atomic.Int64   // unix millis of last pong received from this member
}

type clientConn struct {
	hostPeerID string
	groupID    string
	appType    string
	volatile   bool
	stream     network.Stream
	encoder    *json.Encoder
	sendMu     sync.Mutex   // serialises writes to encoder (handler + pong goroutine)
	cancel     context.CancelFunc
	membersMu  sync.RWMutex
	members    []MemberInfo // last known member list from host
}

// New creates a new group manager and registers the stream handler.
func New(h host.Host, db *storage.DB) *Manager {
	m := &Manager{
		host:        h,
		db:          db,
		selfID:      h.ID().String(),
		groups:      make(map[string]*hostedGroup),
		activeConns: make(map[string]*clientConn),
		listeners:   make([]chan *Event, 0),
		handlers:    make(map[string]Handler),
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
	// Start ping goroutine: keeps the connection alive and detects stalled peers.
	go m.pingLoop(ctx, mc, groupID)

	log.Printf("GROUP: %s joined group %s", remotePeer, groupID)

	// Send welcome to the new member
	enc.Encode(Message{
		Type:  TypeWelcome,
		Group: groupID,
		Payload: WelcomePayload{
			GroupName:  hg.info.Name,
			AppType:    hg.info.AppType,
			MaxMembers: hg.info.MaxMembers,
			Volatile:   hg.info.Volatile,
			Members:    memberList,
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

	// Persist member list for stable groups so peers can browse each other offline.
	// Volatile game groups are ephemeral — no point persisting their member lists.
	if !hg.info.Volatile && len(memberList) > 0 {
		peerIDs := make([]string, len(memberList))
		for i, mi := range memberList {
			peerIDs[i] = mi.PeerID
		}
		_ = m.db.UpsertGroupMembers(groupID, peerIDs)
	}

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

	// Persist updated member list for stable groups only
	if !hg.info.Volatile {
		updatedIDs := make([]string, len(updatedMembers))
		for i, mi := range updatedMembers {
			updatedIDs[i] = mi.PeerID
		}
		_ = m.db.UpsertGroupMembers(groupID, updatedIDs)
	}
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
		case TypePong:
			mc.lastPong.Store(nowMillis())
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
func (m *Manager) CreateGroup(id, name, appType string, maxMembers int, volatile bool) error {
	// Volatile game groups: close any existing hosted group of the same type
	// before creating a new one (new game = fresh group).
	if volatile {
		m.mu.RLock()
		var toClose []string
		for gid, hg := range m.groups {
			hg.mu.RLock()
			if hg.info.Volatile && hg.info.AppType == appType {
				toClose = append(toClose, gid)
			}
			hg.mu.RUnlock()
		}
		m.mu.RUnlock()
		for _, gid := range toClose {
			_ = m.CloseGroup(gid)
		}
	}

	// Enforce hosted group limit — volatile groups are excluded from the cap
	if !volatile {
		m.mu.RLock()
		stableCount := 0
		for _, hg := range m.groups {
			hg.mu.RLock()
			if !hg.info.Volatile {
				stableCount++
			}
			hg.mu.RUnlock()
		}
		m.mu.RUnlock()
		if stableCount >= maxHostedGroups {
			return fmt.Errorf("maximum of %d hosted groups reached", maxHostedGroups)
		}
	}

	if err := m.db.CreateGroup(id, name, appType, maxMembers, volatile); err != nil {
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
	_ = m.db.DeleteGroupMembers(groupID)

	m.notifyListeners(&Event{Type: TypeClose, Group: groupID})

	log.Printf("GROUP: Closed group %s", groupID)
	return nil
}

// ListHostedGroups returns all hosted groups.
func (m *Manager) ListHostedGroups() ([]storage.GroupRow, error) {
	return m.db.ListGroups()
}

// KickMember disconnects a member from a hosted group.
func (m *Manager) KickMember(groupID, peerID string) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	hg.mu.Lock()
	mc, ok := hg.members[peerID]
	if ok {
		kickMsg := Message{Type: TypeClose, Group: groupID}
		mc.encoder.Encode(kickMsg)
		mc.cancel()
		mc.stream.Close()
		delete(hg.members, peerID)
	}
	hg.mu.Unlock()

	if !ok {
		return fmt.Errorf("member not found: %s", peerID)
	}

	m.notifyListeners(&Event{Type: "leave", Group: groupID, From: peerID})
	log.Printf("GROUP: Kicked %s from %s", peerID, groupID)
	return nil
}

// SetMaxMembers updates the max_members limit for a hosted group.
// A limit of 0 means unlimited.
func (m *Manager) SetMaxMembers(groupID string, max int) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	hg.mu.Lock()
	hg.info.MaxMembers = max
	hg.mu.Unlock()

	if err := m.db.SetMaxMembers(groupID, max); err != nil {
		return fmt.Errorf("update max members: %w", err)
	}

	hg.mu.RLock()
	meta := MetaPayload{GroupName: hg.info.Name, AppType: hg.info.AppType, MaxMembers: max}
	hg.mu.RUnlock()

	hg.broadcast(Message{Type: TypeMeta, Group: groupID, Payload: meta}, "")
	m.notifyListeners(&Event{Type: TypeMeta, Group: groupID, Payload: meta})

	log.Printf("GROUP: Set max members for %s to %d", groupID, max)
	return nil
}

// UpdateGroupMeta updates the name and max_members of a hosted group and broadcasts
// the change to all connected members via a TypeMeta message.
func (m *Manager) UpdateGroupMeta(groupID, name string, maxMembers int) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	hg.mu.Lock()
	hg.info.Name = name
	hg.info.MaxMembers = maxMembers
	appType := hg.info.AppType
	hg.mu.Unlock()

	if err := m.db.UpdateGroup(groupID, name, maxMembers); err != nil {
		return fmt.Errorf("update group: %w", err)
	}

	meta := MetaPayload{GroupName: name, AppType: appType, MaxMembers: maxMembers}
	hg.broadcast(Message{Type: TypeMeta, Group: groupID, Payload: meta}, "")
	m.notifyListeners(&Event{Type: TypeMeta, Group: groupID, Payload: meta})

	log.Printf("GROUP: Updated meta for %s — name=%q maxMembers=%d", groupID, name, maxMembers)
	return nil
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

// StoredGroupMembers returns the persisted member peer IDs for a group.
// Works regardless of current connection state — reads from DB.
func (m *Manager) StoredGroupMembers(groupID string) []string {
	peers, _ := m.db.ListGroupMembers(groupID)
	return peers
}

// ClientGroupMembers returns the last known member list for a group we joined as a client.
// Returns nil if not connected as a client to the given group.
func (m *Manager) ClientGroupMembers(groupID string) []MemberInfo {
	m.mu.RLock()
	cc := m.activeConns[groupID]
	m.mu.RUnlock()

	if cc == nil {
		return nil
	}

	cc.membersMu.RLock()
	defer cc.membersMu.RUnlock()
	return cc.members
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
	// Auto-leave any existing connection to this same group (re-join scenario).
	// Other group connections are unaffected — each group has its own slot.
	m.mu.Lock()
	old := m.activeConns[groupID]
	if old != nil {
		delete(m.activeConns, groupID)
	}
	m.mu.Unlock()
	if old != nil {
		old.sendMu.Lock()
		old.encoder.Encode(Message{Type: TypeLeave, Group: old.groupID}) //nolint:errcheck
		old.sendMu.Unlock()
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

	// Extract group name, app type, max members, volatile flag, and initial member list from welcome payload
	groupName := ""
	appType := ""
	maxMembers := 0
	volatile := false
	var initMembers []MemberInfo
	if wp, ok := welcome.Payload.(map[string]any); ok {
		if n, ok := wp["group_name"].(string); ok {
			groupName = n
		}
		if a, ok := wp["app_type"].(string); ok {
			appType = a
		}
		if mm, ok := wp["max_members"].(float64); ok {
			maxMembers = int(mm)
		}
		if v, ok := wp["volatile"].(bool); ok {
			volatile = v
		}
		if rawMembers, ok := wp["members"]; ok {
			if b, err := json.Marshal(rawMembers); err == nil {
				var ml []MemberInfo
				if json.Unmarshal(b, &ml) == nil {
					initMembers = ml
				}
			}
		}
	}

	connCtx, cancel := context.WithCancel(context.Background())
	cc := &clientConn{
		hostPeerID: hostPeerID,
		groupID:    groupID,
		appType:    appType,
		volatile:   volatile,
		stream:     stream,
		encoder:    enc,
		cancel:     cancel,
		members:    initMembers,
	}

	m.mu.Lock()
	m.activeConns[groupID] = cc
	m.mu.Unlock()

	// Persist member list for stable groups so browse works when host is offline.
	// Volatile game groups are ephemeral — skip persistence.
	if !volatile && len(initMembers) > 0 {
		peerIDs := make([]string, len(initMembers))
		for i, mi := range initMembers {
			peerIDs[i] = mi.PeerID
		}
		_ = m.db.UpsertGroupMembers(groupID, peerIDs)
	}

	// Volatile game groups: wipe stale subscriptions of the same type before storing the new one.
	if volatile {
		if subs, err := m.db.ListSubscriptions(); err == nil {
			for _, s := range subs {
				if s.AppType == appType && s.GroupID != groupID {
					_ = m.db.RemoveSubscription(s.HostPeerID, s.GroupID)
					_ = m.db.DeleteGroupMembers(s.GroupID)
				}
			}
		}
	}

	// Store subscription with full metadata
	m.db.AddSubscription(hostPeerID, groupID, groupName, appType, maxMembers, volatile, "member")

	m.notifyListeners(&Event{Type: TypeWelcome, Group: groupID, From: hostPeerID, Payload: welcome.Payload})

	log.Printf("GROUP: Joined group %s on host %s", groupID, hostPeerID)

	// Spawn read goroutine for incoming messages from host
	go m.clientReadLoop(connCtx, dec, cc)

	return nil
}

func (m *Manager) clientReadLoop(ctx context.Context, dec *json.Decoder, cc *clientConn) {
	defer func() {
		m.mu.Lock()
		if m.activeConns[cc.groupID] == cc {
			delete(m.activeConns, cc.groupID)
		}
		m.mu.Unlock()
		cc.stream.Close()
	}()

	// Expect a ping from the host at least every pingInterval; disconnect if
	// clientPingTimeout elapses with no data at all from the host.
	_ = cc.stream.SetReadDeadline(time.Now().Add(clientPingTimeout))

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var msg Message
		if err := dec.Decode(&msg); err != nil {
			log.Printf("GROUP: Client connection lost (group %s): %v", cc.groupID, err)
			m.notifyListeners(&Event{Type: TypeClose, Group: cc.groupID})
			return
		}

		// Reset deadline after each successful message from host.
		_ = cc.stream.SetReadDeadline(time.Now().Add(clientPingTimeout))

		switch msg.Type {
		case TypeClose:
			log.Printf("GROUP: Group %s closed by host", cc.groupID)
			m.db.RemoveSubscription(cc.hostPeerID, cc.groupID)
			m.notifyListeners(&Event{Type: TypeClose, Group: cc.groupID})
			cc.cancel()
			return
		case TypePing:
			// Reply to host's keepalive ping.
			cc.sendMu.Lock()
			cc.encoder.Encode(Message{Type: TypePong, Group: cc.groupID}) //nolint:errcheck
			cc.sendMu.Unlock()
		case TypeMembers:
			// Keep local member list up to date and persist to DB
			if rawPayload, ok := msg.Payload.(map[string]any); ok {
				if rawMembers, ok := rawPayload["members"]; ok {
					if b, err := json.Marshal(rawMembers); err == nil {
						var ml []MemberInfo
						if json.Unmarshal(b, &ml) == nil {
							cc.membersMu.Lock()
							cc.members = ml
							cc.membersMu.Unlock()
							// Persist for stable groups only
							if !cc.volatile {
								peerIDs := make([]string, len(ml))
								for i, mi := range ml {
									peerIDs[i] = mi.PeerID
								}
								_ = m.db.UpsertGroupMembers(cc.groupID, peerIDs)
							}
						}
					}
				}
			}
			m.notifyListeners(&Event{Type: msg.Type, Group: msg.Group, From: msg.From, Payload: msg.Payload})
		case TypeMeta:
			// Host updated group metadata — refresh stored subscription
			if rawPayload, ok := msg.Payload.(map[string]any); ok {
				if b, err := json.Marshal(rawPayload); err == nil {
					var mp MetaPayload
					if json.Unmarshal(b, &mp) == nil && mp.GroupName != "" {
						_ = m.db.AddSubscription(cc.hostPeerID, cc.groupID, mp.GroupName, mp.AppType, mp.MaxMembers, cc.volatile, "member")
					}
				}
			}
			m.notifyListeners(&Event{Type: msg.Type, Group: msg.Group, Payload: msg.Payload})
		case TypeMsg, TypeState, TypeError:
			m.notifyListeners(&Event{Type: msg.Type, Group: msg.Group, From: msg.From, Payload: msg.Payload})
		}
	}
}

// SendToGroup sends a message through the client connection for the given group.
func (m *Manager) SendToGroup(groupID string, payload any) error {
	m.mu.RLock()
	cc := m.activeConns[groupID]
	m.mu.RUnlock()

	if cc == nil {
		return fmt.Errorf("not connected to group %s", groupID)
	}

	return cc.encoder.Encode(Message{
		Type:    TypeMsg,
		Group:   cc.groupID,
		Payload: payload,
	})
}

// LeaveGroup disconnects from the specified remote group.
func (m *Manager) LeaveGroup(groupID string) error {
	m.mu.Lock()
	cc := m.activeConns[groupID]
	if cc != nil {
		delete(m.activeConns, groupID)
	}
	m.mu.Unlock()

	if cc == nil {
		return fmt.Errorf("not connected to group %s", groupID)
	}

	// Send leave message
	cc.sendMu.Lock()
	cc.encoder.Encode(Message{Type: TypeLeave, Group: cc.groupID}) //nolint:errcheck
	cc.sendMu.Unlock()
	cc.cancel()
	cc.stream.Close()

	m.db.RemoveSubscription(cc.hostPeerID, cc.groupID)
	_ = m.db.DeleteGroupMembers(cc.groupID)
	m.notifyListeners(&Event{Type: TypeLeave, Group: cc.groupID})

	log.Printf("GROUP: Left group %s", cc.groupID)
	return nil
}

// ActiveGroup returns the host peer ID for an active client connection to the given group.
func (m *Manager) ActiveGroup(groupID string) (hostPeerID string, connected bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if cc, ok := m.activeConns[groupID]; ok {
		return cc.hostPeerID, true
	}
	return "", false
}

// ActiveGroups returns info about all active client-side group connections.
func (m *Manager) ActiveGroups() []ActiveGroupInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]ActiveGroupInfo, 0, len(m.activeConns))
	for _, cc := range m.activeConns {
		result = append(result, ActiveGroupInfo{
			HostPeerID: cc.hostPeerID,
			GroupID:    cc.groupID,
			AppType:    cc.appType,
		})
	}
	return result
}

// IsGroupConnected returns true if we have an active client connection to the given group.
func (m *Manager) IsGroupConnected(groupID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.activeConns[groupID]
	return ok
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

// RegisterHandler registers h to receive group events whose group app_type matches appType.
// The handler is called in a dedicated goroutine per event.
func (m *Manager) RegisterHandler(appType string, h Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[appType] = h
}

// appTypeForGroupLocked returns the app_type for a group. Caller must hold m.mu (any mode).
func (m *Manager) appTypeForGroupLocked(groupID string) string {
	if hg, ok := m.groups[groupID]; ok {
		return hg.info.AppType
	}
	if cc, ok := m.activeConns[groupID]; ok {
		return cc.appType
	}
	return ""
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
	// Dispatch to the registered type-specific handler (async to avoid blocking the caller).
	appType := m.appTypeForGroupLocked(evt.Group)
	if h, ok := m.handlers[appType]; ok {
		go h.HandleGroupEvent(evt)
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
		// Skip groups we're already connected to
		m.mu.RLock()
		_, alreadyConnected := m.activeConns[sub.GroupID]
		m.mu.RUnlock()
		if alreadyConnected {
			continue
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

const (
	memberWriteTimeout = 5 * time.Second
	pingInterval       = 30 * time.Second
	pingPongTimeout    = 75 * time.Second // disconnect member after 2+ missed pings
	clientPingTimeout  = 75 * time.Second // client disconnects if host goes silent
	maxHostedGroups    = 50               // hard cap on hosted groups per peer
)


// pingLoop sends periodic TypePing messages to a member and disconnects them
// if no TypePong is received within pingPongTimeout.
func (m *Manager) pingLoop(ctx context.Context, mc *memberConn, groupID string) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check pong freshness only after the first ping cycle
			last := mc.lastPong.Load()
			if last > 0 && time.Since(time.UnixMilli(last)) > pingPongTimeout {
				log.Printf("GROUP: Member %s ping timeout, disconnecting", mc.peerID)
				mc.cancel()
				return
			}
			select {
			case mc.sendCh <- Message{Type: TypePing, Group: groupID}:
			default:
				// Buffer full; drainLoop write deadline will catch truly dead connections.
			}
		}
	}
}

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
	AppType    string `json:"app_type"`
	Volatile   bool   `json:"volatile"`
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
		AppType:    hg.info.AppType,
		Volatile:   hg.info.Volatile,
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

	// Volatile game groups: wipe stale subscriptions of the same type before
	// storing this new invite — it's a new game session.
	if inv.Volatile {
		if subs, err := m.db.ListSubscriptions(); err == nil {
			for _, s := range subs {
				if s.AppType == inv.AppType && s.GroupID != inv.GroupID {
					_ = m.db.RemoveSubscription(s.HostPeerID, s.GroupID)
					_ = m.db.DeleteGroupMembers(s.GroupID)
				}
			}
		}
	}

	// Store the subscription immediately so the invited peer can see the group
	// in their list even if the auto-join fails due to transient connectivity.
	// JoinRemoteGroup will upsert it again with full metadata from the welcome.
	_ = m.db.AddSubscription(inv.HostPeerID, inv.GroupID, inv.GroupName, inv.AppType, 0, inv.Volatile, "member")

	// Notify local listeners so the groups page can prompt the user.
	m.notifyListeners(&Event{
		Type:  "invite",
		Group: inv.GroupID,
		From:  inv.HostPeerID,
		Payload: map[string]any{
			"group_id":   inv.GroupID,
			"group_name": inv.GroupName,
			"host":       inv.HostPeerID,
			"app_type":   inv.AppType,
		},
	})

	// Realtime channels (e.g. video calls) require an immediate auto-join so
	// that signaling messages can flow before the user interacts with the UI.
	if inv.AppType == "realtime" {
		go func() {
			if err := m.JoinRemoteGroup(context.Background(), inv.HostPeerID, inv.GroupID); err != nil {
				log.Printf("GROUP: Auto-join realtime group %s failed: %v", inv.GroupID, err)
			}
		}()
	}
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

	// Close all client connections
	for _, cc := range m.activeConns {
		cc.cancel()
		cc.stream.Close()
	}
	m.activeConns = nil

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

// IsKnownGroupPeer returns true if remotePeer is a verified member of groupID.
// Works from both perspectives: as the host (authoritative member list) or as
// a client member (list received from the host via TypeWelcome/TypeMembers).
// Returns false if this peer has no knowledge of the group at all.
func (m *Manager) IsKnownGroupPeer(remotePeer, groupID string) bool {
	// Host side: authoritative check
	m.mu.RLock()
	_, isHost := m.groups[groupID]
	cc := m.activeConns[groupID]
	m.mu.RUnlock()

	if isHost {
		return m.IsPeerInGroup(remotePeer, groupID)
	}

	// Client side: check against our known member list for this group
	if cc != nil {
		// The host itself is always allowed to access our docs
		if remotePeer == cc.hostPeerID {
			return true
		}
		cc.membersMu.RLock()
		defer cc.membersMu.RUnlock()
		for _, mi := range cc.members {
			if mi.PeerID == remotePeer {
				return true
			}
		}
		return false
	}

	// We don't know about this group — serve nothing
	return false
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
