package group

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/petervdpas/goop2/internal/mq"
	"github.com/petervdpas/goop2/internal/storage"

	"github.com/libp2p/go-libp2p/core/host"
)

// Event is emitted to local MQ listeners (via PublishLocal).
type Event struct {
	Type    string `json:"type"`
	Group   string `json:"group"`
	From    string `json:"from,omitempty"`
	Payload any    `json:"payload,omitempty"`
}


// ActiveGroupInfo holds info about one active client-side group connection.
type ActiveGroupInfo struct {
	HostPeerID string `json:"host_peer_id"`
	GroupID    string `json:"group_id"`
	GroupType    string `json:"group_type"`
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

	// Pending join channels: groupID -> channel waiting for welcome or error
	pendingJoinsMu sync.Mutex
	pendingJoins   map[string]chan joinResult

	// Type-specific lifecycle handlers keyed by group_type.
	handlers map[string]TypeHandler

	// MQ unsubscribe functions
	unsubGroup  func()
	unsubInvite func()
	unsubPeer   func()
}

type memberMeta struct {
	peerID   string
	role     string
	joinedAt int64
}

type joinResult struct {
	welcome WelcomePayload
	err     error
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
	groupType    string
	volatile   bool
	membersMu  sync.RWMutex
	members    []MemberInfo // last known member list from host
}

const (
	pingInterval    = PingInterval
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
		pendingJoins: make(map[string]chan joinResult),
		handlers:     make(map[string]TypeHandler),
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
		rest := topic[len("group:"):]
		if before, after, ok := strings.Cut(rest, ":"); ok {
			m.handleMQMessage(from, before, after, payload)
		}
	})

	m.unsubInvite = mqMgr.SubscribeTopic("group.invite", func(from, topic string, payload any) {
		m.handleInvite(from, payload)
	})

	// Watch for peers coming online — auto-rejoin disconnected subscriptions
	m.unsubPeer = mqMgr.SubscribeTopic("peer:announce", func(_, _ string, payload any) {
		m.handlePeerAnnounce(payload)
	})

	log.Printf("GROUP: MQ transport registered (group: + group.invite + peer:announce)")

	// Auto-reconnect to subscribed groups in the background
	go m.reconnectSubscriptions()

	return m
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
	if m.unsubPeer != nil {
		m.unsubPeer()
	}

	return nil
}

// SelfID returns the local peer ID.
func (m *Manager) SelfID() string {
	return m.selfID
}

// RegisterType registers a TypeHandler for the given group_type.
func (m *Manager) RegisterType(groupType string, h TypeHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[groupType] = h
}

// RegisteredTypes returns the list of registered group type names.
func (m *Manager) RegisteredTypes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	types := make([]string, 0, len(m.handlers))
	for t := range m.handlers {
		types = append(types, t)
	}
	return types
}

func (m *Manager) notifyListeners(evt *Event) {
	if m.mq != nil {
		m.mq.PublishLocal("group:"+evt.Group+":"+evt.Type, "", evt)
	}

	m.mu.RLock()
	groupType := m.groupTypeForGroupLocked(evt.Group)
	h := m.handlers[groupType]
	m.mu.RUnlock()
	if h != nil {
		go h.OnEvent(evt)
	}
}

func (m *Manager) groupTypeForGroupLocked(groupID string) string {
	if hg, ok := m.groups[groupID]; ok {
		return hg.info.GroupType
	}
	if cc, ok := m.activeConns[groupID]; ok {
		return cc.groupType
	}
	return ""
}

// membersToStorage converts MemberInfo slice to storage GroupMember slice.
func membersToStorage(members []MemberInfo) []storage.GroupMember {
	gm := make([]storage.GroupMember, len(members))
	for i, mi := range members {
		gm[i] = storage.GroupMember{PeerID: mi.PeerID, Role: mi.Role}
	}
	return gm
}

// resolveMemberNames enriches a MemberInfo slice with peer display names.
func (m *Manager) resolveMemberNames(members []MemberInfo) {
	for i := range members {
		if members[i].PeerID == m.selfID {
			members[i].Name = m.db.GetPeerName(m.selfID)
		} else {
			members[i].Name = m.db.GetPeerName(members[i].PeerID)
		}
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func (g *hostedGroup) memberList(hostID string) []MemberInfo {
	members := make([]MemberInfo, 0, len(g.members)+1)
	if g.hostJoined {
		members = append(members, MemberInfo{
			PeerID:   hostID,
			Role:     "owner",
			JoinedAt: g.hostJoinedAt,
		})
	}
	for _, mm := range g.members {
		members = append(members, MemberInfo{
			PeerID:   mm.peerID,
			Role:     mm.role,
			JoinedAt: mm.joinedAt,
		})
	}
	return members
}
