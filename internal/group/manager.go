package group

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/mq"
	"github.com/petervdpas/goop2/internal/storage"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Event is emitted to local MQ listeners (via PublishLocal).
type Event struct {
	Type    string `json:"type"`
	Group   string `json:"group"`
	From    string `json:"from,omitempty"`
	Payload any    `json:"payload,omitempty"`
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
	mq     *mq.Manager
	mu     sync.RWMutex
	selfID string

	// Host-side: groupID -> *hostedGroup
	groups map[string]*hostedGroup

	// Client-side: outbound connections keyed by groupID (one per group).
	activeConns map[string]*clientConn

	// Pending join channels: groupID -> channel waiting for welcome
	pendingJoinsMu sync.Mutex
	pendingJoins   map[string]chan WelcomePayload

	// Type-specific event handlers keyed by app_type.
	handlers map[string]Handler

	// MQ unsubscribe functions
	unsubGroup  func()
	unsubInvite func()
}

type memberMeta struct {
	peerID   string
	joinedAt int64
}

type hostedGroup struct {
	info         storage.GroupRow
	members      map[string]*memberMeta // peerID -> meta
	hostJoined   bool
	hostJoinedAt int64
	mu           sync.RWMutex
	cancelPing   context.CancelFunc
}

type clientConn struct {
	hostPeerID string
	groupID    string
	appType    string
	volatile   bool
	membersMu  sync.RWMutex
	members    []MemberInfo // last known member list from host
}

const (
	pingInterval    = 60 * time.Second
	maxHostedGroups = 50 // hard cap on hosted groups per peer
)

// New creates a new group manager and registers MQ subscriptions.
func New(h host.Host, db *storage.DB, mqMgr *mq.Manager) *Manager {
	m := &Manager{
		host:         h,
		db:           db,
		mq:           mqMgr,
		selfID:       h.ID().String(),
		groups:       make(map[string]*hostedGroup),
		activeConns:  make(map[string]*clientConn),
		pendingJoins: make(map[string]chan WelcomePayload),
		handlers:     make(map[string]Handler),
	}

	// Load existing groups from DB into memory (restore host-joined state)
	if groups, err := db.ListGroups(); err == nil {
		for _, g := range groups {
			ctx, cancel := context.WithCancel(context.Background())
			hg := &hostedGroup{
				info:       g,
				members:    make(map[string]*memberMeta),
				hostJoined: g.HostJoined,
				cancelPing: cancel,
			}
			m.groups[g.ID] = hg
			go m.pingGroupLoop(ctx, g.ID)
		}
	}

	// Register MQ subscriptions
	m.unsubGroup = mqMgr.SubscribeTopic("group:", func(from, topic string, payload any) {
		rest := strings.TrimPrefix(topic, "group:")
		idx := strings.Index(rest, ":")
		if idx < 0 {
			return
		}
		groupID, msgType := rest[:idx], rest[idx+1:]
		m.handleMQMessage(from, groupID, msgType, payload)
	})

	m.unsubInvite = mqMgr.SubscribeTopic("group.invite", func(from, topic string, payload any) {
		m.handleInvite(from, payload)
	})

	log.Printf("GROUP: MQ transport registered (group: + group.invite)")

	// Auto-reconnect to subscribed groups in the background
	go m.reconnectSubscriptions()

	return m
}

// ─── MQ message routing ───────────────────────────────────────────────────────

func (m *Manager) handleMQMessage(from, groupID, msgType string, payload any) {
	m.mu.RLock()
	hg := m.groups[groupID]
	cc := m.activeConns[groupID]
	m.mu.RUnlock()

	m.pendingJoinsMu.Lock()
	pendingCh := m.pendingJoins[groupID]
	m.pendingJoinsMu.Unlock()

	switch {
	case hg != nil:
		m.handleHostMessage(from, hg, groupID, msgType, payload)
	case pendingCh != nil && msgType == TypeWelcome:
		m.handleWelcomeForPendingJoin(groupID, payload, pendingCh)
	case cc != nil:
		m.handleMemberMessage(from, cc, groupID, msgType, payload)
	default:
		log.Printf("GROUP: Received %s for unknown/pending group %s (from %s)", msgType, groupID, shortID(from))
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// handleHostMessage processes messages received by the group host from members.
func (m *Manager) handleHostMessage(from string, hg *hostedGroup, groupID, msgType string, payload any) {
	switch msgType {
	case TypeJoin:
		hg.mu.Lock()
		if hg.info.MaxMembers > 0 && len(hg.members) >= hg.info.MaxMembers {
			hg.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = m.mq.Send(ctx, from, "group:"+groupID+":"+TypeError,
				ErrorPayload{Code: "full", Message: "group is full"})
			return
		}
		hg.members[from] = &memberMeta{peerID: from, joinedAt: nowMillis()}
		memberList := hg.memberList(m.selfID)
		appType := hg.info.AppType
		volatile := hg.info.Volatile
		name := hg.info.Name
		maxMembers := hg.info.MaxMembers
		hg.mu.Unlock()

		log.Printf("GROUP: %s joined group %s", shortID(from), groupID)

		// Send welcome to new member
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, _ = m.mq.Send(ctx, from, "group:"+groupID+":"+TypeWelcome, WelcomePayload{
			GroupName:  name,
			AppType:    appType,
			MaxMembers: maxMembers,
			Volatile:   volatile,
			Members:    memberList,
		})
		cancel()

		// Broadcast updated member list to all other members
		m.broadcastToGroup(hg, groupID, TypeMembers, MembersPayload{Members: memberList}, from)
		m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, Payload: MembersPayload{Members: memberList}})

		// Persist member list for stable groups
		if !volatile && len(memberList) > 0 {
			peerIDs := make([]string, len(memberList))
			for i, mi := range memberList {
				peerIDs[i] = mi.PeerID
			}
			_ = m.db.UpsertGroupMembers(groupID, peerIDs)
		}

	case TypeLeave:
		hg.mu.Lock()
		delete(hg.members, from)
		members := hg.memberList(m.selfID)
		volatile := hg.info.Volatile
		hg.mu.Unlock()

		log.Printf("GROUP: %s left group %s", shortID(from), groupID)

		m.broadcastToGroup(hg, groupID, TypeMembers, MembersPayload{Members: members}, "")
		m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, From: from, Payload: MembersPayload{Members: members}})

		if !volatile {
			ids := make([]string, len(members))
			for i, mi := range members {
				ids[i] = mi.PeerID
			}
			_ = m.db.UpsertGroupMembers(groupID, ids)
		}

	case TypePong:
		log.Printf("GROUP: Pong from %s in group %s", shortID(from), groupID)

	case TypeMsg, TypeState:
		// Relay to all other members and notify host's browser
		m.broadcastToGroup(hg, groupID, msgType, payload, from)
		m.notifyListeners(&Event{Type: msgType, Group: groupID, From: from, Payload: payload})
	}
}

// handleMemberMessage processes messages received by a group member from the host.
func (m *Manager) handleMemberMessage(from string, cc *clientConn, groupID, msgType string, payload any) {
	switch msgType {
	case TypeMembers:
		if rawPayload, ok := payload.(map[string]any); ok {
			if b, err := json.Marshal(rawPayload); err == nil {
				var mp MembersPayload
				if json.Unmarshal(b, &mp) == nil {
					cc.membersMu.Lock()
					cc.members = mp.Members
					cc.membersMu.Unlock()
					if !cc.volatile {
						peerIDs := make([]string, len(mp.Members))
						for i, mi := range mp.Members {
							peerIDs[i] = mi.PeerID
						}
						_ = m.db.UpsertGroupMembers(groupID, peerIDs)
					}
				}
			}
		}
		m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, From: from, Payload: payload})

	case TypeClose:
		m.mu.Lock()
		if m.activeConns[groupID] == cc {
			delete(m.activeConns, groupID)
		}
		m.mu.Unlock()
		m.db.RemoveSubscription(cc.hostPeerID, groupID) //nolint:errcheck
		m.notifyListeners(&Event{Type: TypeClose, Group: groupID})
		log.Printf("GROUP: Group %s closed by host", groupID)

	case TypePing:
		// Respond with pong
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = m.mq.Send(ctx, from, "group:"+groupID+":"+TypePong, nil)
		}()

	case TypeMeta:
		if rawPayload, ok := payload.(map[string]any); ok {
			if b, err := json.Marshal(rawPayload); err == nil {
				var mp MetaPayload
				if json.Unmarshal(b, &mp) == nil && mp.GroupName != "" {
					_ = m.db.AddSubscription(cc.hostPeerID, groupID, mp.GroupName, mp.AppType, mp.MaxMembers, cc.volatile, "member")
				}
			}
		}
		m.notifyListeners(&Event{Type: TypeMeta, Group: groupID, Payload: payload})

	case TypeMsg, TypeState, TypeError:
		m.notifyListeners(&Event{Type: msgType, Group: groupID, From: from, Payload: payload})
	}
}

// handleWelcomeForPendingJoin delivers the welcome payload to a waiting JoinRemoteGroup call.
func (m *Manager) handleWelcomeForPendingJoin(groupID string, payload any, ch chan WelcomePayload) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	var wp WelcomePayload
	if err := json.Unmarshal(b, &wp); err != nil {
		return
	}
	select {
	case ch <- wp:
	default:
	}
}

// ─── Host-side: group management ─────────────────────────────────────────────

// CreateGroup creates a new hosted group.
func (m *Manager) CreateGroup(id, name, appType string, maxMembers int, volatile bool) error {
	// Volatile game groups: close any existing hosted group of the same type
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

	ctx, cancel := context.WithCancel(context.Background())
	hg := &hostedGroup{
		info:       g,
		members:    make(map[string]*memberMeta),
		cancelPing: cancel,
	}

	m.mu.Lock()
	m.groups[id] = hg
	m.mu.Unlock()

	go m.pingGroupLoop(ctx, id)

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

	// Stop ping goroutine
	hg.mu.Lock()
	if hg.cancelPing != nil {
		hg.cancelPing()
	}
	members := hg.memberList(m.selfID)
	hg.mu.Unlock()

	// Send close to all members concurrently
	for _, mi := range members {
		if mi.PeerID == m.selfID {
			continue
		}
		pid := mi.PeerID
		go func(p string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = m.mq.Send(ctx, p, "group:"+groupID+":"+TypeClose, nil)
		}(pid)
	}

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
	_, ok := hg.members[peerID]
	if ok {
		delete(hg.members, peerID)
	}
	members := hg.memberList(m.selfID)
	volatile := hg.info.Volatile
	hg.mu.Unlock()

	if !ok {
		return fmt.Errorf("member not found: %s", peerID)
	}

	// Tell kicked member their session is over
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = m.mq.Send(ctx, peerID, "group:"+groupID+":"+TypeClose, nil)
	}()

	m.broadcastToGroup(hg, groupID, TypeMembers, MembersPayload{Members: members}, "")
	m.notifyListeners(&Event{Type: "leave", Group: groupID, From: peerID, Payload: MembersPayload{Members: members}})

	if !volatile {
		ids := make([]string, len(members))
		for i, mi := range members {
			ids[i] = mi.PeerID
		}
		_ = m.db.UpsertGroupMembers(groupID, ids)
	}

	log.Printf("GROUP: Kicked %s from %s", shortID(peerID), groupID)
	return nil
}

// SetMaxMembers updates the max_members limit for a hosted group.
func (m *Manager) SetMaxMembers(groupID string, max int) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	hg.mu.Lock()
	hg.info.MaxMembers = max
	meta := MetaPayload{GroupName: hg.info.Name, AppType: hg.info.AppType, MaxMembers: max}
	hg.mu.Unlock()

	if err := m.db.SetMaxMembers(groupID, max); err != nil {
		return fmt.Errorf("update max members: %w", err)
	}

	m.broadcastToGroup(hg, groupID, TypeMeta, meta, "")
	m.notifyListeners(&Event{Type: TypeMeta, Group: groupID, Payload: meta})

	log.Printf("GROUP: Set max members for %s to %d", groupID, max)
	return nil
}

// UpdateGroupMeta updates the name and max_members of a hosted group and broadcasts the change.
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
	m.broadcastToGroup(hg, groupID, TypeMeta, meta, "")
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
func (m *Manager) StoredGroupMembers(groupID string) []string {
	peers, _ := m.db.ListGroupMembers(groupID)
	return peers
}

// ClientGroupMembers returns the last known member list for a group we joined as a client.
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
	m.broadcastToGroup(hg, groupID, TypeMembers, MembersPayload{Members: memberList}, "")
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
	m.broadcastToGroup(hg, groupID, TypeMembers, MembersPayload{Members: memberList}, "")
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

// JoinRemoteGroup sends a join request to a remote host and waits for a welcome.
func (m *Manager) JoinRemoteGroup(ctx context.Context, hostPeerID, groupID string) error {
	// Auto-leave any existing connection to this same group (re-join scenario).
	m.mu.Lock()
	old := m.activeConns[groupID]
	if old != nil {
		delete(m.activeConns, groupID)
	}
	m.mu.Unlock()

	if old != nil {
		leaveCtx, leaveCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = m.mq.Send(leaveCtx, old.hostPeerID, "group:"+groupID+":"+TypeLeave, nil)
		leaveCancel()
		m.db.RemoveSubscription(old.hostPeerID, old.groupID) //nolint:errcheck
		_ = m.db.DeleteGroupMembers(old.groupID)
	}

	// Best-effort connect to ensure the peer is reachable
	pid, err := peer.Decode(hostPeerID)
	if err != nil {
		return fmt.Errorf("invalid host peer ID: %w", err)
	}
	_ = m.host.Connect(ctx, peer.AddrInfo{ID: pid})

	// Register pending welcome channel before sending join
	welcomeCh := make(chan WelcomePayload, 1)
	m.pendingJoinsMu.Lock()
	m.pendingJoins[groupID] = welcomeCh
	m.pendingJoinsMu.Unlock()

	defer func() {
		m.pendingJoinsMu.Lock()
		delete(m.pendingJoins, groupID)
		m.pendingJoinsMu.Unlock()
	}()

	// Set a timeout for the entire join handshake
	joinCtx, joinCancel := context.WithTimeout(ctx, 30*time.Second)
	defer joinCancel()

	// Send join
	if _, err := m.mq.Send(joinCtx, hostPeerID, "group:"+groupID+":"+TypeJoin, nil); err != nil {
		return fmt.Errorf("join send failed: %w", err)
	}

	// Wait for welcome (delivered via MQ subscription → handleWelcomeForPendingJoin)
	var wp WelcomePayload
	select {
	case wp = <-welcomeCh:
	case <-joinCtx.Done():
		return fmt.Errorf("timed out waiting for welcome from %s", shortID(hostPeerID))
	}

	cc := &clientConn{
		hostPeerID: hostPeerID,
		groupID:    groupID,
		appType:    wp.AppType,
		volatile:   wp.Volatile,
		members:    wp.Members,
	}

	m.mu.Lock()
	m.activeConns[groupID] = cc
	m.mu.Unlock()

	// Persist member list for stable groups
	if !wp.Volatile && len(wp.Members) > 0 {
		peerIDs := make([]string, len(wp.Members))
		for i, mi := range wp.Members {
			peerIDs[i] = mi.PeerID
		}
		_ = m.db.UpsertGroupMembers(groupID, peerIDs)
	}

	// Volatile game groups: wipe stale subscriptions of the same type
	if wp.Volatile {
		if subs, err := m.db.ListSubscriptions(); err == nil {
			for _, s := range subs {
				if s.AppType == wp.AppType && s.GroupID != groupID {
					_ = m.db.RemoveSubscription(s.HostPeerID, s.GroupID)
					_ = m.db.DeleteGroupMembers(s.GroupID)
				}
			}
		}
	}

	// Store subscription with full metadata
	m.db.AddSubscription(hostPeerID, groupID, wp.GroupName, wp.AppType, wp.MaxMembers, wp.Volatile, "member") //nolint:errcheck

	m.notifyListeners(&Event{Type: TypeWelcome, Group: groupID, From: hostPeerID, Payload: map[string]any{
		"group_name":  wp.GroupName,
		"app_type":    wp.AppType,
		"max_members": wp.MaxMembers,
		"volatile":    wp.Volatile,
		"members":     wp.Members,
	}})

	log.Printf("GROUP: Joined group %s on host %s", groupID, shortID(hostPeerID))
	return nil
}

// SendToGroup sends a message through the client connection for the given group.
func (m *Manager) SendToGroup(groupID string, payload any) error {
	m.mu.RLock()
	cc := m.activeConns[groupID]
	m.mu.RUnlock()

	if cc == nil {
		return fmt.Errorf("not connected to group %s", groupID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := m.mq.Send(ctx, cc.hostPeerID, "group:"+groupID+":"+TypeMsg, payload)
	return err
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = m.mq.Send(ctx, cc.hostPeerID, "group:"+groupID+":"+TypeLeave, nil)

	m.db.RemoveSubscription(cc.hostPeerID, cc.groupID) //nolint:errcheck
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

// ─── MQ broadcast helpers ─────────────────────────────────────────────────────

// broadcastToGroup sends a message to all members of a hosted group except excludePeerID.
// Send failures are treated as disconnections: the failing member is removed.
func (m *Manager) broadcastToGroup(hg *hostedGroup, groupID, msgType string, payload any, excludePeerID string) {
	hg.mu.RLock()
	members := hg.memberList(m.selfID)
	hg.mu.RUnlock()

	for _, mi := range members {
		if mi.PeerID == m.selfID || mi.PeerID == excludePeerID {
			continue
		}
		pid := mi.PeerID
		go func(p string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := m.mq.Send(ctx, p, "group:"+groupID+":"+msgType, payload); err != nil {
				log.Printf("GROUP: MQ send to %s failed: %v, removing from group", shortID(p), err)
				m.removeMemberAndBroadcast(groupID, p)
			}
		}(pid)
	}
}

// removeMemberAndBroadcast removes a peer from a hosted group and broadcasts the updated member list.
func (m *Manager) removeMemberAndBroadcast(groupID, peerID string) {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()
	if !exists {
		return
	}

	hg.mu.Lock()
	_, wasMember := hg.members[peerID]
	if wasMember {
		delete(hg.members, peerID)
	}
	members := hg.memberList(m.selfID)
	volatile := hg.info.Volatile
	hg.mu.Unlock()

	if !wasMember {
		return
	}

	m.broadcastToGroup(hg, groupID, TypeMembers, MembersPayload{Members: members}, "")
	m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, From: peerID, Payload: MembersPayload{Members: members}})

	if !volatile {
		ids := make([]string, len(members))
		for i, mi := range members {
			ids[i] = mi.PeerID
		}
		_ = m.db.UpsertGroupMembers(groupID, ids)
	}
}

// ─── Heartbeat (host sends pings to all members) ──────────────────────────────

func (m *Manager) pingGroupLoop(ctx context.Context, groupID string) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.RLock()
			hg, exists := m.groups[groupID]
			m.mu.RUnlock()
			if !exists {
				return
			}

			hg.mu.RLock()
			members := hg.memberList(m.selfID)
			hg.mu.RUnlock()

			for _, mi := range members {
				if mi.PeerID == m.selfID {
					continue
				}
				pid := mi.PeerID
				go func(p string) {
					sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					if _, err := m.mq.Send(sendCtx, p, "group:"+groupID+":"+TypePing, nil); err != nil {
						log.Printf("GROUP: Ping to %s failed: %v, removing from group", shortID(p), err)
						m.removeMemberAndBroadcast(groupID, p)
					}
				}(pid)
			}
		}
	}
}

// ─── Browser notification (replaces private SSE) ─────────────────────────────

// notifyListeners publishes the event to the browser via MQ PublishLocal
// and dispatches to any registered type-specific Go handler.
func (m *Manager) notifyListeners(evt *Event) {
	// Push to browser via MQ SSE
	m.mq.PublishLocal("group:"+evt.Group+":"+evt.Type, "", evt)

	// Dispatch to the registered type-specific handler (async to avoid blocking caller).
	m.mu.RLock()
	appType := m.appTypeForGroupLocked(evt.Group)
	h := m.handlers[appType]
	m.mu.RUnlock()
	if h != nil {
		go h.HandleGroupEvent(evt)
	}
}

// RegisterHandler registers h to receive group events whose group app_type matches appType.
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

// ─── Invitations ─────────────────────────────────────────────────────────────

// inviteMsg is the wire format for a group invitation.
type inviteMsg struct {
	GroupID    string `json:"group_id"`
	GroupName  string `json:"group_name"`
	HostPeerID string `json:"host_peer_id"`
	AppType    string `json:"app_type"`
	Volatile   bool   `json:"volatile"`
}

// InvitePeer sends a group invitation to a remote peer via MQ.
func (m *Manager) InvitePeer(ctx context.Context, peerID, groupID string) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	hg.mu.RLock()
	inv := inviteMsg{
		GroupID:    groupID,
		GroupName:  hg.info.Name,
		HostPeerID: m.selfID,
		AppType:    hg.info.AppType,
		Volatile:   hg.info.Volatile,
	}
	hg.mu.RUnlock()

	_, err := m.mq.Send(ctx, peerID, "group.invite", inv)
	if err != nil {
		return fmt.Errorf("invite send failed: %w", err)
	}

	log.Printf("GROUP: Sent invite for group %s to peer %s", groupID, shortID(peerID))
	return nil
}

// handleInvite processes an incoming group invitation received via MQ.
func (m *Manager) handleInvite(from string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		log.Printf("GROUP: Failed to marshal invite payload: %v", err)
		return
	}
	var inv inviteMsg
	if err := json.Unmarshal(b, &inv); err != nil || inv.GroupID == "" {
		log.Printf("GROUP: Failed to decode invite: %v", err)
		return
	}
	// Prefer the actual sender's peer ID over the HostPeerID in the payload
	if from != "" {
		inv.HostPeerID = from
	}

	log.Printf("GROUP: Received invite for group %s from host %s", inv.GroupID, shortID(inv.HostPeerID))

	// Volatile game groups: wipe stale subscriptions of the same type
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
	_ = m.db.AddSubscription(inv.HostPeerID, inv.GroupID, inv.GroupName, inv.AppType, 0, inv.Volatile, "member")

	// Notify browser via MQ PublishLocal on the "group.invite" topic
	// (same topic JS subscribers use — no groupID scoping for invites)
	evt := &Event{
		Type:  "invite",
		Group: inv.GroupID,
		From:  inv.HostPeerID,
		Payload: map[string]any{
			"group_id":   inv.GroupID,
			"group_name": inv.GroupName,
			"host":       inv.HostPeerID,
			"app_type":   inv.AppType,
		},
	}
	m.mq.PublishLocal("group.invite", "", evt)

	// Auto-join for app types that require it
	if inv.AppType == "realtime" || inv.AppType == "template" {
		go func() {
			if err := m.JoinRemoteGroup(context.Background(), inv.HostPeerID, inv.GroupID); err != nil {
				log.Printf("GROUP: Auto-join %s group %s failed: %v", inv.AppType, inv.GroupID, err)
			}
		}()
	}
}

// ─── Subscriptions (client-side persistence) ─────────────────────────────────

// ListSubscriptions returns all stored group subscriptions.
func (m *Manager) ListSubscriptions() ([]storage.SubscriptionRow, error) {
	return m.db.ListSubscriptions()
}

// RejoinSubscription attempts to reconnect to a previously subscribed group.
func (m *Manager) RejoinSubscription(ctx context.Context, hostPeerID, groupID string) error {
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
func (m *Manager) reconnectSubscriptions() {
	// Wait for mDNS / rendezvous peer discovery
	time.Sleep(6 * time.Second)

	subs, err := m.db.ListSubscriptions()
	if err != nil || len(subs) == 0 {
		return
	}

	for _, sub := range subs {
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
			msg := err.Error()
			if i := strings.Index(msg, "\n"); i > 0 {
				msg = msg[:i]
			}
			log.Printf("GROUP: Auto-reconnect to %s failed: %s", sub.GroupID, msg)
		} else {
			log.Printf("GROUP: Auto-reconnected to group %s on host %s", sub.GroupID, shortID(sub.HostPeerID))
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
	for _, mm := range g.members {
		members = append(members, MemberInfo{
			PeerID:   mm.peerID,
			JoinedAt: mm.joinedAt,
		})
	}
	return members
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

	m.broadcastToGroup(hg, groupID, TypeMsg, payload, "")
	m.notifyListeners(&Event{Type: TypeMsg, Group: groupID, From: m.selfID, Payload: payload})
	return nil
}

// Close shuts down the group manager and unregisters MQ subscriptions.
func (m *Manager) Close() error {
	m.mu.Lock()

	// Stop all ping goroutines
	for _, hg := range m.groups {
		hg.mu.Lock()
		if hg.cancelPing != nil {
			hg.cancelPing()
		}
		hg.mu.Unlock()
	}

	m.mu.Unlock()

	// Unregister MQ subscriptions
	if m.unsubGroup != nil {
		m.unsubGroup()
	}
	if m.unsubInvite != nil {
		m.unsubInvite()
	}

	return nil
}

// SelfID returns the local peer ID.
func (m *Manager) SelfID() string {
	return m.selfID
}

// IsPeerInGroup returns true if the given peer is a current member of a hosted group.
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

// IsTemplateMember returns true if peerID is an active member of any hosted group with app_type "template".
func (m *Manager) IsTemplateMember(peerID string) bool {
	m.mu.RLock()
	var templateGroupIDs []string
	for gid, hg := range m.groups {
		if hg.info.AppType == "template" {
			templateGroupIDs = append(templateGroupIDs, gid)
		}
	}
	m.mu.RUnlock()
	for _, gid := range templateGroupIDs {
		if m.IsPeerInGroup(peerID, gid) {
			return true
		}
	}
	return false
}

// IsGroupHost returns true if this peer hosts the given group.
func (m *Manager) IsGroupHost(groupID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.groups[groupID]
	return exists
}

// IsKnownGroupPeer returns true if remotePeer is a verified member of groupID.
func (m *Manager) IsKnownGroupPeer(remotePeer, groupID string) bool {
	m.mu.RLock()
	_, isHost := m.groups[groupID]
	cc := m.activeConns[groupID]
	m.mu.RUnlock()

	if isHost {
		return m.IsPeerInGroup(remotePeer, groupID)
	}

	if cc != nil {
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
