package chat

import (
	"testing"

	"github.com/petervdpas/goop2/internal/group"
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

	grpMgr := group.NewTestManager(db, "self-peer-id")
	t.Cleanup(func() { grpMgr.Close() })

	m := &Manager{
		grp:    grpMgr,
		selfID: "self-peer-id",
		peerName: func(id string) string {
			if id == "self-peer-id" {
				return "Self"
			}
			return id[:8]
		},
		rooms: make(map[string]*roomState),
	}
	grpMgr.RegisterType(GroupTypeName, m)
	return m
}

func TestFlags(t *testing.T) {
	m := &Manager{}
	if !m.Flags().HostCanJoin {
		t.Fatal("chat handler should allow host to join")
	}
}

func TestOnCreateAndClose(t *testing.T) {
	m := testManager(t)

	if err := m.OnCreate("room1", "Test Room", 0); err != nil {
		t.Fatal(err)
	}
	if len(m.rooms) != 1 {
		t.Fatalf("expected 1 room, got %d", len(m.rooms))
	}
	if m.rooms["room1"].info.Name != "Test Room" {
		t.Fatalf("expected name 'Test Room', got %q", m.rooms["room1"].info.Name)
	}

	m.OnClose("room1")
	if len(m.rooms) != 0 {
		t.Fatalf("expected 0 rooms after close, got %d", len(m.rooms))
	}
}

func TestSendMessage(t *testing.T) {
	m := testManager(t)
	_ = m.OnCreate("room1", "Test Room", 0)

	if err := m.SendMessage("room1", "self-peer-id", "hello"); err != nil {
		t.Fatal(err)
	}

	rs := m.rooms["room1"]
	rs.mu.RLock()
	msgs := rs.history.All()
	rs.mu.RUnlock()

	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Text != "hello" {
		t.Fatalf("expected text 'hello', got %q", msgs[0].Text)
	}
	if msgs[0].FromName != "Self" {
		t.Fatalf("expected from_name 'Self', got %q", msgs[0].FromName)
	}
}

func TestSendMessageUnknownRoom(t *testing.T) {
	m := testManager(t)

	err := m.SendMessage("nonexistent", "self-peer-id", "hello")
	if err == nil {
		t.Fatal("expected error for unknown room")
	}
}

func TestGetState(t *testing.T) {
	m := testManager(t)
	_ = m.OnCreate("room1", "Test Room", 0)
	_ = m.SendMessage("room1", "self-peer-id", "msg1")
	_ = m.SendMessage("room1", "self-peer-id", "msg2")

	room, msgs, err := m.GetState("room1")
	if err != nil {
		t.Fatal(err)
	}
	if room.Name != "Test Room" {
		t.Fatalf("expected name 'Test Room', got %q", room.Name)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestListRooms(t *testing.T) {
	m := testManager(t)
	_ = m.OnCreate("room1", "Room A", 0)
	_ = m.OnCreate("room2", "Room B", 0)

	rooms := m.ListRooms()
	if len(rooms) != 2 {
		t.Fatalf("expected 2 rooms, got %d", len(rooms))
	}
}
