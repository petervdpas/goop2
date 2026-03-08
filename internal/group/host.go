package group

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/petervdpas/goop2/internal/storage"
)

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

	hg.mu.Lock()
	if hg.cancelPing != nil {
		hg.cancelPing()
	}
	members := hg.memberList(m.selfID)
	hg.mu.Unlock()

	for _, mi := range members {
		if mi.PeerID == m.selfID {
			continue
		}
		pid := mi.PeerID
		go func(p string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = m.mq.Send(ctx, p, "group:"+groupID+":"+TypeClose, Message{Type: TypeClose, Group: groupID})
		}(pid)
	}

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

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = m.mq.Send(ctx, peerID, "group:"+groupID+":"+TypeClose, Message{Type: TypeClose, Group: groupID})
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
	var newMax int
	if hg.info.MaxMembers > 0 && len(hg.members)+1 > hg.info.MaxMembers {
		hg.info.MaxMembers++
		newMax = hg.info.MaxMembers
	}
	hg.hostJoined = true
	hg.hostJoinedAt = nowMillis()
	memberList := hg.memberList(m.selfID)
	var meta MetaPayload
	if newMax > 0 {
		meta = MetaPayload{GroupName: hg.info.Name, AppType: hg.info.AppType, MaxMembers: newMax}
	}
	hg.mu.Unlock()

	_ = m.db.SetHostJoined(groupID, true)
	if newMax > 0 {
		_ = m.db.SetMaxMembers(groupID, newMax)
		m.broadcastToGroup(hg, groupID, TypeMeta, meta, "")
		m.notifyListeners(&Event{Type: TypeMeta, Group: groupID, Payload: meta})
		log.Printf("GROUP: Host joining full group %s, max_members bumped to %d", groupID, newMax)
	}

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

// broadcastToGroup sends a message to all members of a hosted group except excludePeerID.
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
					if _, err := m.mq.Send(sendCtx, p, "group:"+groupID+":"+TypePing, Message{Type: TypePing, Group: groupID}); err != nil {
						log.Printf("GROUP: Ping to %s failed: %v, removing from group", shortID(p), err)
						m.removeMemberAndBroadcast(groupID, p)
					}
				}(pid)
			}
		}
	}
}
