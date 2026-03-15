package group

// TypeFlags declares per-type rules for a group app_type.
type TypeFlags struct {
	HostCanJoin bool // whether the host can join their own group as a member
}

// TypeHandler defines lifecycle hooks for a group app_type.
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
	Flags() TypeFlags
	OnCreate(groupID, name string, maxMembers int, volatile bool) error
	OnJoin(groupID, peerID string, isHost bool)
	OnLeave(groupID, peerID string, isHost bool)
	OnClose(groupID string)
	OnEvent(evt *Event)
}

// TypeFlagsForGroup returns the TypeFlags for a group's app_type.
// Returns default flags (all true) if no handler is registered.
func (m *Manager) TypeFlagsForGroup(groupID string) TypeFlags {
	if h := m.handlerForGroup(groupID); h != nil {
		return h.Flags()
	}
	return TypeFlags{HostCanJoin: true}
}

// handlerForType returns the registered TypeHandler for the given app_type, or nil.
func (m *Manager) handlerForType(appType string) TypeHandler {
	m.mu.RLock()
	h := m.handlers[appType]
	m.mu.RUnlock()
	return h
}

// handlerForGroup returns the registered TypeHandler for the group's app_type, or nil.
func (m *Manager) handlerForGroup(groupID string) TypeHandler {
	m.mu.RLock()
	appType := m.appTypeForGroupLocked(groupID)
	h := m.handlers[appType]
	m.mu.RUnlock()
	return h
}
