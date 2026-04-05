package datafed

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/mq"
	"github.com/petervdpas/goop2/internal/orm/schema"
)

const GroupTypeName = "data-federation"

type Manager struct {
	mu      sync.RWMutex
	mq      mq.Transport
	grpMgr  *group.Manager
	selfID  string
	schemas func() []*schema.Table

	groups   map[string]*federatedGroup
	onChange func()
}

func New(transport mq.Transport, grpMgr *group.Manager, selfID string, schemas func() []*schema.Table) *Manager {
	m := &Manager{
		mq:      transport,
		grpMgr:  grpMgr,
		selfID:  selfID,
		schemas: schemas,
		groups:  make(map[string]*federatedGroup),
	}
	grpMgr.RegisterType(GroupTypeName, m)

	transport.SubscribeTopic("peer:", func(from, topic string, payload any) {
		switch topic {
		case "peer:gone":
			m.handlePeerGone(from, payload)
		case "peer:announce":
			m.handlePeerAnnounce(from, payload)
		}
	})

	return m
}

func (m *Manager) GroupTypeName() string { return GroupTypeName }

func (m *Manager) SetOnChange(fn func()) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

func (m *Manager) notifyChange() {
	m.mu.RLock()
	fn := m.onChange
	m.mu.RUnlock()
	if fn != nil {
		go fn()
	}
}

func (m *Manager) Flags() group.GroupTypeFlags {
	return group.GroupTypeFlags{HostCanJoin: true}
}

func (m *Manager) OnCreate(groupID, name string, maxMembers int) error {
	m.mu.Lock()
	m.groups[groupID] = newFederatedGroup()
	m.mu.Unlock()
	log.Printf("DATA-FED: Group %s created (%s)", groupID, name)
	return nil
}

func (m *Manager) OnJoin(groupID, peerID string, isHost bool) {
	m.mu.Lock()
	fg, ok := m.groups[groupID]
	if !ok {
		fg = newFederatedGroup()
		m.groups[groupID] = fg
	}
	m.mu.Unlock()

	if isHost {
		tables := m.contextTables()
		if len(tables) > 0 {
			fg.rwmu.Lock()
			fg.contributions[m.selfID] = &PeerContribution{
				PeerID: m.selfID,
				Tables: tables,
			}
			fg.rwmu.Unlock()
		}
	}

	m.publishSync(groupID)
	m.notifyChange()
	log.Printf("DATA-FED: %s joined group %s (host=%v)", peerID, groupID, isHost)
}

func (m *Manager) OnLeave(groupID, peerID string, isHost bool) {
	m.mu.RLock()
	fg, ok := m.groups[groupID]
	m.mu.RUnlock()

	if ok {
		fg.rwmu.Lock()
		delete(fg.contributions, peerID)
		fg.rwmu.Unlock()
		m.publishSync(groupID)
	}
	m.notifyChange()
	log.Printf("DATA-FED: %s left group %s", peerID, groupID)
}

func (m *Manager) OnClose(groupID string) {
	m.mu.Lock()
	delete(m.groups, groupID)
	m.mu.Unlock()
	m.notifyChange()
	log.Printf("DATA-FED: Group %s closed", groupID)
}

func (m *Manager) OnEvent(evt *group.Event) {
	if evt.Type != "msg" {
		return
	}

	raw, ok := group.ExtractControl(evt.Payload, GroupTypeName)
	if !ok {
		return
	}

	var msg controlMsg
	if json.Unmarshal(raw, &msg) != nil {
		return
	}

	switch msg.Action {
	case "schema-offer":
		m.handleSchemaOffer(evt.Group, evt.From, raw)
	case "schema-withdraw":
		m.handleSchemaWithdraw(evt.Group, evt.From)
	}
}

func (m *Manager) contextTables() []schema.Table {
	ptrs := m.schemas()
	tables := make([]schema.Table, 0, len(ptrs))
	for _, t := range ptrs {
		if t.Access != nil && t.Access.Read == "local" {
			continue
		}
		tables = append(tables, *t)
	}
	return tables
}

func (m *Manager) ContextTablesForNames(names []string) []schema.Table {
	all := m.schemas()
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}
	var result []schema.Table
	for _, t := range all {
		if nameSet[t.Name] {
			result = append(result, *t)
		}
	}
	return result
}

func (m *Manager) AllGroups() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.groups))
	for id := range m.groups {
		ids = append(ids, id)
	}
	return ids
}
