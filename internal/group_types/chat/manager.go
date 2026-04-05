package chat

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/mq"
	"github.com/petervdpas/goop2/internal/state"
)

const GroupTypeName = "chat"

// Manager manages chat rooms backed by groups.
type Manager struct {
	grp         *group.Manager
	mq          mq.Transport
	selfID      string
	resolvePeer func(string) state.PeerIdentityPayload

	mu    sync.RWMutex
	rooms map[string]*roomState

	unsubMQ func()
}

type roomState struct {
	mu      sync.RWMutex
	info    Room
	history *RingBuffer
}

// New creates a chat manager and registers the group type handler.
func New(grpMgr *group.Manager, transport mq.Transport, selfID string, resolvePeer func(string) state.PeerIdentityPayload) *Manager {
	m := &Manager{
		grp:         grpMgr,
		mq:          transport,
		selfID:      selfID,
		resolvePeer: resolvePeer,
		rooms:       make(map[string]*roomState),
	}

	grpMgr.RegisterType(GroupTypeName, m)

	m.unsubMQ = transport.SubscribeTopic(topicPrefix, func(from, t string, payload any) {
		m.handleIncoming(from, t, payload)
	})

	return m
}

// Close shuts down the chat manager.
func (m *Manager) Close() {
	if m.unsubMQ != nil {
		m.unsubMQ()
	}
}

// CreateRoom creates a new chat room backed by a hosted group.
func (m *Manager) CreateRoom(name, description, context string, maxMembers int) (*Room, error) {
	id := fmt.Sprintf("%x", time.Now().UnixNano())
	if err := m.grp.CreateGroup(id, name, GroupTypeName, context, maxMembers); err != nil {
		return nil, err
	}
	if err := m.grp.JoinOwnGroup(id); err != nil {
		log.Printf("CHAT: auto-join own room failed: %v", err)
	}

	m.mu.RLock()
	rs := m.rooms[id]
	m.mu.RUnlock()
	if rs != nil {
		rs.mu.Lock()
		rs.info.Description = description
		rs.mu.Unlock()
	}

	return &Room{ID: id, Name: name, Description: description}, nil
}

// SelfID returns the local peer ID.
func (m *Manager) SelfID() string {
	return m.selfID
}

// CloseRoom closes a chat room.
func (m *Manager) CloseRoom(groupID string) error {
	return m.grp.CloseGroup(groupID)
}

// CloseByContext closes all chat rooms whose group context matches the given name.
// Called during template switch to clean up rooms owned by the outgoing template.
func (m *Manager) CloseByContext(context string) {
	groups, err := m.grp.ListHostedGroups()
	if err != nil {
		return
	}
	for _, g := range groups {
		if g.GroupType == GroupTypeName && g.GroupContext == context {
			_ = m.grp.CloseGroup(g.ID)
			log.Printf("CHAT: closed room %s (context=%s)", g.ID, g.GroupContext)
		}
	}
}

// JoinRoom joins a remote chat room.
func (m *Manager) JoinRoom(ctx context.Context, hostPeerID, groupID string) error {
	if err := m.grp.JoinRemoteGroup(ctx, hostPeerID, groupID); err != nil {
		return err
	}
	name := groupID
	if subs, err := m.grp.ListSubscriptions(); err == nil {
		for _, s := range subs {
			if s.GroupID == groupID {
				name = s.GroupName
				break
			}
		}
	}
	m.mu.Lock()
	if _, exists := m.rooms[groupID]; !exists {
		m.rooms[groupID] = &roomState{
			info:    Room{ID: groupID, Name: name},
			history: &RingBuffer{},
		}
	}
	m.mu.Unlock()
	return nil
}

// LeaveRoom leaves a remote chat room.
func (m *Manager) LeaveRoom(groupID string) error {
	return m.grp.LeaveGroup(groupID)
}

// SendMessage sends a chat message to all members of a room.
func (m *Manager) SendMessage(groupID, fromPeerID, text string) error {
	m.mu.RLock()
	rs, exists := m.rooms[groupID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("room not found: %s", groupID)
	}

	msg := Message{
		ID:        fmt.Sprintf("%d-%s", time.Now().UnixMilli(), fromPeerID[:8]),
		From:      fromPeerID,
		FromName:  m.resolvePeer(fromPeerID).Name(),
		Text:      text,
		Timestamp: time.Now().UnixMilli(),
	}

	rs.mu.Lock()
	rs.history.Add(msg)
	rs.mu.Unlock()

	cm := chatMsg{Action: subtopicMsg, Message: &msg}
	m.broadcastToRoom(groupID, subtopicMsg, cm, "")
	return nil
}

// GetState returns the current state of a room including members and recent messages.
func (m *Manager) GetState(groupID string) (*Room, []Message, error) {
	m.mu.RLock()
	rs, exists := m.rooms[groupID]
	m.mu.RUnlock()
	if !exists {
		return nil, nil, fmt.Errorf("room not found: %s", groupID)
	}

	rs.mu.RLock()
	room := rs.info
	room.Members = m.resolveMembers(groupID)
	msgs := rs.history.All()
	rs.mu.RUnlock()

	return &room, msgs, nil
}

// ListRooms returns all active rooms this peer is hosting or has joined.
func (m *Manager) ListRooms() []Room {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rooms := make([]Room, 0, len(m.rooms))
	for _, rs := range m.rooms {
		rs.mu.RLock()
		r := rs.info
		r.Members = m.resolveMembers(r.ID)
		rs.mu.RUnlock()
		rooms = append(rooms, r)
	}
	return rooms
}

func (m *Manager) resolveMembers(groupID string) []Member {
	members := m.grp.HostedGroupMembers(groupID)
	if len(members) == 0 {
		members = m.grp.ClientGroupMembers(groupID)
	}
	out := make([]Member, len(members))
	for i, mi := range members {
		name := mi.Name
		if name == "" {
			name = m.resolvePeer(mi.PeerID).Name()
		}
		out[i] = Member{
			PeerID: mi.PeerID,
			Name:   name,
		}
	}
	return out
}
