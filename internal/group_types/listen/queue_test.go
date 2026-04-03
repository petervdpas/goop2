package listen

import (
	"testing"
)

func TestQueuePersistence(t *testing.T) {
	dir := t.TempDir()
	store := newStateStore(dir)

	m := &Manager{
		store:    store,
		group:    &Group{ID: "listen-abc123"},
		queue:    []string{"/music/track1.mp3", "/music/track2.mp3"},
		queueIdx: 1,
	}

	m.saveQueueToDisk()

	m2 := &Manager{store: store}
	qs := m2.loadQueueFromDisk()
	if qs == nil {
		t.Fatal("expected queue state from disk")
	}
	if qs.GroupID != "listen-abc123" {
		t.Fatalf("group_id = %q, want 'listen-abc123'", qs.GroupID)
	}
	if len(qs.Paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(qs.Paths))
	}
	if qs.Index != 1 {
		t.Fatalf("index = %d, want 1", qs.Index)
	}
}

func TestQueuePersistenceNoStore(t *testing.T) {
	m := &Manager{}
	m.saveQueueToDisk()
	if qs := m.loadQueueFromDisk(); qs != nil {
		t.Fatal("expected nil with no store")
	}
}

func TestGenerateListenID(t *testing.T) {
	id := generateListenID()
	if len(id) < 10 {
		t.Fatalf("id too short: %q", id)
	}
	if id[:7] != "listen-" {
		t.Fatalf("expected 'listen-' prefix, got %q", id)
	}

	id2 := generateListenID()
	if id == id2 {
		t.Fatal("two generated IDs should differ")
	}
}
