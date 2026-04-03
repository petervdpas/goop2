package chat

import (
	"log"

	"github.com/petervdpas/goop2/internal/group"
)

// TypeHandler interface implementation on Manager.

func (m *Manager) Flags() group.TypeFlags {
	return group.TypeFlags{HostCanJoin: true}
}

func (m *Manager) OnCreate(groupID, name string, _ int, _ bool) error {
	m.mu.Lock()
	m.rooms[groupID] = &roomState{
		info: Room{
			ID:   groupID,
			Name: name,
		},
		history: &RingBuffer{},
	}
	m.mu.Unlock()
	log.Printf("CHAT: Room %s created (%s)", groupID, name)
	return nil
}

func (m *Manager) OnJoin(groupID, peerID string, isHost bool) {
	m.mu.RLock()
	rs, exists := m.rooms[groupID]
	m.mu.RUnlock()
	if !exists {
		return
	}

	if !isHost {
		rs.mu.RLock()
		msgs := rs.history.All()
		rs.mu.RUnlock()

		if len(msgs) > 0 {
			m.sendToPeer(peerID, groupID, subtopicHistory, chatMsg{
				Action:   subtopicHistory,
				Messages: msgs,
			})
		}

		log.Printf("CHAT: %s joined room %s", peerID[:8], groupID)
	}

	members := m.resolveMembers(groupID)
	m.broadcastToRoom(groupID, subtopicMembers, chatMsg{
		Action:  subtopicMembers,
		Members: members,
	}, "")
}

func (m *Manager) OnLeave(groupID, peerID string, isHost bool) {
	if !isHost {
		log.Printf("CHAT: %s left room %s", peerID[:8], groupID)
	}

	m.mu.RLock()
	_, exists := m.rooms[groupID]
	m.mu.RUnlock()
	if !exists {
		return
	}

	members := m.resolveMembers(groupID)
	m.broadcastToRoom(groupID, subtopicMembers, chatMsg{
		Action:  subtopicMembers,
		Members: members,
	}, "")
}

func (m *Manager) OnClose(groupID string) {
	m.mu.Lock()
	delete(m.rooms, groupID)
	m.mu.Unlock()
	log.Printf("CHAT: Room %s closed", groupID)
}

func (m *Manager) OnEvent(_ *group.Event) {}
