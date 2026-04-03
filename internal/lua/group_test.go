package lua

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
)

type mockGroupChecker struct {
	members map[string]string // peerID → role
}

type mockGroupManager struct {
	groups  map[string]*mockGroup
	selfID  string
}

type mockGroup struct {
	name    string
	members []group.MemberInfo
}

func newMockGroupManager(selfID string) *mockGroupManager {
	return &mockGroupManager{groups: make(map[string]*mockGroup), selfID: selfID}
}

func (m *mockGroupManager) CreateGroup(id, name, groupType, groupContext string, maxMembers int, volatile bool) error {
	m.groups[id] = &mockGroup{name: name}
	return nil
}

func (m *mockGroupManager) CloseGroup(groupID string) error {
	delete(m.groups, groupID)
	return nil
}

func (m *mockGroupManager) JoinOwnGroup(groupID string) error {
	g, ok := m.groups[groupID]
	if !ok {
		return fmt.Errorf("group not found")
	}
	g.members = append(g.members, group.MemberInfo{PeerID: m.selfID, Role: "owner"})
	return nil
}

func (m *mockGroupManager) KickMember(groupID, peerID string) error {
	g, ok := m.groups[groupID]
	if !ok {
		return fmt.Errorf("group not found")
	}
	for i, mem := range g.members {
		if mem.PeerID == peerID {
			g.members = append(g.members[:i], g.members[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockGroupManager) SetMemberRole(groupID, peerID, role string) error {
	g, ok := m.groups[groupID]
	if !ok {
		return fmt.Errorf("group not found")
	}
	for i, mem := range g.members {
		if mem.PeerID == peerID {
			g.members[i].Role = role
			return nil
		}
	}
	return fmt.Errorf("peer not in group")
}

func (m *mockGroupManager) HostedGroupMembers(groupID string) []group.MemberInfo {
	g, ok := m.groups[groupID]
	if !ok {
		return nil
	}
	return g.members
}

func (m *mockGroupManager) SendToGroupAsHost(groupID string, payload any) error {
	return nil
}

func (m *mockGroupManager) InvitePeer(ctx context.Context, peerID, groupID string) error {
	g, ok := m.groups[groupID]
	if !ok {
		return fmt.Errorf("group not found")
	}
	g.members = append(g.members, group.MemberInfo{PeerID: peerID, Role: "viewer"})
	return nil
}

func (m *mockGroupManager) ListHostedGroups() ([]storage.GroupRow, error) {
	var rows []storage.GroupRow
	for id, g := range m.groups {
		rows = append(rows, storage.GroupRow{ID: id, Name: g.name, GroupType: "clubhouse"})
	}
	return rows, nil
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
    save = function(p)
        init()
        return { id = posts:insert({ title = p.title }) }
    end,
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

func TestOwnerRejectsGroupMember(t *testing.T) {
	gc := &mockGroupChecker{members: map[string]string{"editor-1": "editor"}}
	e, _ := setupGroupEngine(t, gc)

	_, err := e.CallFunction(context.Background(), "editor-1", "test", map[string]any{
		"action": "owner_only",
	})
	if err == nil {
		t.Fatal("goop.owner() should reject group members")
	}
}

func TestGroupCreateAndMembers(t *testing.T) {
	dir := t.TempDir()
	funcDir := filepath.Join(dir, "site", "lua", "functions")
	os.MkdirAll(funcDir, 0755)

	src := `--- @rate_limit 0
local dispatch = goop.route({
    create_room = function()
        local gid = goop.group.create("TestRoom", "clubhouse", 10)
        return { group_id = gid }
    end,
    get_members = function(p)
        local members = goop.group.members(p.group_id)
        return { members = members, count = #members }
    end,
})
function call(req) return dispatch(req) end
`
	os.WriteFile(filepath.Join(funcDir, "rooms.lua"), []byte(src), 0644)

	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := testConfig()
	peers := state.NewPeerTable()
	e, err := NewEngine(cfg, dir, "self-peer-id", func() string { return "TestPeer" }, peers)
	if err != nil {
		t.Fatal(err)
	}
	e.SetDB(db)

	mgr := newMockGroupManager("self-peer-id")
	e.SetGroupManager(mgr)
	t.Cleanup(func() { e.Close() })

	result, err := e.CallFunction(context.Background(), "self-peer-id", "rooms", map[string]any{"action": "create_room"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	groupID, ok := m["group_id"].(string)
	if !ok || groupID == "" {
		t.Fatalf("expected group_id string, got %v", m["group_id"])
	}

	g := mgr.groups[groupID]
	if g == nil {
		t.Fatal("group should exist in mock manager")
	}
	if len(g.members) != 1 {
		t.Fatalf("expected 1 member (host auto-joined), got %d", len(g.members))
	}
	if g.members[0].PeerID != "self-peer-id" {
		t.Fatalf("expected host peer, got %s", g.members[0].PeerID)
	}

	result2, err := e.CallFunction(context.Background(), "self-peer-id", "rooms", map[string]any{"action": "get_members", "group_id": groupID})
	if err != nil {
		t.Fatal(err)
	}
	m2 := result2.(map[string]any)
	count := m2["count"]
	if count != float64(1) {
		t.Fatalf("expected 1 member from Lua, got %v", count)
	}
	members := m2["members"].([]any)
	mem := members[0].(map[string]any)
	if mem["peer_id"] != "self-peer-id" {
		t.Fatalf("expected self-peer-id, got %v", mem["peer_id"])
	}
	if mem["role"] != "owner" {
		t.Fatalf("expected role owner, got %v", mem["role"])
	}
}

func TestGroupSetRole(t *testing.T) {
	dir := t.TempDir()
	funcDir := filepath.Join(dir, "site", "lua", "functions")
	os.MkdirAll(funcDir, 0755)

	src := `--- @rate_limit 0
local dispatch = goop.route({
    create_room = function()
        local gid = goop.group.create("TestRoom", "clubhouse", 10)
        return { group_id = gid }
    end,
    set_role = function(p)
        goop.group.set_role(p.group_id, p.peer_id, p.role)
        return { ok = true }
    end,
    get_members = function(p)
        local members = goop.group.members(p.group_id)
        return { members = members }
    end,
})
function call(req) return dispatch(req) end
`
	os.WriteFile(filepath.Join(funcDir, "roles.lua"), []byte(src), 0644)

	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := testConfig()
	peers := state.NewPeerTable()
	e, err := NewEngine(cfg, dir, "self-peer-id", func() string { return "TestPeer" }, peers)
	if err != nil {
		t.Fatal(err)
	}
	e.SetDB(db)

	mgr := newMockGroupManager("self-peer-id")
	e.SetGroupManager(mgr)
	t.Cleanup(func() { e.Close() })

	result, err := e.CallFunction(context.Background(), "self-peer-id", "roles", map[string]any{"action": "create_room"})
	if err != nil {
		t.Fatal(err)
	}
	groupID := result.(map[string]any)["group_id"].(string)

	mgr.groups[groupID].members = append(mgr.groups[groupID].members, group.MemberInfo{PeerID: "peer-abc", Role: "viewer"})

	_, err = e.CallFunction(context.Background(), "self-peer-id", "roles", map[string]any{
		"action": "set_role", "group_id": groupID, "peer_id": "peer-abc", "role": "coauthor",
	})
	if err != nil {
		t.Fatal(err)
	}

	if mgr.groups[groupID].members[1].Role != "coauthor" {
		t.Fatalf("expected role 'coauthor', got %q", mgr.groups[groupID].members[1].Role)
	}
}

func TestTemplateRequireEmailLua(t *testing.T) {
	dir := t.TempDir()
	funcDir := filepath.Join(dir, "site", "lua", "functions")
	os.MkdirAll(funcDir, 0755)

	src := `--- @rate_limit 0
function call(req)
    return { require_email = goop.template.require_email }
end
`
	os.WriteFile(filepath.Join(funcDir, "tplcheck.lua"), []byte(src), 0644)

	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	db.SetMeta("template_require_email", "1")

	cfg := testConfig()
	peers := state.NewPeerTable()
	e, err := NewEngine(cfg, dir, "self-peer-id", func() string { return "TestPeer" }, peers)
	if err != nil {
		t.Fatal(err)
	}
	e.SetDB(db)
	t.Cleanup(func() { e.Close() })

	result, err := e.CallFunction(context.Background(), "self-peer-id", "tplcheck", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["require_email"] != true {
		t.Fatalf("goop.template.require_email should be true, got %v", m["require_email"])
	}
}
