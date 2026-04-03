package datafed

import (
	"sync"

	"github.com/petervdpas/goop2/internal/orm/schema"
)

// Relationship describes a foreign-key link between federated tables.
type Relationship struct {
	FromTable  string `json:"from_table"`
	FromColumn string `json:"from_column"`
	ToTable    string `json:"to_table"`
	ToColumn   string `json:"to_column"`
}

// SchemaOffer is the payload a peer sends when offering tables.
type SchemaOffer struct {
	Tables        []schema.Table `json:"tables"`
	Relationships []Relationship `json:"relationships"`
}

// SchemaAccept is the payload a host sends when accepting offered tables.
type SchemaAccept struct {
	Accepted []string `json:"accepted"`
}

// PeerContribution tracks the tables and relationships a peer has offered.
type PeerContribution struct {
	PeerID        string
	Tables        []schema.Table
	Relationships []Relationship
}

type federatedGroup struct {
	rwmu          sync.RWMutex
	contributions map[string]*PeerContribution
	suspended     map[string]*PeerContribution
}

func newFederatedGroup() *federatedGroup {
	return &federatedGroup{
		contributions: make(map[string]*PeerContribution),
		suspended:     make(map[string]*PeerContribution),
	}
}

// GroupContributions returns a snapshot of the contributions for a group.
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

// AllPeerSources returns all remote peer contributions across all groups.
func (m *Manager) AllPeerSources() map[string][]schema.Table {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]schema.Table)
	for _, fg := range m.groups {
		fg.rwmu.RLock()
		for peerID, c := range fg.contributions {
			if peerID == m.selfID {
				continue
			}
			existing := result[peerID]
			seen := make(map[string]bool, len(existing))
			for _, t := range existing {
				seen[t.Name] = true
			}
			for _, t := range c.Tables {
				if !seen[t.Name] {
					result[peerID] = append(result[peerID], t)
				}
			}
		}
		fg.rwmu.RUnlock()
	}
	return result
}

// OfferTables adds the host's own tables to a group and broadcasts the offer.
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
	_ = m.grpMgr.SendControl(groupID, GroupTypeName, msg)
}

// WithdrawTables removes the host's tables from a group and broadcasts the withdrawal.
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

	_ = m.grpMgr.SendControl(groupID, GroupTypeName, controlMsg{Action: "schema-withdraw"})
}
