package datafed

import (
	"testing"

	ormschema "github.com/petervdpas/goop2/internal/orm/schema"
)

func TestGroupContributions(t *testing.T) {
	m := testManager(t)
	m.OnCreate("g1", "Fed", 0)

	fg := m.groups["g1"]
	fg.rwmu.Lock()
	fg.contributions["peer-a"] = &PeerContribution{
		PeerID: "peer-a",
		Tables: []ormschema.Table{
			{Name: "users", Columns: []ormschema.Column{{Name: "id", Type: "integer"}}},
		},
	}
	fg.rwmu.Unlock()

	contribs := m.GroupContributions("g1")
	if len(contribs) != 1 {
		t.Fatalf("expected 1 contribution, got %d", len(contribs))
	}
	if contribs["peer-a"] == nil {
		t.Fatal("expected peer-a contribution")
	}
	if contribs["peer-a"].Tables[0].Name != "users" {
		t.Fatalf("expected table 'users', got %q", contribs["peer-a"].Tables[0].Name)
	}
}

func TestGroupContributionsUnknownGroup(t *testing.T) {
	m := testManager(t)
	if c := m.GroupContributions("nonexistent"); c != nil {
		t.Fatalf("expected nil for unknown group, got %v", c)
	}
}

func TestAllPeerSources(t *testing.T) {
	m := testManager(t)
	m.OnCreate("g1", "Fed", 0)

	fg := m.groups["g1"]
	fg.rwmu.Lock()
	fg.contributions["peer-x"] = &PeerContribution{
		PeerID: "peer-x",
		Tables: []ormschema.Table{{Name: "orders"}, {Name: "products"}},
	}
	fg.rwmu.Unlock()

	sources := m.AllPeerSources()
	if len(sources) != 1 {
		t.Fatalf("expected 1 peer source, got %d", len(sources))
	}
	if len(sources["peer-x"]) != 2 {
		t.Fatalf("expected 2 tables from peer-x, got %d", len(sources["peer-x"]))
	}
}

func TestAllPeerSourcesExcludesSelf(t *testing.T) {
	m := testManager(t)
	m.OnCreate("g1", "Fed", 0)

	fg := m.groups["g1"]
	fg.rwmu.Lock()
	fg.contributions["self"] = &PeerContribution{PeerID: "self", Tables: []ormschema.Table{{Name: "local"}}}
	fg.contributions["peer-y"] = &PeerContribution{PeerID: "peer-y", Tables: []ormschema.Table{{Name: "remote"}}}
	fg.rwmu.Unlock()

	sources := m.AllPeerSources()
	if _, ok := sources["self"]; ok {
		t.Fatal("self should be excluded from AllPeerSources")
	}
	if len(sources["peer-y"]) != 1 {
		t.Fatal("peer-y should have 1 table")
	}
}
