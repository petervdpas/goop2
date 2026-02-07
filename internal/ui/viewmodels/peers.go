// internal/ui/viewmodels/peers.go

package viewmodels

import (
	"sort"
	"time"

	"goop/internal/state"
)

type PeerRow struct {
	ID            string `json:"ID"`
	Content       string `json:"Content"`
	Email         string `json:"Email"`
	AvatarHash    string `json:"AvatarHash"`
	VideoDisabled bool   `json:"VideoDisabled"`
	LastSeen      time.Time `json:"LastSeen"`
}

type PeersVM struct {
	BaseVM
	Peers []PeerRow
}

func BuildPeerRows(m map[string]state.SeenPeer) []PeerRow {
	rows := make([]PeerRow, 0, len(m))
	for id, sp := range m {
		rows = append(rows, PeerRow{
			ID:            id,
			Content:       sp.Content,
			Email:         sp.Email,
			AvatarHash:    sp.AvatarHash,
			VideoDisabled: sp.VideoDisabled,
			LastSeen:      sp.LastSeen,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}
