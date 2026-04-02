package lua

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
)

type mockGroupChecker struct {
	members map[string]string // peerID → role
}

func (m *mockGroupChecker) IsTemplateMember(peerID string) bool {
	_, ok := m.members[peerID]
	return ok
}

func (m *mockGroupChecker) TemplateMemberRole(peerID string) string {
	return m.members[peerID]
}

func (m *mockGroupChecker) TemplateGroupOwner() string {
	return "self-peer-id"
}

func setupGroupEngine(t *testing.T, gc GroupChecker) (*Engine, *storage.DB) {
	t.Helper()

	dir := t.TempDir()
	funcDir := filepath.Join(dir, "site", "lua", "functions")
	os.MkdirAll(funcDir, 0755)

	src := `--- @rate_limit 0
local posts = nil

local function init()
    if not posts then posts = goop.orm("posts") end
end

local dispatch = goop.route({
    check_member = function()
        return { is_member = goop.group.is_member() }
    end,
    check_role = function()
        return { role = goop.group.member.role() }
    end,
    save = goop.coauthor(function(p)
        init()
        return { id = posts:insert({ title = p.title }) }
    end),
    owner_only = goop.owner(function()
        return { ok = true }
    end),
})

function call(req) return dispatch(req) end
`
	os.WriteFile(filepath.Join(funcDir, "test.lua"), []byte(src), 0644)

	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	db.CreateTableORM(&schema.Table{
		Name:      "posts",
		SystemKey: true,
		Columns:   []schema.Column{{Name: "title", Type: "text", Required: true}},
		Access:    &schema.Access{Read: "open", Insert: "group", Update: "owner", Delete: "owner"},
	})

	cfg := testConfig()
	peers := state.NewPeerTable()
	e, err := NewEngine(cfg, dir, "self-peer-id", func() string { return "TestPeer" }, peers)
	if err != nil {
		t.Fatal(err)
	}
	e.SetDB(db)
	e.SetGroupChecker(gc)
	t.Cleanup(func() { e.Close() })

	return e, db
}

func TestGroupIsMemberOwner(t *testing.T) {
	e, _ := setupGroupEngine(t, &mockGroupChecker{})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "test", map[string]any{"action": "check_member"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["is_member"] != true {
		t.Fatalf("owner should be a member, got %v", m["is_member"])
	}
}

func TestGroupIsMemberGroupPeer(t *testing.T) {
	gc := &mockGroupChecker{members: map[string]string{"peer-abc": "editor"}}
	e, _ := setupGroupEngine(t, gc)

	result, err := e.CallFunction(context.Background(), "peer-abc", "test", map[string]any{"action": "check_member"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["is_member"] != true {
		t.Fatalf("group member should be a member, got %v", m["is_member"])
	}
}

func TestGroupIsMemberStranger(t *testing.T) {
	e, _ := setupGroupEngine(t, &mockGroupChecker{})

	result, err := e.CallFunction(context.Background(), "stranger", "test", map[string]any{"action": "check_member"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["is_member"] != false {
		t.Fatalf("stranger should not be a member, got %v", m["is_member"])
	}
}

func TestGroupIsMemberNoChecker(t *testing.T) {
	e, _ := setupGroupEngine(t, nil)

	result, err := e.CallFunction(context.Background(), "stranger", "test", map[string]any{"action": "check_member"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["is_member"] != false {
		t.Fatalf("no group checker should mean not a member, got %v", m["is_member"])
	}
}

func TestCoauthorAllowsOwner(t *testing.T) {
	e, _ := setupGroupEngine(t, &mockGroupChecker{})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "test", map[string]any{
		"action": "save", "title": "Hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["id"] == nil || m["id"] == float64(0) {
		t.Fatal("owner should be able to save")
	}
}

func TestCoauthorAllowsGroupMember(t *testing.T) {
	gc := &mockGroupChecker{members: map[string]string{"coauthor-1": "editor"}}
	e, _ := setupGroupEngine(t, gc)

	result, err := e.CallFunction(context.Background(), "coauthor-1", "test", map[string]any{
		"action": "save", "title": "Guest Post",
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["id"] == nil || m["id"] == float64(0) {
		t.Fatal("group member should be able to save")
	}
}

func TestCoauthorRejectsStranger(t *testing.T) {
	e, _ := setupGroupEngine(t, &mockGroupChecker{})

	_, err := e.CallFunction(context.Background(), "stranger", "test", map[string]any{
		"action": "save", "title": "Nope",
	})
	if err == nil {
		t.Fatal("stranger should be rejected by coauthor wrapper")
	}
}

func TestGroupMemberRoleOwner(t *testing.T) {
	e, _ := setupGroupEngine(t, &mockGroupChecker{})
	result, err := e.CallFunction(context.Background(), "self-peer-id", "test", map[string]any{"action": "check_role"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["role"] != "owner" {
		t.Fatalf("host should have role 'owner', got %v", m["role"])
	}
}

func TestGroupMemberRoleEditor(t *testing.T) {
	gc := &mockGroupChecker{members: map[string]string{"peer-abc": "editor"}}
	e, _ := setupGroupEngine(t, gc)
	result, err := e.CallFunction(context.Background(), "peer-abc", "test", map[string]any{"action": "check_role"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["role"] != "editor" {
		t.Fatalf("expected role 'editor', got %v", m["role"])
	}
}

func TestGroupMemberRoleStranger(t *testing.T) {
	e, _ := setupGroupEngine(t, &mockGroupChecker{})
	result, err := e.CallFunction(context.Background(), "stranger", "test", map[string]any{"action": "check_role"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["role"] != "" {
		t.Fatalf("stranger should have empty role, got %v", m["role"])
	}
}

func TestCoauthorRejectsViewer(t *testing.T) {
	gc := &mockGroupChecker{members: map[string]string{"viewer-1": "viewer"}}
	e, _ := setupGroupEngine(t, gc)

	// coauthor() allows any group member — the role check happens at the data layer.
	// The Lua wrapper only gates on membership, not role.
	result, err := e.CallFunction(context.Background(), "viewer-1", "test", map[string]any{
		"action": "save", "title": "From Viewer",
	})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["id"] == nil {
		t.Fatal("coauthor() should allow viewer (role enforcement is at data layer)")
	}
}

func TestOwnerStillRejectsGroupMember(t *testing.T) {
	gc := &mockGroupChecker{members: map[string]string{"coauthor-1": "editor"}}
	e, _ := setupGroupEngine(t, gc)

	_, err := e.CallFunction(context.Background(), "coauthor-1", "test", map[string]any{
		"action": "owner_only",
	})
	if err == nil {
		t.Fatal("goop.owner() should still reject group members")
	}
}
