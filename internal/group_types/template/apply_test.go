package template

import (
	"encoding/json"
	"testing"

	"github.com/petervdpas/goop2/internal/group"
	ormschema "github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/storage"
)

func testHandler(t *testing.T) (*Handler, *storage.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	grpMgr := group.NewTestManager(db, "test-peer")
	t.Cleanup(func() { grpMgr.Close() })

	h := New(grpMgr)
	return h, db
}

func blogSchemaFiles() map[string][]byte {
	posts := ormschema.Table{
		Name: "posts", SystemKey: true,
		Columns: []ormschema.Column{{Name: "title", Type: "text", Required: true}},
		Access:  &ormschema.Access{Read: "open", Insert: "group", Update: "group", Delete: "group"},
		Roles: map[string]ormschema.RoleAccess{
			"coauthor": {Read: true, Insert: true, Update: true, Delete: true},
			"viewer":   {Read: true},
		},
	}
	postsJSON, _ := json.Marshal(posts)
	return map[string][]byte{"schemas/posts.json": postsJSON}
}

func TestApplyCreatesGroup(t *testing.T) {
	h, db := testHandler(t)

	info := AnalyzeSchemas(blogSchemaFiles(), nil)
	groupID := h.Apply(ApplyConfig{
		DB:           db,
		TemplateName: "Blog",
		DefaultRole:  "coauthor",
		SchemaInfo:   info,
	})

	if groupID == "" {
		t.Fatal("expected group to be created")
	}
	if got := db.GetMeta("template_group_id"); got != groupID {
		t.Fatalf("template_group_id = %q, want %q", got, groupID)
	}
	if got := db.GetMeta("template_group_name"); got != "Blog" {
		t.Fatalf("template_group_name = %q, want 'Blog'", got)
	}
}

func TestApplyReusesGroup(t *testing.T) {
	h, db := testHandler(t)
	files := blogSchemaFiles()

	info := AnalyzeSchemas(files, nil)
	first := h.Apply(ApplyConfig{DB: db, TemplateName: "Blog", SchemaInfo: info})
	second := h.Apply(ApplyConfig{DB: db, TemplateName: "Blog", SchemaInfo: info})

	if second != first {
		t.Fatalf("re-apply should reuse group: got %s, want %s", second, first)
	}
}

func TestApplyClosesOnSwitch(t *testing.T) {
	h, db := testHandler(t)

	blogInfo := AnalyzeSchemas(blogSchemaFiles(), nil)
	blogGroup := h.Apply(ApplyConfig{DB: db, TemplateName: "Blog", SchemaInfo: blogInfo})
	if blogGroup == "" {
		t.Fatal("blog group should be created")
	}

	noGroupInfo := SchemaInfo{NeedsGroup: false}
	tttGroup := h.Apply(ApplyConfig{DB: db, TemplateName: "Tic-Tac-Toe", SchemaInfo: noGroupInfo})
	if tttGroup != "" {
		t.Fatalf("tictactoe should not create a group, got %q", tttGroup)
	}

	if got := db.GetMeta("template_group_id"); got != "" {
		t.Fatalf("template_group_id should be empty after switch, got %q", got)
	}

	groups, _ := h.grpMgr.ListHostedGroups()
	for _, g := range groups {
		if g.ID == blogGroup {
			t.Fatal("blog group should have been closed")
		}
	}
}

func TestApplyNewGroupOnDifferentTemplate(t *testing.T) {
	h, db := testHandler(t)

	infoA := AnalyzeSchemas(blogSchemaFiles(), nil)
	groupA := h.Apply(ApplyConfig{DB: db, TemplateName: "Alpha", SchemaInfo: infoA})

	infoB := AnalyzeSchemas(blogSchemaFiles(), nil)
	groupB := h.Apply(ApplyConfig{DB: db, TemplateName: "Beta", SchemaInfo: infoB})

	if groupA == "" || groupB == "" {
		t.Fatal("both templates should create groups")
	}
	if groupB == groupA {
		t.Fatal("different templates should create different groups")
	}

	groups, _ := h.grpMgr.ListHostedGroups()
	for _, g := range groups {
		if g.ID == groupA {
			t.Fatal("Alpha's group should have been closed")
		}
	}
}

func TestApplyNoGroupWithoutGroupAccess(t *testing.T) {
	h, db := testHandler(t)

	schema := ormschema.Table{
		Name:    "items",
		Columns: []ormschema.Column{{Name: "id", Type: "integer"}},
		Access:  &ormschema.Access{Read: "open", Insert: "owner", Update: "owner", Delete: "owner"},
	}
	data, _ := json.Marshal(schema)
	info := AnalyzeSchemas(map[string][]byte{"schemas/items.json": data}, nil)

	groupID := h.Apply(ApplyConfig{DB: db, TemplateName: "NoGroup", SchemaInfo: info})
	if groupID != "" {
		t.Fatalf("should not create group, got %q", groupID)
	}
}

func TestApplySetsDefaultRoleAndRoles(t *testing.T) {
	h, db := testHandler(t)

	info := AnalyzeSchemas(blogSchemaFiles(), nil)
	groupID := h.Apply(ApplyConfig{
		DB:           db,
		TemplateName: "Blog",
		DefaultRole:  "coauthor",
		SchemaInfo:   info,
	})

	g, err := db.GetGroup(groupID)
	if err != nil {
		t.Fatal(err)
	}
	if g.DefaultRole != "coauthor" {
		t.Fatalf("default_role = %q, want 'coauthor'", g.DefaultRole)
	}
	if len(g.Roles) == 0 {
		t.Fatal("roles should be set from schema")
	}
}
