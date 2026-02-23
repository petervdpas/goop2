
package state

import (
	"sync"
	"time"
)

type SeenPeer struct {
	Content        string
	Email          string
	AvatarHash     string
	VideoDisabled  bool
	ActiveTemplate string
	Verified       bool
	Reachable      bool
	LastSeen       time.Time
	OfflineSince   time.Time
	Favorite       bool

	// Consecutive probe failures. A peer is only marked unreachable after
	// failStreak >= 2 distinct failure events (> 4 s apart). This prevents
	// a single transient probe timeout from causing the UI to flash.
	failStreak int
	lastFailAt time.Time
}

type PeerEvent struct {
	Type   string              `json:"type"`
	PeerID string              `json:"peer_id,omitempty"`
	Peer   *SeenPeer           `json:"peer,omitempty"`
	Peers  map[string]SeenPeer `json:"peers,omitempty"`
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

func (t *PeerTable) Upsert(id, content, email, avatarHash string, videoDisabled bool, activeTemplate string, verified bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	reachable := true
	favorite := false
	var failStreak int
	var lastFailAt time.Time
	if existing, ok := t.peers[id]; ok {
		if existing.OfflineSince.IsZero() {
			reachable = existing.Reachable
		}
		// Preserve local state across presence updates.
		favorite = existing.Favorite
		failStreak = existing.failStreak
		lastFailAt = existing.lastFailAt
	}
	peer := SeenPeer{
		Content:        content,
		Email:          email,
		AvatarHash:     avatarHash,
		VideoDisabled:  videoDisabled,
		ActiveTemplate: activeTemplate,
		Verified:       verified,
		Reachable:      reachable,
		LastSeen:       time.Now(),
		Favorite:       favorite,
		failStreak:     failStreak,
		lastFailAt:     lastFailAt,
	}
	t.peers[id] = peer
	t.notifyListeners(PeerEvent{Type: "update", PeerID: id, Peer: &peer})
}

func (t *PeerTable) Seed(id, content, email, avatarHash string, videoDisabled bool, activeTemplate string, verified bool, favorite bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.peers[id]; ok {
		return
	}
	sp := SeenPeer{
		Content:        content,
		Email:          email,
		AvatarHash:     avatarHash,
		VideoDisabled:  videoDisabled,
		ActiveTemplate: activeTemplate,
		Verified:       verified,
		Reachable:      false,
		LastSeen:       time.Now(),
		OfflineSince:   time.Now(),
		Favorite:       favorite,
	}
	t.peers[id] = sp
	t.notifyListeners(PeerEvent{Type: "update", PeerID: id, Peer: &sp})
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

func (t *PeerTable) MarkOffline(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	sp, ok := t.peers[id]
	if !ok {
		return
	}
	wasOnline := sp.OfflineSince.IsZero()
	sp.Reachable = false
	sp.failStreak = 0
	sp.lastFailAt = time.Time{}
	if wasOnline {
		sp.OfflineSince = time.Now()
	}
	t.peers[id] = sp
	if wasOnline {
		t.notifyListeners(PeerEvent{Type: "update", PeerID: id, Peer: &sp})
	}
}

func (t *PeerTable) Get(id string) (SeenPeer, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	sp, ok := t.peers[id]
	return sp, ok
}

func (t *PeerTable) IDs() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	ids := make([]string, 0, len(t.peers))
	for id := range t.peers {
		ids = append(ids, id)
	}
	return ids
}

func (t *PeerTable) SetReachable(id string, reachable bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	sp, ok := t.peers[id]
	if !ok {
		return
	}

	if reachable {
		// Success — reset failure streak immediately and mark reachable.
		sp.failStreak = 0
		sp.lastFailAt = time.Time{}
		if sp.Reachable {
			t.peers[id] = sp
			return
		}
		sp.Reachable = true
		t.peers[id] = sp
		t.notifyListeners(PeerEvent{Type: "update", PeerID: id, Peer: &sp})
		return
	}

	// Failure path — only mark unreachable after 2 distinct failure events.
	// Events within 4 s of each other count as one (concurrent probe dedup).
	if time.Since(sp.lastFailAt) > 4*time.Second {
		sp.failStreak++
		sp.lastFailAt = time.Now()
	}
	t.peers[id] = sp

	if sp.failStreak >= 2 && sp.Reachable {
		sp.Reachable = false
		t.peers[id] = sp
		t.notifyListeners(PeerEvent{Type: "update", PeerID: id, Peer: &sp})
	}
}

func (t *PeerTable) SetFavorite(id string, favorite bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	sp, ok := t.peers[id]
	if !ok {
		return
	}
	sp.Favorite = favorite
	t.peers[id] = sp
	t.notifyListeners(PeerEvent{Type: "update", PeerID: id, Peer: &sp})
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

// PruneStale moves online peers with expired TTL to offline state, then removes
// offline peers that have exceeded the grace period.
func (t *PeerTable) PruneStale(ttlCutoff, graceCutoff time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, sp := range t.peers {
		if sp.OfflineSince.IsZero() {
			if sp.LastSeen.Before(ttlCutoff) {
				sp.Reachable = false
				sp.failStreak = 0
				sp.lastFailAt = time.Time{}
				sp.OfflineSince = time.Now()
				t.peers[id] = sp
				t.notifyListeners(PeerEvent{Type: "update", PeerID: id, Peer: &sp})
			}
		} else {
			if sp.OfflineSince.Before(graceCutoff) {
				delete(t.peers, id)
				t.notifyListeners(PeerEvent{Type: "remove", PeerID: id})
			}
		}
	}
}

func (t *PeerTable) Subscribe() chan PeerEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	ch := make(chan PeerEvent, 16)
	t.listeners = append(t.listeners, ch)
	return ch
}

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

func (t *PeerTable) notifyListeners(evt PeerEvent) {
	for _, ch := range t.listeners {
		select {
		case ch <- evt:
		default:
		}
	}
}
