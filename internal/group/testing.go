package group

import (
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
)

// NewTestManager creates a minimal Manager backed only by a DB,
// suitable for unit tests that don't need P2P or MQ transport.
func NewTestManager(db *storage.DB, selfID string, resolvePeer ...func(string) state.PeerIdentity) *Manager {
	m := &Manager{
		db:           db,
		selfID:       selfID,
		groups:       make(map[string]*hostedGroup),
		activeConns:  make(map[string]*clientConn),
		pendingJoins: make(map[string]chan joinResult),
		handlers:     make(map[string]TypeHandler),
	}
	if len(resolvePeer) > 0 {
		m.resolvePeer = resolvePeer[0]
	}
	return m
}

// SimulateInvite processes an invite payload as if it arrived via MQ.
func (m *Manager) SimulateInvite(from string, payload any) {
	m.handleInvite(from, payload)
}

// SimulateJoin processes a join as if a remote peer sent TypeJoin via MQ.
func (m *Manager) SimulateJoin(peerID, groupID string) {
	m.mu.RLock()
	hg := m.groups[groupID]
	m.mu.RUnlock()
	if hg != nil {
		m.handleHostMessage(peerID, hg, groupID, TypeJoin, nil)
	}
}

// SimulateLeave processes a leave as if a remote peer sent TypeLeave via MQ.
func (m *Manager) SimulateLeave(peerID, groupID string) {
	m.mu.RLock()
	hg := m.groups[groupID]
	m.mu.RUnlock()
	if hg != nil {
		m.handleHostMessage(peerID, hg, groupID, TypeLeave, nil)
	}
}

// SimulateHostClose processes a close as if the host sent TypeClose to a client.
func (m *Manager) SimulateHostClose(groupID string) {
	m.mu.RLock()
	cc := m.activeConns[groupID]
	m.mu.RUnlock()
	if cc != nil {
		m.handleMemberMessage(cc.hostPeerID, cc, groupID, TypeClose, nil)
	}
}

// SetActiveConn sets up a fake client connection for testing.
func (m *Manager) SetActiveConn(groupID, hostPeerID, groupType string) {
	m.mu.Lock()
	m.activeConns[groupID] = &clientConn{
		hostPeerID: hostPeerID,
		groupID:    groupID,
		groupType:  groupType,
	}
	m.mu.Unlock()
}

// SetActiveConnMembers sets the member list on a fake client connection.
func (m *Manager) SetActiveConnMembers(groupID string, members []MemberInfo) {
	m.mu.RLock()
	cc := m.activeConns[groupID]
	m.mu.RUnlock()
	if cc != nil {
		cc.membersMu.Lock()
		cc.members = members
		cc.membersMu.Unlock()
	}
}
