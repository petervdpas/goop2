package listen

import (
	"testing"
	"time"
)

func TestOnCreate(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})

	if err := m.OnCreate("listen-abc", "My Room", 0); err != nil {
		t.Fatal(err)
	}

	g := m.GetGroup()
	if g == nil {
		t.Fatal("expected group after OnCreate")
	}
	if g.ID != "listen-abc" {
		t.Fatalf("id = %q", g.ID)
	}
	if g.Name != "My Room" {
		t.Fatalf("name = %q", g.Name)
	}
	if g.Role != "host" {
		t.Fatalf("role = %q", g.Role)
	}
}

func TestOnCreateAlreadyInGroup(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.OnCreate("listen-1", "Room", 0)

	if err := m.OnCreate("listen-2", "Room 2", 0); err == nil {
		t.Fatal("expected error when already in group")
	}
}

func TestOnClose(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.OnCreate("listen-abc", "Room", 0)

	m.OnClose("listen-abc")

	if g := m.GetGroup(); g != nil {
		t.Fatal("expected nil group after OnClose")
	}
}

func TestOnCloseWrongGroup(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.OnCreate("listen-abc", "Room", 0)

	m.OnClose("listen-other")

	if g := m.GetGroup(); g == nil {
		t.Fatal("should not close unrelated group")
	}
}

func TestClose(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.OnCreate("listen-abc", "Room", 0)

	m.Close()

	if g := m.GetGroup(); g != nil {
		t.Fatal("expected nil group after Close")
	}
}

func TestGetGroupNil(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	if g := m.GetGroup(); g != nil {
		t.Fatal("expected nil group")
	}
}

func TestGetGroupAdvancesPosition(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})

	now := time.Now().UnixMilli()
	m.SetTestGroupFull(&Group{
		ID:   "listen-abc",
		Name: "Room",
		Role: "host",
		Track: &Track{Name: "test.mp3", Duration: 300},
		PlayState: &PlayState{
			Playing:   true,
			Position:  10.0,
			UpdatedAt: now - 2000,
		},
	})

	g := m.GetGroup()
	if g.PlayState.Position < 11.5 {
		t.Fatalf("position should have advanced, got %.1f", g.PlayState.Position)
	}
}

func TestGetGroupPausedNoAdvance(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})

	now := time.Now().UnixMilli()
	m.SetTestGroupFull(&Group{
		ID:   "listen-abc",
		Role: "host",
		PlayState: &PlayState{
			Playing:   false,
			Position:  10.0,
			UpdatedAt: now - 5000,
		},
	})

	g := m.GetGroup()
	if g.PlayState.Position != 10.0 {
		t.Fatalf("paused position should not advance, got %.1f", g.PlayState.Position)
	}
}

func TestOnJoinAndOnLeaveDontPanic(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.OnJoin("listen-abc", "peer1", false)
	m.OnLeave("listen-abc", "peer1", false)
}

func TestCurrentPosition(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})

	if pos := m.currentPosition(); pos != 0 {
		t.Fatalf("nil group position = %f, want 0", pos)
	}

	now := time.Now().UnixMilli()
	m.SetTestGroupFull(&Group{
		ID:   "g",
		Role: "host",
		PlayState: &PlayState{
			Playing:   false,
			Position:  5.0,
			UpdatedAt: now - 3000,
		},
	})
	if pos := m.currentPosition(); pos != 5.0 {
		t.Fatalf("paused position = %f, want 5.0", pos)
	}

	m.group.PlayState.Playing = true
	pos := m.currentPosition()
	if pos < 7.5 {
		t.Fatalf("playing position should have advanced, got %f", pos)
	}
}
