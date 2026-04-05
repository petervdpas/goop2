package chat

import (
	"testing"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/state"
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
		resolvePeer: func(id string) state.PeerIdentityPayload {
			if id == "self-peer-id" {
				return state.PeerIdentityPayload{Content: "Self", Known: true}
			}
			return state.PeerIdentityPayload{Content: id, Known: true}
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

func TestResolveMembersUsesPeerNameFallback(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	grpMgr := group.NewTestManager(db, "joiner-peer")
	t.Cleanup(func() { grpMgr.Close() })

	m := &Manager{
		grp:    grpMgr,
		selfID: "joiner-peer",
		resolvePeer: func(id string) state.PeerIdentityPayload {
			switch id {
			case "joiner-peer":
				return state.PeerIdentityPayload{Content: "Joiner", Known: true}
			case "host-peer-id":
				return state.PeerIdentityPayload{Content: "HostName", Known: true}
			}
			return state.PeerIdentityPayload{}
		},
		rooms: make(map[string]*roomState),
	}

	grpMgr.SetActiveConn("room1", "host-peer-id", GroupTypeName)
	grpMgr.SetActiveConnMembers("room1", []group.MemberInfo{
		{PeerID: "host-peer-id", Name: ""},
		{PeerID: "joiner-peer", Name: ""},
	})
	m.RegisterJoinedRoom("room1", "Test Room")

	room, _, err := m.GetState("room1")
	if err != nil {
		t.Fatal(err)
	}

	if len(room.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(room.Members))
	}

	names := map[string]string{}
	for _, mem := range room.Members {
		if mem.Name == "" {
			t.Fatalf("member %s has empty name — peerName fallback not working", mem.PeerID)
		}
		names[mem.PeerID] = mem.Name
	}
	if names["host-peer-id"] != "HostName" {
		t.Fatalf("expected host name 'HostName', got %q", names["host-peer-id"])
	}
	if names["joiner-peer"] != "Joiner" {
		t.Fatalf("expected joiner name 'Joiner', got %q", names["joiner-peer"])
	}
}

func TestSendMessageFromNameResolved(t *testing.T) {
	m := testManager(t)
	_ = m.OnCreate("room1", "Test Room", 0)

	_ = m.SendMessage("room1", "self-peer-id", "hello")

	_, msgs, _ := m.GetState("room1")
	if msgs[0].FromName != "Self" {
		t.Fatalf("expected FromName 'Self', got %q", msgs[0].FromName)
	}
}
