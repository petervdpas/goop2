package routes

import (
	"encoding/json"
	"testing"

	"github.com/petervdpas/goop2/internal/group"
	ormschema "github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/sitetemplates"
)

func testDepsWithGroups(t *testing.T) Deps {
	t.Helper()
	d, _ := testDeps(t)
	d.GroupManager = group.NewTestManager(d.DB, "test-peer-id")
	t.Cleanup(func() { d.GroupManager.Close() })
	return d
}

func TestTemplateGroupCreated(t *testing.T) {
	d := testDepsWithGroups(t)

	files, _ := sitetemplates.SiteFiles("blog")
	if err := applyTemplateFiles(d, files, "", nil, "Blog", []string{"posts", "blog_config"}); err != nil {
		t.Fatal(err)
	}

	groupID := d.DB.GetMeta("template_group_id")
	if groupID == "" {
		t.Fatal("template_group_id should be set after applying blog template")
	}

	groups, err := d.GroupManager.ListHostedGroups()
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, g := range groups {
		if g.ID == groupID {
			found = true
			if g.GroupType != "template" {
				t.Fatalf("group type = %q, want 'template'", g.GroupType)
			}
			if g.GroupContext != "Blog" {
				t.Fatalf("group context = %q, want 'Blog'", g.GroupContext)
			}
		}
	}
	if !found {
		t.Fatalf("group %s not found in hosted groups", groupID)
	}
}

func TestTemplateGroupReusedOnReapply(t *testing.T) {
	d := testDepsWithGroups(t)

	files, _ := sitetemplates.SiteFiles("blog")
	if err := applyTemplateFiles(d, files, "", nil, "Blog", []string{"posts", "blog_config"}); err != nil {
		t.Fatal(err)
	}
	firstGroupID := d.DB.GetMeta("template_group_id")
	if firstGroupID == "" {
		t.Fatal("template_group_id should be set")
	}

	if err := applyTemplateFiles(d, files, "", nil, "Blog", []string{"posts", "blog_config"}); err != nil {
		t.Fatal(err)
	}
	secondGroupID := d.DB.GetMeta("template_group_id")

	if secondGroupID != firstGroupID {
		t.Fatalf("re-apply should reuse group: got %s, want %s", secondGroupID, firstGroupID)
	}
}

func TestTemplateGroupClosedOnSwitch(t *testing.T) {
	d := testDepsWithGroups(t)

	blogFiles, _ := sitetemplates.SiteFiles("blog")
	if err := applyTemplateFiles(d, blogFiles, "", nil, "Blog", []string{"posts", "blog_config"}); err != nil {
		t.Fatal(err)
	}
	blogGroupID := d.DB.GetMeta("template_group_id")
	if blogGroupID == "" {
		t.Fatal("template_group_id should be set after blog")
	}

	tttFiles, _ := sitetemplates.SiteFiles("tictactoe")
	if err := applyTemplateFiles(d, tttFiles, "", nil, "Tic-Tac-Toe", []string{"games"}); err != nil {
		t.Fatal(err)
	}

	newGroupID := d.DB.GetMeta("template_group_id")
	if newGroupID != "" {
		t.Fatalf("tictactoe has no group policy, template_group_id should be empty, got %q", newGroupID)
	}

	groups, _ := d.GroupManager.ListHostedGroups()
	for _, g := range groups {
		if g.ID == blogGroupID {
			t.Fatal("blog group should have been closed after switching to tictactoe")
		}
	}
}

func TestTemplateGroupNewOnDifferentGroupTemplate(t *testing.T) {
	d := testDepsWithGroups(t)

	blogSchema := ormschema.Table{
		Name: "posts", SystemKey: true,
		Columns: []ormschema.Column{{Name: "title", Type: "text", Required: true}},
		Access:  &ormschema.Access{Read: "open", Insert: "group", Update: "owner", Delete: "owner"},
	}
	blogJSON, _ := json.Marshal(blogSchema)
	tplA := map[string][]byte{
		"index.html":         []byte("<h1>A</h1>"),
		"schemas/posts.json": blogJSON,
	}

	otherSchema := ormschema.Table{
		Name: "tasks", SystemKey: true,
		Columns: []ormschema.Column{{Name: "name", Type: "text", Required: true}},
		Access:  &ormschema.Access{Read: "group", Insert: "group", Update: "owner", Delete: "owner"},
	}
	otherJSON, _ := json.Marshal(otherSchema)
	tplB := map[string][]byte{
		"index.html":         []byte("<h1>B</h1>"),
		"schemas/tasks.json": otherJSON,
	}

	if err := applyTemplateFiles(d, tplA, "", nil, "Alpha", []string{"posts"}); err != nil {
		t.Fatal(err)
	}
	groupA := d.DB.GetMeta("template_group_id")
	if groupA == "" {
		t.Fatal("group should be created for Alpha")
	}

	if err := applyTemplateFiles(d, tplB, "", nil, "Beta", []string{"tasks"}); err != nil {
		t.Fatal(err)
	}
	groupB := d.DB.GetMeta("template_group_id")
	if groupB == "" {
		t.Fatal("group should be created for Beta")
	}

	if groupB == groupA {
		t.Fatal("switching to a different template should create a new group, not reuse")
	}

	groups, _ := d.GroupManager.ListHostedGroups()
	for _, g := range groups {
		if g.ID == groupA {
			t.Fatal("Alpha's group should have been closed")
		}
	}
}

func TestNoGroupTemplateNoGroupCreated(t *testing.T) {
	d := testDepsWithGroups(t)

	tttFiles, _ := sitetemplates.SiteFiles("tictactoe")
	if err := applyTemplateFiles(d, tttFiles, "", nil, "Tic-Tac-Toe", []string{"games"}); err != nil {
		t.Fatal(err)
	}

	groupID := d.DB.GetMeta("template_group_id")
	if groupID != "" {
		t.Fatalf("tictactoe has no group policy, template_group_id should be empty, got %q", groupID)
	}

	groups, _ := d.GroupManager.ListHostedGroups()
	for _, g := range groups {
		if g.GroupType == "template" {
			t.Fatal("no template group should exist for tictactoe")
		}
	}
}
