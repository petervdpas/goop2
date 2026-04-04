package chat

import "github.com/petervdpas/goop2/internal/group"

func NewTestManager(grpMgr *group.Manager, selfID string, peerName func(string) string) *Manager {
	m := &Manager{
		grp:      grpMgr,
		selfID:   selfID,
		peerName: peerName,
		rooms:    make(map[string]*roomState),
	}
	grpMgr.RegisterType(GroupTypeName, m)
	return m
}
