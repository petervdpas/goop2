package listen

import (
	"maps"
	"testing"
	"time"
)

func controlPayload(action string, extra map[string]any) any {
	ctrl := map[string]any{"action": action}
	maps.Copy(ctrl, extra)
	return map[string]any{"listen": ctrl}
}

func testManagerWithGroup(t *testing.T) *Manager {
	t.Helper()
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.SetTestGroupFull(&Group{
		ID:   "listen-abc",
		Name: "Room",
		Role: "listener",
	})
	return m
}

func TestHandleControlEventLoad(t *testing.T) {
	m := testManagerWithGroup(t)

	payload := controlPayload("load", map[string]any{
		"track": map[string]any{
			"name":     "song.mp3",
			"duration": 180.0,
			"bitrate":  128000,
			"format":   "mp3",
		},
		"queue":       []any{"song.mp3", "next.mp3"},
		"queue_types": []any{"file", "file"},
		"queue_index": 0,
		"queue_total": 2,
	})
	m.handleControlEvent(payload)

	g := m.GetGroup()
	if g.Track == nil {
		t.Fatal("expected track after load")
	}
	if g.Track.Name != "song.mp3" {
		t.Fatalf("track name = %q", g.Track.Name)
	}
	if g.PlayState == nil || g.PlayState.Playing {
		t.Fatal("expected paused play state after load")
	}
	if g.QueueTotal != 2 {
		t.Fatalf("queue_total = %d, want 2", g.QueueTotal)
	}
}

func TestHandleControlEventPlay(t *testing.T) {
	m := testManagerWithGroup(t)
	m.group.PlayState = &PlayState{Playing: false, Position: 0, UpdatedAt: time.Now().UnixMilli()}

	m.handleControlEvent(controlPayload("play", map[string]any{"position": 5.5}))

	g := m.GetGroup()
	if !g.PlayState.Playing {
		t.Fatal("expected playing after play event")
	}
}

func TestHandleControlEventPause(t *testing.T) {
	m := testManagerWithGroup(t)
	m.group.PlayState = &PlayState{Playing: true, Position: 10.0, UpdatedAt: time.Now().UnixMilli()}

	m.handleControlEvent(controlPayload("pause", map[string]any{"position": 15.0}))

	g := m.GetGroup()
	if g.PlayState.Playing {
		t.Fatal("expected paused after pause event")
	}
	if g.PlayState.Position != 15.0 {
		t.Fatalf("position = %f, want 15.0", g.PlayState.Position)
	}
}

func TestHandleControlEventSeek(t *testing.T) {
	m := testManagerWithGroup(t)
	m.group.PlayState = &PlayState{Playing: true, Position: 10.0, UpdatedAt: time.Now().UnixMilli()}

	m.handleControlEvent(controlPayload("seek", map[string]any{"position": 30.0}))

	g := m.GetGroup()
	if !g.PlayState.Playing {
		t.Fatal("seek should preserve playing state")
	}
}

func TestHandleControlEventSync(t *testing.T) {
	m := testManagerWithGroup(t)
	m.group.Track = &Track{Name: "old.mp3"}
	m.group.PlayState = &PlayState{Playing: false, Position: 0, UpdatedAt: time.Now().UnixMilli()}

	m.handleControlEvent(controlPayload("sync", map[string]any{
		"position": 42.0,
		"track": map[string]any{
			"name":     "new.mp3",
			"duration": 200.0,
			"bitrate":  256000,
			"format":   "mp3",
		},
		"queue_total": 3,
		"queue":       []any{"a", "b", "c"},
		"queue_types": []any{"file", "file", "stream"},
		"queue_index": 1,
	}))

	g := m.GetGroup()
	if g.Track.Name != "new.mp3" {
		t.Fatalf("sync should update track, got %q", g.Track.Name)
	}
	if !g.PlayState.Playing {
		t.Fatal("sync should set playing=true")
	}
	if g.QueueTotal != 3 {
		t.Fatalf("queue_total = %d, want 3", g.QueueTotal)
	}
}

func TestHandleControlEventClose(t *testing.T) {
	m := testManagerWithGroup(t)

	m.handleControlEvent(controlPayload("close", nil))

	if m.GetGroup() != nil {
		t.Fatal("expected nil group after close control")
	}
}

func TestHandleControlEventNilGroup(t *testing.T) {
	m := NewTestManagerOpts(TestManagerOpts{SelfID: "me"})
	m.handleControlEvent(controlPayload("play", map[string]any{"position": 0.0}))
}

func TestHandleControlEventInvalidPayload(t *testing.T) {
	m := testManagerWithGroup(t)
	m.handleControlEvent("not a map")
	m.handleControlEvent(map[string]any{"wrong_key": "data"})
}
