package viewmodels

import (
	"testing"
	"time"

	"github.com/petervdpas/goop2/internal/state"
)

func TestBuildPeerRow(t *testing.T) {
	now := time.Now()
	sp := state.SeenPeer{
		Content:        "Alice",
		Email:          "alice@test.com",
		AvatarHash:     "abc123",
		VideoDisabled:  true,
		ActiveTemplate: "blog",
		PublicKey:      "pk-xyz",
		Verified:       true,
		Reachable:      true,
		LastSeen:       now,
		Favorite:       true,
	}

	row := BuildPeerRow("peer-1", sp)

	if row.ID != "peer-1" {
		t.Errorf("ID = %q", row.ID)
	}
	if row.Content != "Alice" {
		t.Errorf("Content = %q", row.Content)
	}
	if row.Email != "alice@test.com" {
		t.Errorf("Email = %q", row.Email)
	}
	if row.AvatarHash != "abc123" {
		t.Errorf("AvatarHash = %q", row.AvatarHash)
	}
	if !row.VideoDisabled {
		t.Error("VideoDisabled should be true")
	}
	if row.ActiveTemplate != "blog" {
		t.Errorf("ActiveTemplate = %q", row.ActiveTemplate)
	}
	if row.PublicKey != "pk-xyz" {
		t.Errorf("PublicKey = %q", row.PublicKey)
	}
	if !row.Verified {
		t.Error("Verified should be true")
	}
	if !row.Reachable {
		t.Error("Reachable should be true")
	}
	if row.Offline {
		t.Error("Offline should be false when OfflineSince is zero")
	}
	if !row.LastSeen.Equal(now) {
		t.Errorf("LastSeen = %v", row.LastSeen)
	}
	if !row.Favorite {
		t.Error("Favorite should be true")
	}
}

func TestBuildPeerRow_Offline(t *testing.T) {
	sp := state.SeenPeer{
		Content:      "Bob",
		OfflineSince: time.Now(),
	}
	row := BuildPeerRow("peer-2", sp)
	if !row.Offline {
		t.Error("Offline should be true when OfflineSince is non-zero")
	}
}

func TestBuildPeerRows_Empty(t *testing.T) {
	rows := BuildPeerRows(map[string]state.SeenPeer{})
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestBuildPeerRows_Sorted(t *testing.T) {
	m := map[string]state.SeenPeer{
		"charlie": {Content: "Charlie"},
		"alice":   {Content: "Alice"},
		"bob":     {Content: "Bob"},
	}
	rows := BuildPeerRows(m)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0].ID != "alice" || rows[1].ID != "bob" || rows[2].ID != "charlie" {
		t.Errorf("rows not sorted by ID: %s, %s, %s", rows[0].ID, rows[1].ID, rows[2].ID)
	}
}

func TestBuildPeerRows_MapsFields(t *testing.T) {
	m := map[string]state.SeenPeer{
		"p1": {Content: "Peer One", Verified: true},
	}
	rows := BuildPeerRows(m)
	if rows[0].Content != "Peer One" {
		t.Errorf("Content = %q", rows[0].Content)
	}
	if !rows[0].Verified {
		t.Error("Verified should be true")
	}
}
