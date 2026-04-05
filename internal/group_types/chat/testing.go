package chat

import (
	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/state"
)

func NewTestManager(grpMgr *group.Manager, selfID string, resolvePeer func(string) state.PeerIdentityPayload) *Manager {
	m := &Manager{
		grp:         grpMgr,
		selfID:      selfID,
		resolvePeer: resolvePeer,
		rooms:       make(map[string]*roomState),
	}
	grpMgr.RegisterType(GroupTypeName, m)
	return m
}

// RegisterJoinedRoom creates a room entry for a group the joiner has joined
// remotely. In production this should happen inside JoinRoom; this helper
// exposes the gap so BDD tests can exercise the joiner path.
func (m *Manager) RegisterJoinedRoom(groupID, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.rooms[groupID]; !exists {
		m.rooms[groupID] = &roomState{
			info:    Room{ID: groupID, Name: name},
			history: &RingBuffer{},
		}
	}
}
