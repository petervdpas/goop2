package storage

import (
	"testing"
)

func TestUpsertAndGetCachedPeer(t *testing.T) {
	db := testDB(t)

	p := CachedPeer{
		PeerID:         "peer1",
		Content:        "Alice",
		Email:          "alice@example.com",
		AvatarHash:     "abc123",
		VideoDisabled:  true,
		ActiveTemplate: "blog",
		Verified:       true,
		PublicKey:       "pk1",
		Addrs:          []string{"/ip4/1.2.3.4/tcp/4001"},
	}
	if err := db.UpsertCachedPeer(p); err != nil {
		t.Fatal(err)
	}

	got, ok := db.GetCachedPeer("peer1")
	if !ok {
		t.Fatal("expected to find peer1")
	}
	if got.Content != "Alice" {
		t.Fatalf("content = %q, want 'Alice'", got.Content)
	}
	if got.Email != "alice@example.com" {
		t.Fatalf("email = %q", got.Email)
	}
	if !got.VideoDisabled {
		t.Fatal("expected VideoDisabled = true")
	}
	if !got.Verified {
		t.Fatal("expected Verified = true")
	}
	if got.PublicKey != "pk1" {
		t.Fatalf("public_key = %q", got.PublicKey)
	}
	if got.ActiveTemplate != "blog" {
		t.Fatalf("active_template = %q", got.ActiveTemplate)
	}
	if len(got.Addrs) != 1 || got.Addrs[0] != "/ip4/1.2.3.4/tcp/4001" {
		t.Fatalf("addrs = %v", got.Addrs)
	}
	if got.Favorite {
		t.Fatal("should not be favorite yet")
	}
}

func TestGetCachedPeerMissing(t *testing.T) {
	db := testDB(t)

	_, ok := db.GetCachedPeer("nonexistent")
	if ok {
		t.Fatal("should not find nonexistent peer")
	}
}

func TestUpsertCachedPeerUpdate(t *testing.T) {
	db := testDB(t)

	db.UpsertCachedPeer(CachedPeer{PeerID: "p1", Content: "Old"})
	db.UpsertCachedPeer(CachedPeer{PeerID: "p1", Content: "New"})

	got, _ := db.GetCachedPeer("p1")
	if got.Content != "New" {
		t.Fatalf("content = %q, want 'New'", got.Content)
	}
}

func TestUpsertCachedPeerPreservesAddrsWhenEmptyList(t *testing.T) {
	db := testDB(t)

	db.UpsertCachedPeer(CachedPeer{
		PeerID: "p1",
		Addrs:  []string{"/ip4/1.2.3.4/tcp/4001"},
	})

	db.UpsertCachedPeer(CachedPeer{
		PeerID:  "p1",
		Content: "Updated",
		Addrs:   []string{},
	})

	got, _ := db.GetCachedPeer("p1")
	if len(got.Addrs) != 1 {
		t.Fatalf("addrs should be preserved when update sends empty list, got %v", got.Addrs)
	}
}

func TestDeleteCachedPeer(t *testing.T) {
	db := testDB(t)

	db.UpsertCachedPeer(CachedPeer{PeerID: "p1", Content: "Alice"})

	if err := db.DeleteCachedPeer("p1"); err != nil {
		t.Fatal(err)
	}

	_, ok := db.GetCachedPeer("p1")
	if ok {
		t.Fatal("peer should be deleted")
	}
}

func TestGetPeerName(t *testing.T) {
	db := testDB(t)

	if name := db.GetPeerName("nobody"); name != "" {
		t.Fatalf("expected empty, got %q", name)
	}

	db.UpsertCachedPeer(CachedPeer{PeerID: "p1", Content: "Alice"})
	if name := db.GetPeerName("p1"); name != "Alice" {
		t.Fatalf("name = %q, want 'Alice'", name)
	}
}

func TestListCachedPeers(t *testing.T) {
	db := testDB(t)

	db.UpsertCachedPeer(CachedPeer{PeerID: "p1", Content: "Alice"})
	db.UpsertCachedPeer(CachedPeer{PeerID: "p2", Content: "Bob"})

	peers, err := db.ListCachedPeers()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers))
	}
}

func TestListCachedPeersEmpty(t *testing.T) {
	db := testDB(t)

	peers, err := db.ListCachedPeers()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 0 {
		t.Fatalf("expected 0 peers, got %d", len(peers))
	}
}

func TestSetFavorite(t *testing.T) {
	db := testDB(t)

	db.UpsertCachedPeer(CachedPeer{PeerID: "p1", Content: "Alice", Email: "a@b.com"})

	if err := db.SetFavorite("p1", true); err != nil {
		t.Fatal(err)
	}

	got, ok := db.GetCachedPeer("p1")
	if !ok {
		t.Fatal("expected to find peer")
	}
	if !got.Favorite {
		t.Fatal("expected Favorite = true after SetFavorite(true)")
	}

	if err := db.SetFavorite("p1", false); err != nil {
		t.Fatal(err)
	}

	got, _ = db.GetCachedPeer("p1")
	if got.Favorite {
		t.Fatal("expected Favorite = false after SetFavorite(false)")
	}
}

func TestFavoriteSurvivesPeerDeletion(t *testing.T) {
	db := testDB(t)

	db.UpsertCachedPeer(CachedPeer{PeerID: "p1", Content: "Alice"})
	db.SetFavorite("p1", true)
	db.DeleteCachedPeer("p1")

	got, ok := db.GetCachedPeer("p1")
	if !ok {
		t.Fatal("favorited peer should still be findable via _favorites after cache deletion")
	}
	if got.Content != "Alice" {
		t.Fatalf("content = %q, want 'Alice'", got.Content)
	}
	if !got.Favorite {
		t.Fatal("should still be favorite")
	}
}

func TestListCachedPeersIncludesOfflineFavorites(t *testing.T) {
	db := testDB(t)

	db.UpsertCachedPeer(CachedPeer{PeerID: "p1", Content: "Alice"})
	db.UpsertCachedPeer(CachedPeer{PeerID: "p2", Content: "Bob"})
	db.SetFavorite("p1", true)
	db.DeleteCachedPeer("p1")

	peers, err := db.ListCachedPeers()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers (1 cached + 1 offline fav), got %d", len(peers))
	}
	names := map[string]bool{}
	for _, p := range peers {
		names[p.Content] = true
	}
	if !names["Alice"] || !names["Bob"] {
		t.Fatalf("expected both Alice and Bob, got %v", names)
	}
}

func TestUpsertPeerProtocols(t *testing.T) {
	db := testDB(t)

	db.UpsertCachedPeer(CachedPeer{PeerID: "p1", Content: "Alice"})

	protos := []string{"/goop/mq/1.0.0", "/goop/data/1.0.0"}
	if err := db.UpsertPeerProtocols("p1", protos); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetCachedPeer("p1")
	if len(got.Protocols) != 2 {
		t.Fatalf("expected 2 protocols, got %d", len(got.Protocols))
	}
	if got.Protocols[0] != "/goop/mq/1.0.0" {
		t.Fatalf("protocol[0] = %q", got.Protocols[0])
	}
}

func TestUpsertPeerProtocolsMirrorsToFavorites(t *testing.T) {
	db := testDB(t)

	db.UpsertCachedPeer(CachedPeer{PeerID: "p1", Content: "Alice"})
	db.SetFavorite("p1", true)
	db.UpsertPeerProtocols("p1", []string{"/goop/mq/1.0.0"})

	db.DeleteCachedPeer("p1")

	got, ok := db.GetCachedPeer("p1")
	if !ok {
		t.Fatal("should find via favorites")
	}
	if len(got.Protocols) != 1 {
		t.Fatalf("expected 1 protocol in favorites, got %d", len(got.Protocols))
	}
}

func TestUpsertCachedPeerMirrorsFavoriteMetadata(t *testing.T) {
	db := testDB(t)

	db.UpsertCachedPeer(CachedPeer{PeerID: "p1", Content: "Alice"})
	db.SetFavorite("p1", true)

	db.UpsertCachedPeer(CachedPeer{PeerID: "p1", Content: "Alice Updated", Email: "new@example.com"})

	db.DeleteCachedPeer("p1")

	got, ok := db.GetCachedPeer("p1")
	if !ok {
		t.Fatal("should find via favorites")
	}
	if got.Content != "Alice Updated" {
		t.Fatalf("favorites content = %q, want 'Alice Updated'", got.Content)
	}
	if got.Email != "new@example.com" {
		t.Fatalf("favorites email = %q, want 'new@example.com'", got.Email)
	}
}
