package group

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/petervdpas/goop2/internal/storage"
)

// CreateGroup creates a new hosted group. Source links the group to its creator
// (e.g. the template name) for cleanup on re-apply.
func (m *Manager) CreateGroup(id, name, groupType, groupContext string, maxMembers int) error {
	volatile := false
	if h := m.handlerForType(groupType); h != nil {
		volatile = h.Flags().Volatile
	}

	// Volatile types: close any existing hosted group of the same type
	if volatile {
		m.mu.RLock()
		var toClose []string
		for gid, hg := range m.groups {
			hg.mu.RLock()
			if hg.info.GroupType == groupType {
				toClose = append(toClose, gid)
			}
			hg.mu.RUnlock()
		}
		m.mu.RUnlock()
		for _, gid := range toClose {
			_ = m.CloseGroup(gid)
		}
	}

	// Enforce hosted group limit — volatile types are excluded from the cap
	if !volatile {
		m.mu.RLock()
		stableCount := 0
		for _, hg := range m.groups {
			hg.mu.RLock()
			if !m.isVolatileType(hg.info.GroupType) {
				stableCount++
			}
			hg.mu.RUnlock()
		}
		m.mu.RUnlock()
		if stableCount >= maxHostedGroups {
			return fmt.Errorf("maximum of %d hosted groups reached", maxHostedGroups)
		}
	}

	if err := m.db.CreateGroup(id, name, m.selfID, groupType, groupContext, maxMembers, volatile); err != nil {
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

	if m.mq != nil {
		go m.pingGroupLoop(ctx, id)
	}

	log.Printf("GROUP: Created group %s (%s)", id, name)

	if h := m.handlerForType(groupType); h != nil {
		if err := h.OnCreate(id, name, maxMembers); err != nil {
			_ = m.CloseGroup(id)
			return err
		}
	}

	return nil
}

// RestoreGroup re-registers a persisted group into the in-memory state.
// Used on startup to restore groups that were active before a restart.
func (m *Manager) RestoreGroup(groupID string) error {
	g, err := m.db.GetGroup(groupID)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if _, exists := m.groups[groupID]; exists {
		m.mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	hg := &hostedGroup{
		info:       g,
		members:    make(map[string]*memberMeta),
		cancelPing: cancel,
	}
	m.groups[groupID] = hg
	m.mu.Unlock()

	go m.pingGroupLoop(ctx, groupID)

	log.Printf("GROUP: Restored group %s (%s)", groupID, g.Name)
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
	groupType := hg.info.GroupType
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
			ctx, cancel := context.WithTimeout(context.Background(), SendTimeout)
			defer cancel()
			_, _ = m.mq.Send(ctx, p, "group:"+groupID+":"+TypeClose, Message{Type: TypeClose, Group: groupID})
		}(pid)
	}

	if err := m.db.DeleteGroup(groupID); err != nil {
		log.Printf("GROUP: Failed to delete group %s from DB: %v", groupID, err)
	}
	_ = m.db.DeleteGroupMembers(groupID)

	m.notifyListeners(&Event{Type: TypeClose, Group: groupID})

	if h := m.handlerForType(groupType); h != nil {
		h.OnClose(groupID)
	}

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
	groupType := hg.info.GroupType
	hg.mu.Unlock()

	if !ok {
		return fmt.Errorf("member not found: %s", peerID)
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), SendTimeout)
		defer cancel()
		_, _ = m.mq.Send(ctx, peerID, "group:"+groupID+":"+TypeClose, Message{Type: TypeClose, Group: groupID})
	}()

	m.broadcastToGroup(hg, groupID, TypeMembers, MembersPayload{Members: members}, "")
	m.notifyListeners(&Event{Type: "leave", Group: groupID, From: peerID, Payload: MembersPayload{Members: members}})

	if !m.isVolatileType(groupType) {
		_ = m.db.UpsertGroupMembers(groupID, membersToStorage(members))
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
	meta := MetaPayload{GroupName: hg.info.Name, GroupType: hg.info.GroupType, MaxMembers: max}
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
	groupType := hg.info.GroupType
	hg.mu.Unlock()

	if err := m.db.UpdateGroup(groupID, name, maxMembers); err != nil {
		return fmt.Errorf("update group: %w", err)
	}

	meta := MetaPayload{GroupName: name, GroupType: groupType, MaxMembers: maxMembers}
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
	members := hg.memberList(m.selfID)
	hg.mu.RUnlock()
	m.resolveMemberNames(members)
	return members
}

// SetDefaultRole updates the default role assigned to new members of a hosted group.
func (m *Manager) SetDefaultRole(groupID, role string) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	if err := m.db.SetDefaultRole(groupID, role); err != nil {
		return err
	}

	hg.mu.Lock()
	hg.info.DefaultRole = role
	hg.mu.Unlock()
	return nil
}

// SetGroupRoles updates the available roles for a hosted group.
func (m *Manager) SetGroupRoles(groupID string, roles []string) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	if err := m.db.SetGroupRoles(groupID, roles); err != nil {
		return err
	}

	hg.mu.Lock()
	hg.info.Roles = roles
	hg.mu.Unlock()
	return nil
}

// SetMemberRole updates a member's role in a hosted group, persists it, and broadcasts the change.
func (m *Manager) SetMemberRole(groupID, peerID, role string) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	hg.mu.Lock()
	mm, ok := hg.members[peerID]
	if !ok {
		hg.mu.Unlock()
		return fmt.Errorf("peer %s not in group %s", peerID, groupID)
	}
	mm.role = role
	members := hg.memberList(m.selfID)
	groupType := hg.info.GroupType
	hg.mu.Unlock()

	if !m.isVolatileType(groupType) {
		_ = m.db.SetMemberRole(groupID, peerID, role)
		_ = m.db.UpsertGroupMembers(groupID, membersToStorage(members))
	}

	m.broadcastToGroup(hg, groupID, TypeMembers, MembersPayload{Members: members}, "")
	m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, Payload: MembersPayload{Members: members}})

	return nil
}

// StoredGroupMembers returns the persisted member peer IDs for a group.
func (m *Manager) StoredGroupMembers(groupID string) []storage.GroupMember {
	members, _ := m.db.ListGroupMembers(groupID)
	return members
}

// JoinOwnGroup adds the host as a member of their own hosted group.
func (m *Manager) JoinOwnGroup(groupID string) error {
	m.mu.RLock()
	hg, exists := m.groups[groupID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("group not found: %s", groupID)
	}

	if h := m.handlerForGroup(groupID); h != nil {
		if !h.Flags().HostCanJoin {
			return fmt.Errorf("group type does not allow host to join")
		}
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
		meta = MetaPayload{GroupName: hg.info.Name, GroupType: hg.info.GroupType, MaxMembers: newMax}
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

	if h := m.handlerForGroup(groupID); h != nil {
		h.OnJoin(groupID, m.selfID, true)
	}

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
	if len(hg.members) > 0 {
		hg.mu.Unlock()
		return fmt.Errorf("cannot leave: %d members still in group", len(hg.members))
	}
	hg.hostJoined = false
	hg.hostJoinedAt = 0
	memberList := hg.memberList(m.selfID)
	hg.mu.Unlock()

	_ = m.db.SetHostJoined(groupID, false)

	m.broadcastToGroup(hg, groupID, TypeMembers, MembersPayload{Members: memberList}, "")
	m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, Payload: MembersPayload{Members: memberList}})

	if h := m.handlerForGroup(groupID); h != nil {
		h.OnLeave(groupID, m.selfID, true)
	}

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
			ctx, cancel := context.WithTimeout(context.Background(), BroadcastTimeout)
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
	groupType := hg.info.GroupType
	hg.mu.Unlock()

	if !wasMember {
		return
	}

	m.broadcastToGroup(hg, groupID, TypeMembers, MembersPayload{Members: members}, "")
	m.notifyListeners(&Event{Type: TypeMembers, Group: groupID, From: peerID, Payload: MembersPayload{Members: members}})

	if !m.isVolatileType(groupType) {
		_ = m.db.UpsertGroupMembers(groupID, membersToStorage(members))
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
					sendCtx, cancel := context.WithTimeout(context.Background(), BroadcastTimeout)
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
