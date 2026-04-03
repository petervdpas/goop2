package group

// GroupTypeFlags declares per-type rules for a group group_type.
type GroupTypeFlags struct {
	HostCanJoin bool // whether the host can join their own group as a member
	Volatile    bool // ephemeral: no member persistence, excluded from group cap
}

// TypeHandler defines lifecycle hooks for a group group_type.
// Each group type (listen, cluster, files, etc.) can register a handler
// to receive lifecycle callbacks when groups of that type are created,
// joined, left, closed, or receive events.
//
// All hooks are called on the HOST side:
//   - OnCreate: a new group was created
//   - OnJoin: a member joined the group (isHost=true when the host joins their own group)
//   - OnLeave: a member left the group (isHost=true when the host leaves their own group)
//   - OnClose: the group was closed
//   - OnEvent: any group event (msg, members, meta, etc.)
//
// All hooks are called outside the group manager's locks, so handlers
// may call back into read methods on Manager safely.
// Hooks should not block for extended periods — spawn goroutines for
// long-running work.
type TypeHandler interface {
	Flags() GroupTypeFlags
	OnCreate(groupID, name string, maxMembers int) error
	OnJoin(groupID, peerID string, isHost bool)
	OnLeave(groupID, peerID string, isHost bool)
	OnClose(groupID string)
	OnEvent(evt *Event)
}

// GroupTypeFlagsForGroup returns the GroupTypeFlags for a group's group_type.
// Returns default flags (all true) if no handler is registered.
func (m *Manager) GroupTypeFlagsForGroup(groupID string) GroupTypeFlags {
	if h := m.handlerForGroup(groupID); h != nil {
		return h.Flags()
	}
	return GroupTypeFlags{HostCanJoin: true}
}

// isVolatileType returns whether the given group type has the Volatile flag set.
func (m *Manager) isVolatileType(groupType string) bool {
	if h := m.handlerForType(groupType); h != nil {
		return h.Flags().Volatile
	}
	return false
}

// handlerForType returns the registered TypeHandler for the given group_type, or nil.
func (m *Manager) handlerForType(groupType string) TypeHandler {
	m.mu.RLock()
	h := m.handlers[groupType]
	m.mu.RUnlock()
	return h
}

// handlerForGroup returns the registered TypeHandler for the group's group_type, or nil.
func (m *Manager) handlerForGroup(groupID string) TypeHandler {
	m.mu.RLock()
	groupType := m.groupTypeForGroupLocked(groupID)
	h := m.handlers[groupType]
	m.mu.RUnlock()
	return h
}
