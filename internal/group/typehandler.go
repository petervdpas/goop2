package group

// TypeHandler defines lifecycle hooks for a group app_type.
// Each group type (listen, cluster, files, etc.) can register a handler
// to receive lifecycle callbacks when groups of that type are created,
// joined, left, closed, or receive events.
//
// All hooks are called outside the group manager's locks, so handlers
// may call back into read methods on Manager safely.
// Hooks should not block for extended periods — spawn goroutines for
// long-running work.
type TypeHandler interface {
	OnCreate(groupID, name string, maxMembers int, volatile bool) error
	OnJoin(groupID, peerID string, welcome *WelcomePayload) error
	OnLeave(groupID, peerID string)
	OnClose(groupID string)
	OnEvent(evt *Event)
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
