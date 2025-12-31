// internal/state/peers.go

package state

import (
	"sync"
	"time"
)

type SeenPeer struct {
	Content  string
	LastSeen time.Time
}

type PeerTable struct {
	mu    sync.Mutex
	peers map[string]SeenPeer
}

func NewPeerTable() *PeerTable {
	return &PeerTable{peers: map[string]SeenPeer{}}
}

func (t *PeerTable) Upsert(id, content string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peers[id] = SeenPeer{Content: content, LastSeen: time.Now()}
}

func (t *PeerTable) Touch(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	sp, ok := t.peers[id]
	if !ok {
		return
	}
	sp.LastSeen = time.Now()
	t.peers[id] = sp
}

func (t *PeerTable) Remove(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.peers, id)
}

func (t *PeerTable) Snapshot() map[string]SeenPeer {
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := make(map[string]SeenPeer, len(t.peers))
	for k, v := range t.peers {
		cp[k] = v
	}
	return cp
}

func (t *PeerTable) PruneOlderThan(cutoff time.Time) (dropped []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, sp := range t.peers {
		if sp.LastSeen.Before(cutoff) {
			delete(t.peers, id)
			dropped = append(dropped, id)
		}
	}
	return dropped
}
