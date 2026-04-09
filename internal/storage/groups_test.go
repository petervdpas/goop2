package storage

import (
	"testing"
)

func TestCreateAndGetGroup(t *testing.T) {
	db := testDB(t)

	if err := db.CreateGroup("g1", "Test Group", "owner1", "template", "blog", 10, false); err != nil {
		t.Fatal(err)
	}

	g, err := db.GetGroup("g1")
	if err != nil {
		t.Fatal(err)
	}
	if g.ID != "g1" {
		t.Fatalf("id = %q", g.ID)
	}
	if g.Name != "Test Group" {
		t.Fatalf("name = %q", g.Name)
	}
	if g.Owner != "owner1" {
		t.Fatalf("owner = %q", g.Owner)
	}
	if g.GroupType != "template" {
		t.Fatalf("group_type = %q", g.GroupType)
	}
	if g.GroupContext != "blog" {
		t.Fatalf("group_context = %q", g.GroupContext)
	}
	if g.MaxMembers != 10 {
		t.Fatalf("max_members = %d", g.MaxMembers)
	}
	if g.Volatile {
		t.Fatal("should not be volatile")
	}
}

func TestCreateGroupVolatile(t *testing.T) {
	db := testDB(t)

	db.CreateGroup("g1", "Volatile", "o", "chat", "", 0, true)
	g, _ := db.GetGroup("g1")
	if !g.Volatile {
		t.Fatal("expected volatile = true")
	}
}

func TestGetGroupNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetGroup("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing group")
	}
}

func TestListGroups(t *testing.T) {
	db := testDB(t)

	db.CreateGroup("g1", "First", "o", "template", "", 0, false)
	db.CreateGroup("g2", "Second", "o", "chat", "", 0, false)

	groups, err := db.ListGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
}

func TestListGroupsEmpty(t *testing.T) {
	db := testDB(t)

	groups, err := db.ListGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(groups))
	}
}

func TestDeleteGroup(t *testing.T) {
	db := testDB(t)

	db.CreateGroup("g1", "Test", "o", "template", "", 0, false)
	if err := db.DeleteGroup("g1"); err != nil {
		t.Fatal(err)
	}

	_, err := db.GetGroup("g1")
	if err == nil {
		t.Fatal("group should be deleted")
	}
}

func TestSetMaxMembers(t *testing.T) {
	db := testDB(t)

	db.CreateGroup("g1", "Test", "o", "template", "", 5, false)
	db.SetMaxMembers("g1", 20)

	g, _ := db.GetGroup("g1")
	if g.MaxMembers != 20 {
		t.Fatalf("max_members = %d, want 20", g.MaxMembers)
	}
}

func TestUpdateGroup(t *testing.T) {
	db := testDB(t)

	db.CreateGroup("g1", "Old Name", "o", "template", "", 5, false)
	db.UpdateGroup("g1", "New Name", 15)

	g, _ := db.GetGroup("g1")
	if g.Name != "New Name" {
		t.Fatalf("name = %q, want 'New Name'", g.Name)
	}
	if g.MaxMembers != 15 {
		t.Fatalf("max_members = %d, want 15", g.MaxMembers)
	}
}

func TestSetGroupRoles(t *testing.T) {
	db := testDB(t)

	db.CreateGroup("g1", "Test", "o", "template", "", 0, false)
	db.SetGroupRoles("g1", []string{"admin", "editor", "viewer"})

	g, _ := db.GetGroup("g1")
	if len(g.Roles) != 3 {
		t.Fatalf("expected 3 roles, got %d", len(g.Roles))
	}
	if g.Roles[0] != "admin" {
		t.Fatalf("roles[0] = %q", g.Roles[0])
	}
}

func TestSetDefaultRole(t *testing.T) {
	db := testDB(t)

	db.CreateGroup("g1", "Test", "o", "template", "", 0, false)
	db.SetDefaultRole("g1", "editor")

	g, _ := db.GetGroup("g1")
	if g.DefaultRole != "editor" {
		t.Fatalf("default_role = %q, want 'editor'", g.DefaultRole)
	}
}

func TestSetHostJoined(t *testing.T) {
	db := testDB(t)

	db.CreateGroup("g1", "Test", "o", "template", "", 0, false)

	g, _ := db.GetGroup("g1")
	if g.HostJoined {
		t.Fatal("should not be host_joined initially")
	}

	db.SetHostJoined("g1", true)
	g, _ = db.GetGroup("g1")
	if !g.HostJoined {
		t.Fatal("expected host_joined = true")
	}

	db.SetHostJoined("g1", false)
	g, _ = db.GetGroup("g1")
	if g.HostJoined {
		t.Fatal("expected host_joined = false")
	}
}

func TestGroupMembersRoundtrip(t *testing.T) {
	db := testDB(t)

	db.CreateGroup("g1", "Test", "o", "template", "", 0, false)

	members := []GroupMember{
		{PeerID: "p1", Role: "admin"},
		{PeerID: "p2", Role: "editor"},
		{PeerID: "p3", Role: "viewer"},
	}
	if err := db.UpsertGroupMembers("g1", members); err != nil {
		t.Fatal(err)
	}

	got, err := db.ListGroupMembers("g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 members, got %d", len(got))
	}

	roles := map[string]string{}
	for _, m := range got {
		roles[m.PeerID] = m.Role
	}
	if roles["p1"] != "admin" {
		t.Fatalf("p1 role = %q, want 'admin'", roles["p1"])
	}
	if roles["p2"] != "editor" {
		t.Fatalf("p2 role = %q", roles["p2"])
	}
}

func TestUpsertGroupMembersReplaces(t *testing.T) {
	db := testDB(t)

	db.CreateGroup("g1", "Test", "o", "template", "", 0, false)
	db.UpsertGroupMembers("g1", []GroupMember{{PeerID: "p1", Role: "viewer"}, {PeerID: "p2", Role: "viewer"}})

	db.UpsertGroupMembers("g1", []GroupMember{{PeerID: "p3", Role: "admin"}})

	got, _ := db.ListGroupMembers("g1")
	if len(got) != 1 {
		t.Fatalf("expected 1 member after replace, got %d", len(got))
	}
	if got[0].PeerID != "p3" {
		t.Fatalf("peer_id = %q, want 'p3'", got[0].PeerID)
	}
}

func TestUpsertGroupMembersEmptyRole(t *testing.T) {
	db := testDB(t)

	db.CreateGroup("g1", "Test", "o", "template", "", 0, false)
	db.UpsertGroupMembers("g1", []GroupMember{{PeerID: "p1", Role: ""}})

	got, _ := db.ListGroupMembers("g1")
	if got[0].Role != "viewer" {
		t.Fatalf("empty role should default to 'viewer', got %q", got[0].Role)
	}
}

func TestDeleteGroupMembers(t *testing.T) {
	db := testDB(t)

	db.CreateGroup("g1", "Test", "o", "template", "", 0, false)
	db.UpsertGroupMembers("g1", []GroupMember{{PeerID: "p1", Role: "viewer"}})

	if err := db.DeleteGroupMembers("g1"); err != nil {
		t.Fatal(err)
	}

	got, _ := db.ListGroupMembers("g1")
	if len(got) != 0 {
		t.Fatalf("expected 0 members after delete, got %d", len(got))
	}
}

func TestSetMemberRole(t *testing.T) {
	db := testDB(t)

	db.CreateGroup("g1", "Test", "o", "template", "", 0, false)
	db.UpsertGroupMembers("g1", []GroupMember{{PeerID: "p1", Role: "viewer"}})

	db.SetMemberRole("g1", "p1", "admin")

	got, _ := db.ListGroupMembers("g1")
	if got[0].Role != "admin" {
		t.Fatalf("role = %q, want 'admin'", got[0].Role)
	}
}

func TestAddAndListSubscriptions(t *testing.T) {
	db := testDB(t)

	if err := db.AddSubscription("host1", "g1", "Group 1", "template", 10, false, "member", "HostName"); err != nil {
		t.Fatal(err)
	}
	if err := db.AddSubscription("host2", "g2", "Group 2", "chat", 0, true, "viewer", "Host2"); err != nil {
		t.Fatal(err)
	}

	subs, err := db.ListSubscriptions()
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", len(subs))
	}

	found := false
	for _, s := range subs {
		if s.HostPeerID == "host2" && s.GroupID == "g2" {
			found = true
			if s.GroupName != "Group 2" {
				t.Fatalf("group_name = %q", s.GroupName)
			}
			if !s.Volatile {
				t.Fatal("expected volatile")
			}
			if s.HostName != "Host2" {
				t.Fatalf("host_name = %q", s.HostName)
			}
		}
	}
	if !found {
		t.Fatal("subscription for host2/g2 not found")
	}
}

func TestRemoveSubscription(t *testing.T) {
	db := testDB(t)

	db.AddSubscription("host1", "g1", "Group 1", "template", 0, false, "member", "")
	if err := db.RemoveSubscription("host1", "g1"); err != nil {
		t.Fatal(err)
	}

	subs, _ := db.ListSubscriptions()
	if len(subs) != 0 {
		t.Fatalf("expected 0 subscriptions, got %d", len(subs))
	}
}

func TestUpdateSubscriptionHostName(t *testing.T) {
	db := testDB(t)

	db.AddSubscription("host1", "g1", "G1", "template", 0, false, "member", "OldName")
	db.AddSubscription("host1", "g2", "G2", "chat", 0, false, "member", "OldName")

	if err := db.UpdateSubscriptionHostName("host1", "NewName"); err != nil {
		t.Fatal(err)
	}

	subs, _ := db.ListSubscriptions()
	for _, s := range subs {
		if s.HostPeerID == "host1" && s.HostName != "NewName" {
			t.Fatalf("host_name = %q, want 'NewName'", s.HostName)
		}
	}
}

func TestAddSubscriptionUpsert(t *testing.T) {
	db := testDB(t)

	db.AddSubscription("host1", "g1", "Old Name", "template", 5, false, "member", "")
	db.AddSubscription("host1", "g1", "New Name", "template", 10, false, "admin", "")

	subs, _ := db.ListSubscriptions()
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription after upsert, got %d", len(subs))
	}
	if subs[0].GroupName != "New Name" {
		t.Fatalf("group_name = %q, want 'New Name'", subs[0].GroupName)
	}
}
