package listen

import (
	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/mq"
)

type TestManagerOpts struct {
	SelfID string
	Store  *group.StateStore
	MQ     mq.Transport
}

func NewTestManager(store *group.StateStore) *Manager {
	return NewTestManagerOpts(TestManagerOpts{Store: store})
}

func NewTestManagerOpts(opts TestManagerOpts) *Manager {
	transport := opts.MQ
	if transport == nil {
		transport = mq.NopTransport{}
	}
	return &Manager{
		selfID: opts.SelfID,
		mq:     transport,
		store:  opts.Store,
		pipes:  make(map[string]*listenerPipe),
	}
}

func (m *Manager) SetTestGroup(id string) {
	m.group = &Group{ID: id}
}

func (m *Manager) SetTestGroupFull(g *Group) {
	m.group = g
}

func (m *Manager) SetTestQueue(paths []string, idx int) {
	m.queue = paths
	m.queueIdx = idx
}
