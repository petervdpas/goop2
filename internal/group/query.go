package group

import "github.com/petervdpas/goop2/internal/storage"

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
