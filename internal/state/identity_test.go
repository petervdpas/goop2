package state

import (
	"testing"
	"time"
)

func TestFromSeenPeer(t *testing.T) {
	sp := SeenPeer{
		Content:             "Alice",
		Email:               "alice@example.com",
		AvatarHash:          "abc123",
		Reachable:           true,
		Verified:            true,
		GoopClientVersion:   "2.4.52",
		PublicKey:           "pk-alice",
		EncryptionSupported: true,
		ActiveTemplate:      "blog",
		VideoDisabled:       false,
		LastSeen:            time.Now(),
	}

	pi := FromSeenPeer(sp)

	if pi.Name != "Alice" {
		t.Fatalf("expected Name 'Alice', got %q", pi.Name)
	}
	if pi.Email != "alice@example.com" {
		t.Fatalf("expected Email 'alice@example.com', got %q", pi.Email)
	}
	if pi.AvatarHash != "abc123" {
		t.Fatalf("expected AvatarHash 'abc123', got %q", pi.AvatarHash)
	}
	if !pi.Reachable {
		t.Fatal("expected Reachable true")
	}
	if !pi.Verified {
		t.Fatal("expected Verified true")
	}
	if pi.GoopClientVersion != "2.4.52" {
		t.Fatalf("expected GoopClientVersion '2.4.52', got %q", pi.GoopClientVersion)
	}
	if pi.PublicKey != "pk-alice" {
		t.Fatalf("expected PublicKey 'pk-alice', got %q", pi.PublicKey)
	}
	if !pi.EncryptionSupported {
		t.Fatal("expected EncryptionSupported true")
	}
	if pi.ActiveTemplate != "blog" {
		t.Fatalf("expected ActiveTemplate 'blog', got %q", pi.ActiveTemplate)
	}
	if pi.VideoDisabled {
		t.Fatal("expected VideoDisabled false")
	}
	if !pi.Known {
		t.Fatal("expected Known true")
	}
}

func TestFromSeenPeerEmptyContent(t *testing.T) {
	sp := SeenPeer{}
	pi := FromSeenPeer(sp)

	if pi.Name != "" {
		t.Fatalf("expected empty Name, got %q", pi.Name)
	}
	if !pi.Known {
		t.Fatal("expected Known true even with empty content")
	}
}

func TestPeerIdentityZeroValue(t *testing.T) {
	var pi PeerIdentity

	if pi.Known {
		t.Fatal("zero-value PeerIdentity should have Known=false")
	}
	if pi.Name != "" {
		t.Fatalf("zero-value should have empty Name, got %q", pi.Name)
	}
	if pi.Reachable {
		t.Fatal("zero-value should have Reachable=false")
	}
}

func TestFromSeenPeerPreservesAllFields(t *testing.T) {
	sp := SeenPeer{
		Content:             "Bob",
		Email:               "bob@test.com",
		AvatarHash:          "xyz",
		VideoDisabled:       true,
		ActiveTemplate:      "todo",
		PublicKey:           "pk-bob",
		EncryptionSupported: false,
		Verified:            false,
		GoopClientVersion:   "2.3.0",
		Reachable:           false,
	}

	pi := FromSeenPeer(sp)

	if pi.VideoDisabled != true {
		t.Fatal("VideoDisabled should be true")
	}
	if pi.EncryptionSupported != false {
		t.Fatal("EncryptionSupported should be false")
	}
	if pi.Verified != false {
		t.Fatal("Verified should be false")
	}
	if pi.Reachable != false {
		t.Fatal("Reachable should be false")
	}
}
