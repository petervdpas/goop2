package datafed

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/mq"
	"github.com/petervdpas/goop2/internal/orm/schema"
)

const AppType = "data-federation"

type Relationship struct {
	FromTable  string `json:"from_table"`
	FromColumn string `json:"from_column"`
	ToTable    string `json:"to_table"`
	ToColumn   string `json:"to_column"`
}

type SchemaOffer struct {
	Tables        []schema.Table `json:"tables"`
	Relationships []Relationship `json:"relationships"`
}

type SchemaAccept struct {
	Accepted []string `json:"accepted"`
}

type PeerContribution struct {
	PeerID        string
	Tables        []schema.Table
	Relationships []Relationship
}

type Manager struct {
	mu      sync.RWMutex
	mqMgr   *mq.Manager
	grpMgr  *group.Manager
	selfID  string
	schemas func() []*schema.Table

	groups map[string]*federatedGroup
}

type federatedGroup struct {
	rwmu          sync.RWMutex
	contributions map[string]*PeerContribution
	suspended     map[string]*PeerContribution
}

func New(mqMgr *mq.Manager, grpMgr *group.Manager, selfID string, schemas func() []*schema.Table) *Manager {
	m := &Manager{
		mqMgr:   mqMgr,
		grpMgr:  grpMgr,
		selfID:  selfID,
		schemas: schemas,
		groups:  make(map[string]*federatedGroup),
	}
	grpMgr.RegisterType(AppType, m)

	mqMgr.SubscribeTopic("peer:", func(from, topic string, payload any) {
		switch topic {
		case "peer:gone":
			m.handlePeerGone(from, payload)
		case "peer:announce":
			m.handlePeerAnnounce(from, payload)
		}
	})

	return m
}

func (m *Manager) AppType() string { return AppType }

func (m *Manager) Flags() group.TypeFlags {
	return group.TypeFlags{HostCanJoin: true}
}

func (m *Manager) OnCreate(groupID, name string, maxMembers int, volatile bool) error {
	m.mu.Lock()
	m.groups[groupID] = &federatedGroup{
		contributions: make(map[string]*PeerContribution),
		suspended:     make(map[string]*PeerContribution),
	}
	m.mu.Unlock()
	log.Printf("DATA-FED: Group %s created (%s)", groupID, name)
	return nil
}

func (m *Manager) OnJoin(groupID, peerID string, isHost bool) {
	m.mu.Lock()
	fg, ok := m.groups[groupID]
	if !ok {
		fg = &federatedGroup{
			contributions: make(map[string]*PeerContribution),
			suspended:     make(map[string]*PeerContribution),
		}
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
	log.Printf("DATA-FED: %s left group %s", peerID, groupID)
}

func (m *Manager) OnClose(groupID string) {
	m.mu.Lock()
	delete(m.groups, groupID)
	m.mu.Unlock()
	log.Printf("DATA-FED: Group %s closed", groupID)
}

func (m *Manager) OnEvent(evt *group.Event) {
	if evt.Type != "msg" {
		return
	}

	raw, ok := group.ExtractControl(evt.Payload, AppType)
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

type controlMsg struct {
	Action string `json:"action"`
}

type schemaOfferMsg struct {
	Action        string         `json:"action"`
	Tables        []schema.Table `json:"tables"`
	Relationships []Relationship `json:"relationships"`
}

type schemaSyncMsg struct {
	Action        string                      `json:"action"`
	Contributions map[string][]schema.Table    `json:"contributions"`
	Relationships map[string][]Relationship    `json:"relationships"`
}

func (m *Manager) handleSchemaOffer(groupID, from string, raw json.RawMessage) {
	var offer schemaOfferMsg
	if json.Unmarshal(raw, &offer) != nil {
		return
	}

	m.mu.RLock()
	fg, ok := m.groups[groupID]
	m.mu.RUnlock()
	if !ok {
		return
	}

	fg.rwmu.Lock()
	fg.contributions[from] = &PeerContribution{
		PeerID:        from,
		Tables:        offer.Tables,
		Relationships: offer.Relationships,
	}
	fg.rwmu.Unlock()

	m.publishSync(groupID)
	log.Printf("DATA-FED: %s offered %d tables to group %s", from, len(offer.Tables), groupID)
}

func (m *Manager) handleSchemaWithdraw(groupID, from string) {
	m.mu.RLock()
	fg, ok := m.groups[groupID]
	m.mu.RUnlock()
	if !ok {
		return
	}

	fg.rwmu.Lock()
	delete(fg.contributions, from)
	fg.rwmu.Unlock()

	m.publishSync(groupID)
	log.Printf("DATA-FED: %s withdrew from group %s", from, groupID)
}

func (m *Manager) publishSync(groupID string) {
	m.mu.RLock()
	fg, ok := m.groups[groupID]
	m.mu.RUnlock()
	if !ok {
		return
	}

	sync := schemaSyncMsg{
		Action:        "schema-sync",
		Contributions: make(map[string][]schema.Table),
		Relationships: make(map[string][]Relationship),
	}

	fg.rwmu.RLock()
	for peerID, c := range fg.contributions {
		sync.Contributions[peerID] = c.Tables
		if len(c.Relationships) > 0 {
			sync.Relationships[peerID] = c.Relationships
		}
	}
	fg.rwmu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), group.BroadcastTimeout)
	defer cancel()
	_ = m.grpMgr.SendControl(groupID, AppType, sync)
	_ = ctx
}

func (m *Manager) contextTables() []schema.Table {
	ptrs := m.schemas()
	tables := make([]schema.Table, len(ptrs))
	for i, t := range ptrs {
		tables[i] = *t
	}
	return tables
}

func (m *Manager) GroupContributions(groupID string) map[string]*PeerContribution {
	m.mu.RLock()
	fg, ok := m.groups[groupID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}

	fg.rwmu.RLock()
	defer fg.rwmu.RUnlock()

	result := make(map[string]*PeerContribution, len(fg.contributions))
	for k, v := range fg.contributions {
		result[k] = v
	}
	return result
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

func (m *Manager) OfferTables(groupID string, tables []schema.Table, rels []Relationship) {
	m.mu.RLock()
	fg, ok := m.groups[groupID]
	m.mu.RUnlock()
	if !ok {
		return
	}

	fg.rwmu.Lock()
	fg.contributions[m.selfID] = &PeerContribution{
		PeerID:        m.selfID,
		Tables:        tables,
		Relationships: rels,
	}
	fg.rwmu.Unlock()

	m.publishSync(groupID)

	msg := schemaOfferMsg{
		Action:        "schema-offer",
		Tables:        tables,
		Relationships: rels,
	}
	_ = m.grpMgr.SendControl(groupID, AppType, msg)
}

func (m *Manager) WithdrawTables(groupID string) {
	m.mu.RLock()
	fg, ok := m.groups[groupID]
	m.mu.RUnlock()
	if !ok {
		return
	}

	fg.rwmu.Lock()
	delete(fg.contributions, m.selfID)
	fg.rwmu.Unlock()

	m.publishSync(groupID)

	_ = m.grpMgr.SendControl(groupID, AppType, controlMsg{Action: "schema-withdraw"})
}

func (m *Manager) handlePeerGone(from string, payload any) {
	peerID := extractPeerID(payload)
	if peerID == "" || peerID == m.selfID {
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	changed := false
	for groupID, fg := range m.groups {
		fg.rwmu.Lock()
		if c, ok := fg.contributions[peerID]; ok {
			if fg.suspended == nil {
				fg.suspended = make(map[string]*PeerContribution)
			}
			fg.suspended[peerID] = c
			delete(fg.contributions, peerID)
			changed = true
			log.Printf("DATA-FED: suspended %s from group %s (peer gone)", peerID, groupID)
		}
		fg.rwmu.Unlock()
	}

	if changed {
		for groupID := range m.groups {
			m.publishSync(groupID)
		}
	}
}

func (m *Manager) handlePeerAnnounce(from string, payload any) {
	peerID := extractPeerID(payload)
	if peerID == "" || peerID == m.selfID {
		return
	}

	if isOffline(payload) {
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	changed := false
	for groupID, fg := range m.groups {
		fg.rwmu.Lock()
		if c, ok := fg.suspended[peerID]; ok {
			fg.contributions[peerID] = c
			delete(fg.suspended, peerID)
			changed = true
			log.Printf("DATA-FED: restored %s to group %s (peer back)", peerID, groupID)
		}
		fg.rwmu.Unlock()
	}

	if changed {
		for groupID := range m.groups {
			m.publishSync(groupID)
		}
	}
}

func extractPeerID(payload any) string {
	switch p := payload.(type) {
	case map[string]any:
		if id, ok := p["peerID"].(string); ok {
			return id
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	var obj struct {
		PeerID string `json:"peerID"`
	}
	if json.Unmarshal(data, &obj) == nil {
		return obj.PeerID
	}
	return ""
}

func isOffline(payload any) bool {
	switch p := payload.(type) {
	case map[string]any:
		if offline, ok := p["offline"].(bool); ok {
			return offline
		}
	}
	return false
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

