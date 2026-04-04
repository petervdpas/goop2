package datafed

import (
	"github.com/petervdpas/goop2/internal/group"
	ormschema "github.com/petervdpas/goop2/internal/orm/schema"
)

func NewTestManager(grpMgr *group.Manager, selfID string, schemas func() []*ormschema.Table) *Manager {
	m := &Manager{
		grpMgr:  grpMgr,
		selfID:  selfID,
		schemas: schemas,
		groups:  make(map[string]*federatedGroup),
	}
	grpMgr.RegisterType(GroupTypeName, m)
	return m
}

func (m *Manager) AddTestContribution(groupID, peerID, tableName string) {
	m.mu.RLock()
	fg, ok := m.groups[groupID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	fg.rwmu.Lock()
	fg.contributions[peerID] = &PeerContribution{
		PeerID: peerID,
		Tables: []ormschema.Table{{Name: tableName}},
	}
	fg.rwmu.Unlock()
}
