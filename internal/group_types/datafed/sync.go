package datafed

import (
	"context"
	"encoding/json"
	"log"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/orm/schema"
)

type controlMsg struct {
	Action string `json:"action"`
}

type schemaOfferMsg struct {
	Action        string         `json:"action"`
	Tables        []schema.Table `json:"tables"`
	Relationships []Relationship `json:"relationships"`
}

type schemaSyncMsg struct {
	Action        string                   `json:"action"`
	Contributions map[string][]schema.Table `json:"contributions"`
	Relationships map[string][]Relationship `json:"relationships"`
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
	_ = m.grpMgr.SendControl(groupID, GroupTypeName, sync)
	_ = ctx
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
	m.notifyChange()
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
	m.notifyChange()
	log.Printf("DATA-FED: %s withdrew from group %s", from, groupID)
}
