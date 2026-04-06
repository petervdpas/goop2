package chat

import (
	"testing"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
)

type testManagerOpts struct {
	selfID      string
	resolvePeer func(string) state.PeerIdentityPayload
}

func testManager(t *testing.T, opts ...testManagerOpts) (*Manager, *group.Manager) {
	t.Helper()

	var o testManagerOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.selfID == "" {
		o.selfID = "self-peer-id"
	}
	if o.resolvePeer == nil {
		o.resolvePeer = func(id string) state.PeerIdentityPayload {
			if id == o.selfID {
				return state.PeerIdentityPayload{Content: "Self", Known: true}
			}
			return state.PeerIdentityPayload{Content: id, Known: true}
		}
	}

	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	grpMgr := group.NewTestManager(db, o.selfID)
	t.Cleanup(func() { grpMgr.Close() })

	m := NewTestManager(grpMgr, o.selfID, o.resolvePeer)
	return m, grpMgr
}

func TestFlags(t *testing.T) {
	m := &Manager{}
	if !m.Flags().HostCanJoin {
		t.Fatal("chat handler should allow host to join")
	}
}

func TestOnCreateAndClose(t *testing.T) {
	m, _ := testManager(t)

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
	m, _ := testManager(t)
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
	m, _ := testManager(t)

	err := m.SendMessage("nonexistent", "self-peer-id", "hello")
	if err == nil {
		t.Fatal("expected error for unknown room")
	}
}

func TestGetState(t *testing.T) {
	m, _ := testManager(t)
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
	m, _ := testManager(t)
	_ = m.OnCreate("room1", "Room A", 0)
	_ = m.OnCreate("room2", "Room B", 0)

	rooms := m.ListRooms()
	if len(rooms) != 2 {
		t.Fatalf("expected 2 rooms, got %d", len(rooms))
	}
}

func TestResolveMembersUsesPeerNameFallback(t *testing.T) {
	m, grpMgr := testManager(t, testManagerOpts{
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
	})

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
	m, _ := testManager(t)
	_ = m.OnCreate("room1", "Test Room", 0)

	_ = m.SendMessage("room1", "self-peer-id", "hello")

	_, msgs, _ := m.GetState("room1")
	if msgs[0].FromName != "Self" {
		t.Fatalf("expected FromName 'Self', got %q", msgs[0].FromName)
	}
}
