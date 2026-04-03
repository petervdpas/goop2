package datafed

import (
	"testing"

	ormschema "github.com/petervdpas/goop2/internal/orm/schema"
)

func TestPeerGoneSuspends(t *testing.T) {
	m := testManager(t)
	m.OnCreate("g1", "Fed", 0, false)

	fg := m.groups["g1"]
	fg.rwmu.Lock()
	fg.contributions["peer-c"] = &PeerContribution{
		PeerID: "peer-c",
		Tables: []ormschema.Table{{Name: "data"}},
	}
	fg.rwmu.Unlock()

	m.handlePeerGone("", map[string]any{"peerID": "peer-c"})

	fg.rwmu.RLock()
	_, active := fg.contributions["peer-c"]
	_, suspended := fg.suspended["peer-c"]
	fg.rwmu.RUnlock()

	if active {
		t.Fatal("peer-c should not be in active contributions")
	}
	if !suspended {
		t.Fatal("peer-c should be in suspended")
	}
}

func TestPeerAnnounceRestores(t *testing.T) {
	m := testManager(t)
	m.OnCreate("g1", "Fed", 0, false)

	fg := m.groups["g1"]
	fg.rwmu.Lock()
	fg.suspended["peer-c"] = &PeerContribution{
		PeerID: "peer-c",
		Tables: []ormschema.Table{{Name: "data"}},
	}
	fg.rwmu.Unlock()

	m.handlePeerAnnounce("", map[string]any{"peerID": "peer-c"})

	fg.rwmu.RLock()
	_, active := fg.contributions["peer-c"]
	_, suspended := fg.suspended["peer-c"]
	fg.rwmu.RUnlock()

	if !active {
		t.Fatal("peer-c should be restored to active")
	}
	if suspended {
		t.Fatal("peer-c should not be in suspended after announce")
	}
}
