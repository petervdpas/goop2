package group

import (
	"testing"

	"github.com/petervdpas/goop2/internal/storage"
)

func TestPurgeInvalidRemovesOrphanType(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	m := NewTestManager(db, "self")

	if err := m.CreateGroup("g1", "Orphan", "nonexistent", "ctx", 0, false); err != nil {
		t.Fatal(err)
	}

	n := m.PurgeInvalid(nil)
	if n != 1 {
		t.Fatalf("expected 1 purged, got %d", n)
	}

	groups, _ := m.ListHostedGroups()
	for _, g := range groups {
		if g.ID == "g1" {
			t.Fatal("orphan group should have been removed")
		}
	}
}

func TestPurgeInvalidRemovesMissingContext(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	m := NewTestManager(db, "self")
	m.RegisterType("listen", &stubHandler{})

	if err := m.CreateGroup("g1", "Room", "listen", "", 0, false); err != nil {
		t.Fatal(err)
	}

	n := m.PurgeInvalid(nil)
	if n != 1 {
		t.Fatalf("expected 1 purged, got %d", n)
	}
}

func TestPurgeInvalidKeepsValid(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	m := NewTestManager(db, "self")
	m.RegisterType("listen", &stubHandler{})

	if err := m.CreateGroup("g1", "Room", "listen", "Friday Jams", 0, false); err != nil {
		t.Fatal(err)
	}

	n := m.PurgeInvalid(nil)
	if n != 0 {
		t.Fatalf("expected 0 purged, got %d", n)
	}

	groups, _ := m.ListHostedGroups()
	found := false
	for _, g := range groups {
		if g.ID == "g1" {
			found = true
		}
	}
	if !found {
		t.Fatal("valid group should still exist")
	}
}

func TestPurgeInvalidSkipsType(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	m := NewTestManager(db, "self")

	if err := m.CreateGroup("g1", "Blog Co-authors", "template", "Blog", 0, false); err != nil {
		t.Fatal(err)
	}

	n := m.PurgeInvalid(map[string]bool{"template": true})
	if n != 0 {
		t.Fatalf("expected 0 purged (template skipped), got %d", n)
	}
}

func TestPurgeValidTemplateWithHandler(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	m := NewTestManager(db, "self")
	m.RegisterType("template", &stubHandler{})

	if err := m.CreateGroup("g1", "Blog Co-authors", "template", "Blog", 0, false); err != nil {
		t.Fatal(err)
	}

	n := m.PurgeInvalid(nil)
	if n != 0 {
		t.Fatalf("expected 0 purged (template has handler), got %d", n)
	}
}

type stubHandler struct{}

func (s *stubHandler) Flags() TypeFlags                                     { return TypeFlags{HostCanJoin: true} }
func (s *stubHandler) OnCreate(groupID, name string, max int, vol bool) error { return nil }
func (s *stubHandler) OnJoin(groupID, peerID string, isHost bool)           {}
func (s *stubHandler) OnLeave(groupID, peerID string, isHost bool)          {}
func (s *stubHandler) OnClose(groupID string)                               {}
func (s *stubHandler) OnEvent(evt *Event)                                   {}
