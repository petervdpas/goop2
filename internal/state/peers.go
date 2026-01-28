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

// PeerEvent represents a change to the peer table
type PeerEvent struct {
	Type    string   `json:"type"` // "update", "remove", "snapshot"
	PeerID  string   `json:"peer_id,omitempty"`
	Peer    *SeenPeer `json:"peer,omitempty"`
	Peers   map[string]SeenPeer `json:"peers,omitempty"` // for snapshot
}

type PeerTable struct {
	mu        sync.Mutex
	peers     map[string]SeenPeer
	listeners []chan PeerEvent
}

func NewPeerTable() *PeerTable {
	return &PeerTable{
		peers:     map[string]SeenPeer{},
		listeners: make([]chan PeerEvent, 0),
	}
}

func (t *PeerTable) Upsert(id, content string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	peer := SeenPeer{Content: content, LastSeen: time.Now()}
	t.peers[id] = peer
	t.notifyListeners(PeerEvent{Type: "update", PeerID: id, Peer: &peer})
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
	t.notifyListeners(PeerEvent{Type: "remove", PeerID: id})
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
	// Notify listeners about removals
	for _, id := range dropped {
		t.notifyListeners(PeerEvent{Type: "remove", PeerID: id})
	}
	return dropped
}

// Subscribe returns a channel that receives peer events
func (t *PeerTable) Subscribe() chan PeerEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	ch := make(chan PeerEvent, 16)
	t.listeners = append(t.listeners, ch)
	return ch
}

// Unsubscribe removes a listener channel
func (t *PeerTable) Unsubscribe(ch chan PeerEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, listener := range t.listeners {
		if listener == ch {
			close(listener)
			t.listeners = append(t.listeners[:i], t.listeners[i+1:]...)
			return
		}
	}
}

// notifyListeners sends an event to all listeners (must be called with lock held)
func (t *PeerTable) notifyListeners(evt PeerEvent) {
	for _, ch := range t.listeners {
		select {
		case ch <- evt:
		default:
			// Listener buffer full, skip
		}
	}
}
