package state

import (
	"testing"
	"time"
)

func TestUpsert_NewPeer(t *testing.T) {
	pt := NewPeerTable()
	pt.Upsert("peer-1", "Alice", "alice@test.com", "hash1", false, "blog", "pk1", true, true, "2.4.0")

	sp, ok := pt.Get("peer-1")
	if !ok {
		t.Fatal("peer should exist after upsert")
	}
	if sp.Content != "Alice" {
		t.Fatalf("expected Content='Alice', got %q", sp.Content)
	}
	if sp.Email != "alice@test.com" {
		t.Fatalf("expected Email, got %q", sp.Email)
	}
	if sp.PublicKey != "pk1" {
		t.Fatalf("expected PublicKey='pk1', got %q", sp.PublicKey)
	}
	if !sp.EncryptionSupported {
		t.Fatal("expected EncryptionSupported=true")
	}
	if !sp.Verified {
		t.Fatal("expected Verified=true")
	}
	if sp.Reachable {
		t.Fatal("new peer should not be reachable until probed")
	}
}

func TestUpsert_PreservesLocalState(t *testing.T) {
	pt := NewPeerTable()
	pt.Upsert("peer-1", "Alice", "", "", false, "", "pk1", true, false, "2.4.0")
	pt.SetReachable("peer-1", true)
	pt.SetFavorite("peer-1", true)

	sp, _ := pt.Get("peer-1")
	if !sp.Reachable {
		t.Fatal("should be reachable after SetReachable(true)")
	}
	if !sp.Favorite {
		t.Fatal("should be favorite after SetFavorite(true)")
	}

	pt.Upsert("peer-1", "Alice Updated", "alice@new.com", "", false, "", "", false, false, "")

	sp, _ = pt.Get("peer-1")
	if sp.Content != "Alice Updated" {
		t.Fatalf("content should update, got %q", sp.Content)
	}
	if !sp.Reachable {
		t.Fatal("reachable should be preserved across upsert")
	}
	if !sp.Favorite {
		t.Fatal("favorite should be preserved across upsert")
	}
	if sp.PublicKey != "pk1" {
		t.Fatalf("public key should be preserved when incoming is empty, got %q", sp.PublicKey)
	}
	if !sp.EncryptionSupported {
		t.Fatal("encryption support should be preserved when incoming is false")
	}
}

func TestUpsert_ClearsOfflineSince(t *testing.T) {
	pt := NewPeerTable()
	pt.Seed("peer-1", "Alice", "", "", false, "", "", false, false)

	sp, _ := pt.Get("peer-1")
	if sp.OfflineSince.IsZero() {
		t.Fatal("seeded peer should have OfflineSince set")
	}

	pt.Upsert("peer-1", "Alice", "", "", false, "", "", false, false, "")

	sp, _ = pt.Get("peer-1")
	if !sp.OfflineSince.IsZero() {
		t.Fatal("upsert should clear OfflineSince (peer came back online)")
	}
}

func TestSeed_DoesNotOverwriteExisting(t *testing.T) {
	pt := NewPeerTable()
	pt.Upsert("peer-1", "Alice", "alice@real.com", "", false, "", "", false, false, "")

	pt.Seed("peer-1", "Old Alice", "old@email.com", "", false, "", "", false, false)

	sp, _ := pt.Get("peer-1")
	if sp.Content != "Alice" {
		t.Fatalf("seed should not overwrite existing peer, got %q", sp.Content)
	}
}

func TestSetReachable_Success(t *testing.T) {
	pt := NewPeerTable()
	pt.Upsert("peer-1", "Alice", "", "", false, "", "", false, false, "")

	pt.SetReachable("peer-1", true)

	sp, _ := pt.Get("peer-1")
	if !sp.Reachable {
		t.Fatal("should be reachable after success")
	}
}

func TestSetReachable_FailStreak(t *testing.T) {
	pt := NewPeerTable()
	pt.Upsert("peer-1", "Alice", "", "", false, "", "", false, false, "")
	pt.SetReachable("peer-1", true)

	sp, _ := pt.Get("peer-1")
	if !sp.Reachable {
		t.Fatal("should be reachable initially")
	}

	pt.SetReachable("peer-1", false)
	sp, _ = pt.Get("peer-1")
	if !sp.Reachable {
		t.Fatal("should still be reachable after 1 failure (need 2)")
	}

	pt.mu.Lock()
	p := pt.peers["peer-1"]
	p.lastFailAt = time.Now().Add(-3 * time.Second)
	pt.peers["peer-1"] = p
	pt.mu.Unlock()

	pt.SetReachable("peer-1", false)
	sp, _ = pt.Get("peer-1")
	if sp.Reachable {
		t.Fatal("should be unreachable after 2 distinct failures")
	}
}

func TestSetReachable_SuccessResetsStreak(t *testing.T) {
	pt := NewPeerTable()
	pt.Upsert("peer-1", "Alice", "", "", false, "", "", false, false, "")
	pt.SetReachable("peer-1", true)

	pt.SetReachable("peer-1", false)

	pt.SetReachable("peer-1", true)

	pt.mu.Lock()
	p := pt.peers["peer-1"]
	streak := p.failStreak
	pt.mu.Unlock()

	if streak != 0 {
		t.Fatalf("success should reset failStreak to 0, got %d", streak)
	}
}

func TestMarkOffline(t *testing.T) {
	pt := NewPeerTable()
	pt.Upsert("peer-1", "Alice", "", "", false, "", "", false, false, "")
	pt.SetReachable("peer-1", true)

	pt.MarkOffline("peer-1")

	sp, _ := pt.Get("peer-1")
	if sp.Reachable {
		t.Fatal("should not be reachable after MarkOffline")
	}
	if sp.OfflineSince.IsZero() {
		t.Fatal("OfflineSince should be set")
	}
}

func TestPruneStale_TTLMovesToOffline(t *testing.T) {
	pt := NewPeerTable()
	pt.Upsert("peer-1", "Alice", "", "", false, "", "", false, false, "")

	pt.mu.Lock()
	p := pt.peers["peer-1"]
	p.LastSeen = time.Now().Add(-30 * time.Second)
	pt.peers["peer-1"] = p
	pt.mu.Unlock()

	ttlCutoff := time.Now().Add(-20 * time.Second)
	graceCutoff := time.Now().Add(-10 * time.Minute)
	pt.PruneStale(ttlCutoff, graceCutoff)

	sp, ok := pt.Get("peer-1")
	if !ok {
		t.Fatal("peer should still exist (only moved to offline, not removed)")
	}
	if sp.OfflineSince.IsZero() {
		t.Fatal("should be marked offline after TTL expiry")
	}
}

func TestPruneStale_GraceRemovesPeer(t *testing.T) {
	pt := NewPeerTable()
	pt.Upsert("peer-1", "Alice", "", "", false, "", "", false, false, "")
	pt.MarkOffline("peer-1")

	pt.mu.Lock()
	p := pt.peers["peer-1"]
	p.OfflineSince = time.Now().Add(-20 * time.Minute)
	pt.peers["peer-1"] = p
	pt.mu.Unlock()

	ttlCutoff := time.Now().Add(-20 * time.Second)
	graceCutoff := time.Now().Add(-10 * time.Minute)
	pt.PruneStale(ttlCutoff, graceCutoff)

	_, ok := pt.Get("peer-1")
	if ok {
		t.Fatal("peer should be removed after grace period")
	}
}

func TestSubscribe_ReceivesEvents(t *testing.T) {
	pt := NewPeerTable()
	ch := pt.Subscribe()
	defer pt.Unsubscribe(ch)

	pt.Upsert("peer-1", "Alice", "", "", false, "", "", false, false, "")

	select {
	case evt := <-ch:
		if evt.Type != "update" {
			t.Fatalf("expected update event, got %q", evt.Type)
		}
		if evt.PeerID != "peer-1" {
			t.Fatalf("expected peer-1, got %q", evt.PeerID)
		}
	default:
		t.Fatal("should receive event after upsert")
	}
}

func TestRemove_BroadcastsEvent(t *testing.T) {
	pt := NewPeerTable()
	pt.Upsert("peer-1", "Alice", "", "", false, "", "", false, false, "")

	ch := pt.Subscribe()
	defer pt.Unsubscribe(ch)

	pt.Remove("peer-1")

	select {
	case evt := <-ch:
		if evt.Type != "remove" {
			t.Fatalf("expected remove event, got %q", evt.Type)
		}
	default:
		t.Fatal("should receive remove event")
	}

	_, ok := pt.Get("peer-1")
	if ok {
		t.Fatal("peer should be gone after Remove")
	}
}

func TestSnapshot_ReturnsCopy(t *testing.T) {
	pt := NewPeerTable()
	pt.Upsert("peer-1", "Alice", "", "", false, "", "", false, false, "")
	pt.Upsert("peer-2", "Bob", "", "", false, "", "", false, false, "")

	snap := pt.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 peers in snapshot, got %d", len(snap))
	}

	delete(snap, "peer-1")

	_, ok := pt.Get("peer-1")
	if !ok {
		t.Fatal("deleting from snapshot should not affect the table")
	}
}
