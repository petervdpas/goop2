package group

import (
	"fmt"
	"testing"

	"github.com/petervdpas/goop2/internal/storage"
)

func openTestDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func hostManager(t *testing.T, db *storage.DB) *Manager {
	t.Helper()
	m := NewTestManager(db, "host-peer-id")
	t.Cleanup(func() { m.Close() })
	return m
}

// ── Scenario: Host creates a group ─────────────────────────────────────────

func TestScenario_HostCreatesGroup(t *testing.T) {
	// Given a host peer
	db := openTestDB(t)
	host := hostManager(t, db)

	// When the host creates a template group
	err := host.CreateGroup("g1", "Blog Co-authors", "template", "blog", 0)

	// Then the group exists in DB and in memory
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	g, err := db.GetGroup("g1")
	if err != nil {
		t.Fatalf("group not in DB: %v", err)
	}
	if g.Name != "Blog Co-authors" {
		t.Fatalf("expected name 'Blog Co-authors', got %q", g.Name)
	}
	if g.GroupType != "template" {
		t.Fatalf("expected type 'template', got %q", g.GroupType)
	}
	if g.GroupContext != "blog" {
		t.Fatalf("expected context 'blog', got %q", g.GroupContext)
	}

	host.mu.RLock()
	_, inMemory := host.groups["g1"]
	host.mu.RUnlock()
	if !inMemory {
		t.Fatal("group should be in memory")
	}
}

// ── Scenario: Host creates and closes a group ──────────────────────────────

func TestScenario_HostClosesGroup(t *testing.T) {
	// Given a host with a group
	db := openTestDB(t)
	host := hostManager(t, db)
	_ = host.CreateGroup("g1", "Test", "template", "", 0)

	// When the host closes the group
	err := host.CloseGroup("g1")

	// Then the group is removed from memory and DB
	if err != nil {
		t.Fatalf("close failed: %v", err)
	}
	host.mu.RLock()
	_, exists := host.groups["g1"]
	host.mu.RUnlock()
	if exists {
		t.Fatal("group should be removed from memory")
	}
	groups, _ := db.ListGroups()
	for _, g := range groups {
		if g.ID == "g1" {
			t.Fatal("group should be removed from DB")
		}
	}
}

// ── Scenario: Host joins own group ─────────────────────────────────────────

func TestScenario_HostJoinsOwnGroup(t *testing.T) {
	// Given a host with a group
	db := openTestDB(t)
	host := hostManager(t, db)
	_ = host.CreateGroup("g1", "Test", "template", "", 0)

	// When the host joins the group
	err := host.JoinOwnGroup("g1")

	// Then the host appears in the member list as owner
	if err != nil {
		t.Fatalf("join failed: %v", err)
	}
	members := host.HostedGroupMembers("g1")
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0].PeerID != "host-peer-id" {
		t.Fatalf("expected host peer ID, got %q", members[0].PeerID)
	}
	if members[0].Role != "owner" {
		t.Fatalf("expected role 'owner', got %q", members[0].Role)
	}
}

// ── Scenario: Remote peer joins a hosted group ─────────────────────────────

func TestScenario_RemotePeerJoins(t *testing.T) {
	// Given a host with a group that the host has joined
	db := openTestDB(t)
	host := hostManager(t, db)
	_ = host.CreateGroup("g1", "Test", "template", "", 0)
	_ = host.JoinOwnGroup("g1")

	// When a remote peer joins
	host.mu.RLock()
	hg := host.groups["g1"]
	host.mu.RUnlock()
	host.handleHostMessage("remote-peer", hg, "g1", TypeJoin, nil)

	// Then the member list has 2 members
	members := host.HostedGroupMembers("g1")
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	// And the remote peer has the default role
	var remoteMember *MemberInfo
	for _, m := range members {
		if m.PeerID == "remote-peer" {
			remoteMember = &m
			break
		}
	}
	if remoteMember == nil {
		t.Fatal("remote peer should be in member list")
	}
	if remoteMember.Role != "viewer" {
		t.Fatalf("expected default role 'viewer', got %q", remoteMember.Role)
	}
}

// ── Scenario: Remote peer leaves a hosted group ────────────────────────────

func TestScenario_RemotePeerLeaves(t *testing.T) {
	// Given a host with a group and a remote member
	db := openTestDB(t)
	host := hostManager(t, db)
	_ = host.CreateGroup("g1", "Test", "template", "", 0)
	_ = host.JoinOwnGroup("g1")
	host.mu.RLock()
	hg := host.groups["g1"]
	host.mu.RUnlock()
	host.handleHostMessage("remote-peer", hg, "g1", TypeJoin, nil)

	// When the remote peer leaves
	host.handleHostMessage("remote-peer", hg, "g1", TypeLeave, nil)

	// Then only the host remains
	members := host.HostedGroupMembers("g1")
	if len(members) != 1 {
		t.Fatalf("expected 1 member after leave, got %d", len(members))
	}
}

// ── Scenario: Group rejects join when full ─────────────────────────────────

func TestScenario_GroupFull_RejectsJoin(t *testing.T) {
	// Given a group with max 2 members, host + 1 remote already in
	db := openTestDB(t)
	host := hostManager(t, db)
	_ = host.CreateGroup("g1", "Test", "template", "", 2)
	_ = host.JoinOwnGroup("g1")
	host.mu.RLock()
	hg := host.groups["g1"]
	host.mu.RUnlock()
	host.handleHostMessage("peer-a", hg, "g1", TypeJoin, nil)

	// When another peer tries to join
	host.handleHostMessage("peer-b", hg, "g1", TypeJoin, nil)

	// Then peer-b is NOT added (group is full)
	members := host.HostedGroupMembers("g1")
	if len(members) != 2 {
		t.Fatalf("expected 2 members (full), got %d", len(members))
	}
	for _, m := range members {
		if m.PeerID == "peer-b" {
			t.Fatal("peer-b should not be in the group")
		}
	}
}

// ── Scenario: Invite creates a subscription ────────────────────────────────

func TestScenario_InviteCreatesSubscription(t *testing.T) {
	// Given a remote peer
	db := openTestDB(t)
	remote := hostManager(t, db)

	// When an invite arrives from a host
	remote.handleInvite("host-abc", map[string]any{
		"group_id":    "g1",
		"group_name":  "Blog Co-authors",
		"host_peer_id": "host-abc",
		"group_type":  "template",
	})

	// Then a subscription is stored in the DB
	subs, _ := db.ListSubscriptions()
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}
	if subs[0].GroupName != "Blog Co-authors" {
		t.Fatalf("expected name 'Blog Co-authors', got %q", subs[0].GroupName)
	}
	if subs[0].HostPeerID != "host-abc" {
		t.Fatalf("expected host 'host-abc', got %q", subs[0].HostPeerID)
	}
	if subs[0].Volatile {
		t.Fatal("template subscription should not be volatile")
	}
}

// ── Scenario: Non-volatile invite preserves existing subscriptions ─────────

func TestScenario_NonVolatileInvite_PreservesOtherSubs(t *testing.T) {
	// Given a remote peer with an existing subscription
	db := openTestDB(t)
	remote := hostManager(t, db)
	_ = db.AddSubscription("host-a", "g-existing", "Kanban", "template", 0, false, "member", "HostA")

	// When a new non-volatile invite arrives
	remote.handleInvite("host-b", map[string]any{
		"group_id":    "g-new",
		"group_name":  "Chat Room",
		"host_peer_id": "host-b",
		"group_type":  "template",
	})

	// Then both subscriptions exist
	subs, _ := db.ListSubscriptions()
	if len(subs) != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", len(subs))
	}
}

// ── Scenario: Volatile invite wipes same-type subscriptions ────────────────

func TestScenario_VolatileInvite_WipesSameType(t *testing.T) {
	// Given a volatile type handler and an existing volatile subscription
	db := openTestDB(t)
	remote := hostManager(t, db)
	remote.RegisterType("cluster", &testVolatileHandler{})
	_ = db.AddSubscription("host-a", "g-old", "Old Cluster", "cluster", 0, true, "member", "HostA")

	// When a new volatile invite of the same type arrives
	remote.handleInvite("host-b", map[string]any{
		"group_id":    "g-new",
		"group_name":  "New Cluster",
		"host_peer_id": "host-b",
		"group_type":  "cluster",
	})

	// Then the old subscription is wiped and only the new one remains
	subs, _ := db.ListSubscriptions()
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription (old wiped), got %d", len(subs))
	}
	if subs[0].GroupID != "g-new" {
		t.Fatalf("expected g-new, got %q", subs[0].GroupID)
	}
}

// ── Scenario: Volatile invite does NOT wipe different types ────────────────

func TestScenario_VolatileInvite_PreservesDifferentType(t *testing.T) {
	// Given a template subscription and a volatile type handler
	db := openTestDB(t)
	remote := hostManager(t, db)
	remote.RegisterType("cluster", &testVolatileHandler{})
	_ = db.AddSubscription("host-a", "g-template", "Blog", "template", 0, false, "member", "HostA")

	// When a volatile invite of a different type arrives
	remote.handleInvite("host-b", map[string]any{
		"group_id":    "g-cluster",
		"group_name":  "Compute",
		"host_peer_id": "host-b",
		"group_type":  "cluster",
	})

	// Then both subscriptions exist
	subs, _ := db.ListSubscriptions()
	if len(subs) != 2 {
		t.Fatalf("expected 2 subscriptions (different types), got %d", len(subs))
	}
}

// ── Scenario: Host close removes client subscription ───────────────────────

func TestScenario_HostClose_RemovesClientSubscription(t *testing.T) {
	// Given a remote peer with a subscription and active connection
	db := openTestDB(t)
	remote := hostManager(t, db)
	_ = db.AddSubscription("host-abc", "g1", "Blog", "template", 0, false, "member", "Host")
	remote.mu.Lock()
	remote.activeConns["g1"] = &clientConn{
		hostPeerID: "host-abc",
		groupID:    "g1",
		groupType:  "template",
	}
	remote.mu.Unlock()

	// When the host sends a close message
	remote.handleMemberMessage("host-abc", remote.activeConns["g1"], "g1", TypeClose, nil)

	// Then the subscription is removed
	subs, _ := db.ListSubscriptions()
	if len(subs) != 0 {
		t.Fatalf("expected 0 subscriptions after close, got %d", len(subs))
	}

	// And the active connection is removed
	remote.mu.RLock()
	_, connected := remote.activeConns["g1"]
	remote.mu.RUnlock()
	if connected {
		t.Fatal("active connection should be removed after close")
	}
}

// ── Scenario: Subscription survives failed auto-join ───────────────────────

func TestScenario_SubscriptionSurvivesFailedAutoJoin(t *testing.T) {
	// Given a remote peer
	db := openTestDB(t)
	remote := hostManager(t, db)

	// When an invite arrives (auto-join will fail because no MQ transport)
	remote.handleInvite("host-abc", map[string]any{
		"group_id":    "g1",
		"group_name":  "Blog Co-authors",
		"host_peer_id": "host-abc",
		"group_type":  "template",
	})

	// Then the subscription from the invite still exists despite join failure
	subs, _ := db.ListSubscriptions()
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription (should persist despite failed join), got %d", len(subs))
	}
}

// ── Scenario: AddSubscription overwrites with INSERT OR REPLACE ────────────

func TestScenario_JoinOverwritesInviteSubscription(t *testing.T) {
	// Given an invite-created subscription with minimal data
	db := openTestDB(t)
	_ = db.AddSubscription("host-abc", "g1", "Blog", "template", 0, false, "member", "Host")

	// When the join completes and stores richer data
	_ = db.AddSubscription("host-abc", "g1", "Blog Co-authors", "template", 10, false, "coauthor", "HostName")

	// Then only one subscription exists with the updated data
	subs, _ := db.ListSubscriptions()
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}
	if subs[0].GroupName != "Blog Co-authors" {
		t.Fatalf("expected updated name, got %q", subs[0].GroupName)
	}
	if subs[0].Role != "coauthor" {
		t.Fatalf("expected updated role, got %q", subs[0].Role)
	}
	if subs[0].MaxMembers != 10 {
		t.Fatalf("expected max_members 10, got %d", subs[0].MaxMembers)
	}
}

// ── Scenario: RemoveSubscription only affects targeted group ───────────────

func TestScenario_RemoveSubscription_OnlyTargeted(t *testing.T) {
	// Given two subscriptions
	db := openTestDB(t)
	_ = db.AddSubscription("host-a", "g1", "Group A", "template", 0, false, "member", "HostA")
	_ = db.AddSubscription("host-b", "g2", "Group B", "template", 0, false, "member", "HostB")

	// When one is removed
	_ = db.RemoveSubscription("host-a", "g1")

	// Then the other remains
	subs, _ := db.ListSubscriptions()
	if len(subs) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(subs))
	}
	if subs[0].GroupID != "g2" {
		t.Fatalf("expected g2 to remain, got %q", subs[0].GroupID)
	}
}

// ── Scenario: isRejection classification ───────────────────────────────────

func TestScenario_IsRejection_Timeout_NotRejection(t *testing.T) {
	if isRejection(fmt.Errorf("timed out waiting for welcome")) {
		t.Fatal("timeout should NOT be a rejection")
	}
}

func TestScenario_IsRejection_DialFailure_NotRejection(t *testing.T) {
	if isRejection(fmt.Errorf("failed to dial peer: connection refused")) {
		t.Fatal("dial failure should NOT be a rejection")
	}
}

func TestScenario_IsRejection_StreamFailure_NotRejection(t *testing.T) {
	if isRejection(fmt.Errorf("failed to open stream")) {
		t.Fatal("stream failure should NOT be a rejection")
	}
}

func TestScenario_IsRejection_GroupNotFound_IsRejection(t *testing.T) {
	if !isRejection(fmt.Errorf("group not found")) {
		t.Fatal("group not found SHOULD be a rejection")
	}
}

func TestScenario_IsRejection_GroupFull_IsRejection(t *testing.T) {
	if !isRejection(fmt.Errorf("group is full")) {
		t.Fatal("group full SHOULD be a rejection")
	}
}

// ── Test helpers ───────────────────────────────────────────────────────────

type testVolatileHandler struct{}

func (h *testVolatileHandler) Flags() GroupTypeFlags {
	return GroupTypeFlags{HostCanJoin: false, Volatile: true}
}
func (h *testVolatileHandler) OnCreate(_, _ string, _ int) error { return nil }
func (h *testVolatileHandler) OnJoin(_, _ string, _ bool)        {}
func (h *testVolatileHandler) OnLeave(_, _ string, _ bool)       {}
func (h *testVolatileHandler) OnClose(_ string)                  {}
func (h *testVolatileHandler) OnEvent(_ *Event)                  {}
