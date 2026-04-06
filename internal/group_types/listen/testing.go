package listen

import "github.com/petervdpas/goop2/internal/group"

func NewTestManager(store *group.StateStore) *Manager {
	return &Manager{
		store: store,
		pipes: make(map[string]*listenerPipe),
	}
}

func (m *Manager) SetTestGroup(id string) {
	m.group = &Group{ID: id}
}

func (m *Manager) SetTestQueue(paths []string, idx int) {
	m.queue = paths
	m.queueIdx = idx
}
