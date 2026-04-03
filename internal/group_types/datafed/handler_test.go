package datafed

import (
	"testing"

	"github.com/petervdpas/goop2/internal/group"
	ormschema "github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/storage"
)

func testManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	grpMgr := group.NewTestManager(db, "self")
	t.Cleanup(func() { grpMgr.Close() })

	m := &Manager{
		grpMgr:  grpMgr,
		selfID:  "self",
		schemas: func() []*ormschema.Table { return nil },
		groups:  make(map[string]*federatedGroup),
	}
	grpMgr.RegisterType(GroupTypeName, m)
	return m
}

func TestFlags(t *testing.T) {
	m := &Manager{}
	if !m.Flags().HostCanJoin {
		t.Fatal("datafed handler should allow host to join")
	}
}

func TestOnCreateAndClose(t *testing.T) {
	m := testManager(t)

	if err := m.OnCreate("g1", "Federation", 0, false); err != nil {
		t.Fatal(err)
	}
	if len(m.groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(m.groups))
	}

	m.OnClose("g1")
	if len(m.groups) != 0 {
		t.Fatal("group should be removed on close")
	}
}

func TestOnLeaveRemovesContribution(t *testing.T) {
	m := testManager(t)
	m.OnCreate("g1", "Fed", 0, false)

	fg := m.groups["g1"]
	fg.rwmu.Lock()
	fg.contributions["peer-b"] = &PeerContribution{
		PeerID: "peer-b",
		Tables: []ormschema.Table{{Name: "items"}},
	}
	fg.rwmu.Unlock()

	m.OnLeave("g1", "peer-b", false)

	contribs := m.GroupContributions("g1")
	if len(contribs) != 0 {
		t.Fatal("contribution should be removed on leave")
	}
}

func TestAllGroups(t *testing.T) {
	m := testManager(t)
	m.OnCreate("g1", "A", 0, false)
	m.OnCreate("g2", "B", 0, false)

	ids := m.AllGroups()
	if len(ids) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(ids))
	}
}
