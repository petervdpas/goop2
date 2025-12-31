// internal/ui/render/viewmodels.go

package render

import (
	"sort"
	"time"

	"goop/internal/config"
	"goop/internal/state"
)

type BaseVM struct {
	Title       string
	Active      string
	SelfName    string
	SelfID      string
	ContentTmpl string
	BaseURL     string
}

// ---------- Peers ----------

type PeerRow struct {
	ID       string
	Content  string
	LastSeen time.Time
}

type PeersVM struct {
	BaseVM
	Peers []PeerRow
}

func BuildPeerRows(m map[string]state.SeenPeer) []PeerRow {
	rows := make([]PeerRow, 0, len(m))
	for id, sp := range m {
		rows = append(rows, PeerRow{
			ID:       id,
			Content:  sp.Content,
			LastSeen: sp.LastSeen,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

// ---------- Self ----------

type SelfVM struct {
	BaseVM
}

// ---------- Peer content ----------

type PeerContentVM struct {
	BaseVM
	PeerID  string
	Content string
}

// ---------- Settings ----------

type SettingsVM struct {
	BaseVM
	CfgPath string
	CSRF    string

	Saved bool
	Error string

	Cfg config.Config
}

// ---------- Logs ----------

type LogsVM struct {
	BaseVM
}

// ---------- Editor ----------

type EditorVM struct {
	BaseVM
	CSRF string

	Path    string
	Content string
	ETag    string

	Dir   string
	Files []EditorFileRow

	Tree  []EditorTreeRow
	Saved bool
	Error string
}

type EditorFileRow struct {
	Path  string // root-relative
	Size  int64
	ETag  string
	Mod   int64 // unix seconds
	IsDir bool
}

type EditorTreeRow struct {
	Path  string
	IsDir bool
	Depth int
}
