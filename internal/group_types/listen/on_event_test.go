package listen

import (
	"testing"
	"time"

	"github.com/petervdpas/goop2/internal/group"
)

func TestOnEventIgnoresOwnMessages(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.SetTestGroupFull(&Group{ID: "g1", Role: "listener"})

	m.OnEvent(&group.Event{
		Group: "g1",
		From:  "me",
		Type:  "msg",
	})

	if m.GetGroup() == nil {
		t.Fatal("own messages should be ignored, group should still exist")
	}
}

func TestOnEventIgnoresWrongGroup(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.SetTestGroupFull(&Group{ID: "g1", Role: "listener"})

	m.OnEvent(&group.Event{
		Group: "g2",
		From:  "other",
		Type:  "msg",
	})

	if m.GetGroup() == nil {
		t.Fatal("events for wrong group should be ignored")
	}
}

func TestOnEventLeaveAsListener(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.SetTestGroupFull(&Group{ID: "g1", Role: "listener"})

	m.OnEvent(&group.Event{
		Group: "g1",
		From:  "host",
		Type:  "leave",
	})

	if m.GetGroup() != nil {
		t.Fatal("listener should leave group on leave event")
	}
}

func TestOnEventLeaveAsHostNoOp(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.SetTestGroupFull(&Group{ID: "g1", Role: "host"})

	m.OnEvent(&group.Event{
		Group: "g1",
		From:  "other",
		Type:  "leave",
	})

	if m.GetGroup() == nil {
		t.Fatal("host should not leave on leave event")
	}
}

func TestOnEventMsgRoutesToControlHandler(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.SetTestGroupFull(&Group{
		ID:   "g1",
		Role: "listener",
		PlayState: &PlayState{
			Playing:   false,
			Position:  0,
			UpdatedAt: time.Now().UnixMilli(),
		},
	})

	m.OnEvent(&group.Event{
		Group:   "g1",
		From:    "host",
		Type:    "msg",
		Payload: controlPayload("play", map[string]any{"position": 10.0}),
	})

	g := m.GetGroup()
	if !g.PlayState.Playing {
		t.Fatal("msg event should have triggered play")
	}
}

