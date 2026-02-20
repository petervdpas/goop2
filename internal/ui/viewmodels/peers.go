
package viewmodels

import (
	"sort"
	"time"

	"github.com/petervdpas/goop2/internal/state"
)

type PeerRow struct {
	ID             string    `json:"ID"`
	Content        string    `json:"Content"`
	Email          string    `json:"Email"`
	AvatarHash     string    `json:"AvatarHash"`
	VideoDisabled  bool      `json:"VideoDisabled"`
	ActiveTemplate string    `json:"ActiveTemplate"`
	Verified       bool      `json:"Verified"`
	Reachable      bool      `json:"Reachable"`
	Offline        bool      `json:"Offline"`
	LastSeen       time.Time `json:"LastSeen"`
	Favorite       bool      `json:"Favorite"`
}

type PeersVM struct {
	BaseVM
	Peers             []PeerRow
	SelfVideoDisabled bool
	HideUnverified    bool
	Splash            string
}

func BuildPeerRow(id string, sp state.SeenPeer) PeerRow {
	return PeerRow{
		ID:             id,
		Content:        sp.Content,
		Email:          sp.Email,
		AvatarHash:     sp.AvatarHash,
		VideoDisabled:  sp.VideoDisabled,
		ActiveTemplate: sp.ActiveTemplate,
		Verified:       sp.Verified,
		Reachable:      sp.Reachable,
		Offline:        !sp.OfflineSince.IsZero(),
		LastSeen:       sp.LastSeen,
		Favorite:       sp.Favorite,
	}
}

func BuildPeerRows(m map[string]state.SeenPeer) []PeerRow {
	rows := make([]PeerRow, 0, len(m))
	for id, sp := range m {
		rows = append(rows, BuildPeerRow(id, sp))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}
