// internal/app/handle.go
package app

import (
	"sync"
)

type PeerInfo struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	LastSeen int64  `json:"lastSeen"` // unix seconds
}

var (
	mu        sync.RWMutex
	rtPeers   snapshotter
	rtSelfID  string
	rtSelfLbl string
)

type snapshotter interface {
	Snapshot() map[string]SeenPeer
}

type SeenPeer struct {
	Content  string
	LastSeen timeLike
}

type timeLike interface {
	Unix() int64
}

func setRuntime(peers snapshotter, selfID, selfLbl string) {
	mu.Lock()
	defer mu.Unlock()
	rtPeers = peers
	rtSelfID = selfID
	rtSelfLbl = selfLbl
}

func RuntimeSelf() (id, label string, ok bool) {
	mu.RLock()
	defer mu.RUnlock()
	if rtPeers == nil {
		return "", "", false
	}
	return rtSelfID, rtSelfLbl, true
}

func PeersSnapshot() []PeerInfo {
	mu.RLock()
	p := rtPeers
	mu.RUnlock()

	if p == nil {
		return nil
	}

	snap := p.Snapshot()
	out := make([]PeerInfo, 0, len(snap))
	for id, sp := range snap {
		out = append(out, PeerInfo{
			ID:       id,
			Content:  sp.Content,
			LastSeen: sp.LastSeen.Unix(),
		})
	}
	return out
}
