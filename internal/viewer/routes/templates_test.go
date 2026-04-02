package routes

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petervdpas/goop2/internal/content"
	ormschema "github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/sitetemplates"
	"github.com/petervdpas/goop2/internal/storage"
)

func testDeps(t *testing.T) (Deps, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	os.MkdirAll(filepath.Join(dir, "site"), 0o755)
	store, err2 := content.NewStore(dir, "site")
	if err2 != nil {
		t.Fatal(err2)
	}

	return Deps{DB: db, Content: store, PeerDir: dir}, dir
}

func TestApplyBuiltinBlog(t *testing.T) {
	d, _ := testDeps(t)

	files, err := sitetemplates.SiteFiles("blog")
	if err != nil {
		t.Fatal(err)
	}

	if err := applyTemplateFiles(d, files, "", nil, "Blog", []string{"posts", "blog_config"}, false); err != nil {
		t.Fatal(err)
	}

	tables, err := d.DB.ListTables()
	if err != nil {
		t.Fatal(err)
	}

	tableNames := map[string]bool{}
	for _, tbl := range tables {
		tableNames[tbl.Name] = true
	}
	if !tableNames["posts"] {
		t.Fatal("expected table 'posts' to be created")
	}
	if !tableNames["blog_config"] {
		t.Fatal("expected table 'blog_config' to be created")
	}

	if !d.DB.IsORM("posts") {
		t.Fatal("posts should be ORM-managed")
	}
	if !d.DB.IsORM("blog_config") {
		t.Fatal("blog_config should be ORM-managed")
	}

	access := d.DB.GetAccess("posts")
	if access.Insert != "group" {
		t.Fatalf("posts access.insert = %q, want 'group'", access.Insert)
	}
	if access.Read != "open" {
		t.Fatalf("posts access.read = %q, want 'open'", access.Read)
	}

	access = d.DB.GetAccess("blog_config")
	if access.Insert != "owner" {
		t.Fatalf("blog_config access.insert = %q, want 'owner'", access.Insert)
	}
}

func TestApplyBuiltinEnquete(t *testing.T) {
	d, _ := testDeps(t)

	files, err := sitetemplates.SiteFiles("enquete")
	if err != nil {
		t.Fatal(err)
	}

	if err := applyTemplateFiles(d, files, "", nil, "Enquete", []string{"responses"}, false); err != nil {
		t.Fatal(err)
	}

	if !d.DB.IsORM("responses") {
		t.Fatal("responses should be ORM-managed")
	}

	access := d.DB.GetAccess("responses")
	if access.Insert != "open" {
		t.Fatalf("responses access.insert = %q, want 'open'", access.Insert)
	}
	if access.Read != "owner" {
		t.Fatalf("responses access.read = %q, want 'owner'", access.Read)
	}
}

func TestApplyBuiltinTictactoe(t *testing.T) {
	d, _ := testDeps(t)

	files, err := sitetemplates.SiteFiles("tictactoe")
	if err != nil {
		t.Fatal(err)
	}

	if err := applyTemplateFiles(d, files, "", nil, "Tic-Tac-Toe", []string{"games"}, false); err != nil {
		t.Fatal(err)
	}

	if !d.DB.IsORM("games") {
		t.Fatal("games should be ORM-managed")
	}

	access := d.DB.GetAccess("games")
	if access.Insert != "open" {
		t.Fatalf("games access.insert = %q, want 'open'", access.Insert)
	}
}

func TestApplyBuiltinClubhouse(t *testing.T) {
	d, _ := testDeps(t)

	files, err := sitetemplates.SiteFiles("clubhouse")
	if err != nil {
		t.Fatal(err)
	}

	if err := applyTemplateFiles(d, files, "", nil, "Clubhouse", []string{"rooms"}, false); err != nil {
		t.Fatal(err)
	}

	if !d.DB.IsORM("rooms") {
		t.Fatal("rooms should be ORM-managed")
	}

	access := d.DB.GetAccess("rooms")
	if access.Insert != "owner" {
		t.Fatalf("rooms access.insert = %q, want 'owner'", access.Insert)
	}
}

func TestApplyReplacesExistingTables(t *testing.T) {
	d, _ := testDeps(t)

	blogFiles, _ := sitetemplates.SiteFiles("blog")
	if err := applyTemplateFiles(d, blogFiles, "", nil, "Blog", []string{"posts", "blog_config"}, false); err != nil {
		t.Fatal(err)
	}

	if !d.DB.IsORM("posts") {
		t.Fatal("posts should exist after blog apply")
	}

	tttFiles, _ := sitetemplates.SiteFiles("tictactoe")
	if err := applyTemplateFiles(d, tttFiles, "", nil, "Tic-Tac-Toe", []string{"games"}, false); err != nil {
		t.Fatal(err)
	}

	if d.DB.IsORM("posts") {
		t.Fatal("posts should NOT exist after tictactoe apply")
	}
	if !d.DB.IsORM("games") {
		t.Fatal("games should exist after tictactoe apply")
	}
}

func TestApplyWritesSiteFiles(t *testing.T) {
	d, dir := testDeps(t)

	files, _ := sitetemplates.SiteFiles("blog")
	if err := applyTemplateFiles(d, files, "", nil, "Blog", []string{"posts", "blog_config"}, false); err != nil {
		t.Fatal(err)
	}

	siteDir := filepath.Join(dir, "site")
	indexPath := filepath.Join(siteDir, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Fatal("index.html should be written to site dir")
	}

	schemaDir := filepath.Join(dir, "schemas")
	entries, err := os.ReadDir(schemaDir)
	if err != nil {
		t.Fatal("schemas/ dir should exist in peer dir")
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 schema files, got %d", len(entries))
	}

	// schemas should NOT be in site dir
	if _, err := os.Stat(filepath.Join(siteDir, "schemas")); !os.IsNotExist(err) {
		t.Fatal("schemas/ should NOT be written to site dir")
	}
}

func TestApplyGroupPolicyDetection(t *testing.T) {
	blogSchema := ormschema.Table{
		Name:      "posts",
		SystemKey: true,
		Columns:   []ormschema.Column{{Name: "title", Type: "text", Required: true}},
		Access:    &ormschema.Access{Read: "open", Insert: "group", Update: "owner", Delete: "owner"},
	}
	ownerSchema := ormschema.Table{
		Name:      "config",
		SystemKey: true,
		Columns:   []ormschema.Column{{Name: "key", Type: "text", Required: true}},
		Access:    &ormschema.Access{Read: "open", Insert: "owner", Update: "owner", Delete: "owner"},
	}

	blogJSON, _ := json.Marshal(blogSchema)
	ownerJSON, _ := json.Marshal(ownerSchema)

	tests := []struct {
		name     string
		files    map[string][]byte
		wantGroup bool
	}{
		{
			name:      "group policy in schema",
			files:     map[string][]byte{"schemas/posts.json": blogJSON},
			wantGroup: true,
		},
		{
			name:      "no group policy",
			files:     map[string][]byte{"schemas/config.json": ownerJSON},
			wantGroup: false,
		},
		{
			name:      "no schemas at all",
			files:     map[string][]byte{"index.html": []byte("<h1>hi</h1>")},
			wantGroup: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hasGroup := false
			for rel, data := range tc.files {
				if len(rel) < 9 || rel[:8] != "schemas/" || rel[len(rel)-5:] != ".json" {
					continue
				}
				var tbl ormschema.Table
				if json.Unmarshal(data, &tbl) == nil && tbl.Access != nil && tbl.Access.Insert == "group" {
					hasGroup = true
					break
				}
			}
			if hasGroup != tc.wantGroup {
				t.Fatalf("hasGroupPolicy = %v, want %v", hasGroup, tc.wantGroup)
			}
		})
	}
}

func TestSystemKeyValidation(t *testing.T) {
	withKey := ormschema.Table{
		Name:      "t1",
		SystemKey: true,
		Columns:   []ormschema.Column{{Name: "title", Type: "text", Required: true}},
	}
	if err := withKey.Validate(); err != nil {
		t.Fatalf("system_key table should validate: %v", err)
	}

	withoutKey := ormschema.Table{
		Name:    "t2",
		Columns: []ormschema.Column{{Name: "title", Type: "text", Required: true}},
	}
	if err := withoutKey.Validate(); err == nil {
		t.Fatal("table without key or system_key should fail validation")
	}

	withUserKey := ormschema.Table{
		Name:    "t3",
		Columns: []ormschema.Column{{Name: "id", Type: "guid", Key: true, Required: true, Auto: true}},
	}
	if err := withUserKey.Validate(); err != nil {
		t.Fatalf("table with user key should validate: %v", err)
	}
}

func TestSeedFunctionCalled(t *testing.T) {
	d, _ := testDeps(t)

	seedCalled := false
	d.LuaCall = func(ctx context.Context, function string, params map[string]any) (any, error) {
		if function == "seed" {
			seedCalled = true
		}
		return nil, nil
	}
	d.EnsureLua = func() {}

	schema := ormschema.Table{
		Name:      "items",
		SystemKey: true,
		Columns:   []ormschema.Column{{Name: "name", Type: "text", Required: true}},
	}
	schemaJSON, _ := json.Marshal(schema)

	files := map[string][]byte{
		"schemas/items.json":      schemaJSON,
		"lua/functions/seed.lua":  []byte("function call(req) return 'seeded' end"),
	}

	if err := applyTemplateFiles(d, files, "", nil, "Test", []string{"items"}, false); err != nil {
		t.Fatal(err)
	}

	if !seedCalled {
		t.Fatal("seed function should have been called")
	}
}

func TestSeedNotCalledWithoutSeedFile(t *testing.T) {
	d, _ := testDeps(t)

	seedCalled := false
	d.LuaCall = func(ctx context.Context, function string, params map[string]any) (any, error) {
		if function == "seed" {
			seedCalled = true
		}
		return nil, nil
	}
	d.EnsureLua = func() {}

	schema := ormschema.Table{
		Name:      "items",
		SystemKey: true,
		Columns:   []ormschema.Column{{Name: "name", Type: "text", Required: true}},
	}
	schemaJSON, _ := json.Marshal(schema)

	files := map[string][]byte{
		"schemas/items.json":          schemaJSON,
		"lua/functions/other.lua":     []byte("function call(req) return 'ok' end"),
	}

	if err := applyTemplateFiles(d, files, "", nil, "Test", []string{"items"}, false); err != nil {
		t.Fatal(err)
	}

	if seedCalled {
		t.Fatal("seed function should NOT have been called without seed.lua")
	}
}

func TestLegacySchemaSQLStillWorks(t *testing.T) {
	d, _ := testDeps(t)

	sqlSchema := `CREATE TABLE legacy (
		_id INTEGER PRIMARY KEY AUTOINCREMENT,
		_owner TEXT NOT NULL,
		_owner_email TEXT DEFAULT '',
		_created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		title TEXT NOT NULL
	);`

	policies := map[string]string{"legacy": "open"}

	if err := applyTemplateFiles(d, nil, sqlSchema, policies, "Legacy", nil, false); err != nil {
		t.Fatal(err)
	}

	tables, _ := d.DB.ListTables()
	found := false
	for _, tbl := range tables {
		if tbl.Name == "legacy" {
			found = true
			if tbl.InsertPolicy != "open" {
				t.Fatalf("legacy table insert_policy = %q, want 'open'", tbl.InsertPolicy)
			}
		}
	}
	if !found {
		t.Fatal("legacy table should be created via schema.sql")
	}

	if d.DB.IsORM("legacy") {
		t.Fatal("legacy table should NOT be ORM-managed")
	}
}

func TestUserTablesSurviveTemplateApply(t *testing.T) {
	d, _ := testDeps(t)

	// Apply blog template first
	blogFiles, _ := sitetemplates.SiteFiles("blog")
	if err := applyTemplateFiles(d, blogFiles, "", nil, "Blog", []string{"posts", "blog_config"}, false); err != nil {
		t.Fatal(err)
	}
	if !d.DB.IsORM("posts") {
		t.Fatal("posts should exist after blog apply")
	}

	// User creates their own table
	userTable := &ormschema.Table{
		Name:      "my_notes",
		SystemKey: true,
		Columns:   []ormschema.Column{{Name: "text", Type: "text", Required: true}},
	}
	if err := d.DB.CreateTableORM(userTable); err != nil {
		t.Fatal(err)
	}
	d.DB.OrmInsert("my_notes", "user1", "", map[string]any{"text": "important"})

	// Apply tictactoe — should drop blog tables but keep user table
	tttFiles, _ := sitetemplates.SiteFiles("tictactoe")
	if err := applyTemplateFiles(d, tttFiles, "", nil, "Tic-Tac-Toe", []string{"games"}, false); err != nil {
		t.Fatal(err)
	}

	if d.DB.IsORM("posts") {
		t.Fatal("posts should be gone after tictactoe apply")
	}
	if !d.DB.IsORM("games") {
		t.Fatal("games should exist after tictactoe apply")
	}
	if !d.DB.IsORM("my_notes") {
		t.Fatal("user table 'my_notes' should survive template apply")
	}

	// Verify user data is intact
	rows, err := d.DB.OrmList("my_notes", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 user row, got %d", len(rows))
	}
	if rows[0]["text"] != "important" {
		t.Fatalf("user data corrupted: got %v", rows[0]["text"])
	}
}

func TestTemplateSwitchCleansOldSchemaFiles(t *testing.T) {
	d, dir := testDeps(t)

	blogFiles, _ := sitetemplates.SiteFiles("blog")
	if err := applyTemplateFiles(d, blogFiles, "", nil, "Blog", []string{"posts", "blog_config"}, false); err != nil {
		t.Fatal(err)
	}

	postsSchema := filepath.Join(dir, "schemas", "posts.json")
	configSchema := filepath.Join(dir, "schemas", "blog_config.json")
	if _, err := os.Stat(postsSchema); os.IsNotExist(err) {
		t.Fatal("posts.json should exist after blog apply")
	}
	if _, err := os.Stat(configSchema); os.IsNotExist(err) {
		t.Fatal("blog_config.json should exist after blog apply")
	}

	tttFiles, _ := sitetemplates.SiteFiles("tictactoe")
	if err := applyTemplateFiles(d, tttFiles, "", nil, "Tic-Tac-Toe", []string{"games"}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(postsSchema); !os.IsNotExist(err) {
		t.Fatal("posts.json should be removed after switching to tictactoe")
	}
	if _, err := os.Stat(configSchema); !os.IsNotExist(err) {
		t.Fatal("blog_config.json should be removed after switching to tictactoe")
	}

	gamesSchema := filepath.Join(dir, "schemas", "games.json")
	if _, err := os.Stat(gamesSchema); os.IsNotExist(err) {
		t.Fatal("games.json should exist after tictactoe apply")
	}
}

func TestTemplateSwitchWithMockTemplates(t *testing.T) {
	d, dir := testDeps(t)

	schemaA := ormschema.Table{
		Name: "posts", SystemKey: true,
		Columns: []ormschema.Column{{Name: "title", Type: "text", Required: true}},
		Access:  &ormschema.Access{Read: "open", Insert: "group", Update: "owner", Delete: "owner"},
	}
	schemaB := ormschema.Table{
		Name: "config", SystemKey: true,
		Columns: []ormschema.Column{{Name: "key", Type: "text", Required: true}},
		Access:  &ormschema.Access{Read: "open", Insert: "owner", Update: "owner", Delete: "owner"},
	}
	schemaC := ormschema.Table{
		Name: "games", SystemKey: true,
		Columns: []ormschema.Column{{Name: "board", Type: "text", Required: true}},
		Access:  &ormschema.Access{Read: "open", Insert: "open", Update: "owner", Delete: "owner"},
	}

	aJSON, _ := json.Marshal(schemaA)
	bJSON, _ := json.Marshal(schemaB)
	cJSON, _ := json.Marshal(schemaC)

	templateAlpha := map[string][]byte{
		"index.html":          []byte("<h1>Alpha</h1>"),
		"schemas/posts.json":  aJSON,
		"schemas/config.json": bJSON,
	}
	templateBeta := map[string][]byte{
		"index.html":         []byte("<h1>Beta</h1>"),
		"schemas/games.json": cJSON,
	}

	// Apply Alpha
	if err := applyTemplateFiles(d, templateAlpha, "", nil, "Alpha", []string{"posts", "config"}, false); err != nil {
		t.Fatal(err)
	}

	if !d.DB.IsORM("posts") || !d.DB.IsORM("config") {
		t.Fatal("Alpha tables should exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "schemas", "posts.json")); os.IsNotExist(err) {
		t.Fatal("posts.json should exist on disk")
	}

	// User creates own table + schema file
	userTable := &ormschema.Table{
		Name: "my_stuff", SystemKey: true,
		Columns: []ormschema.Column{{Name: "note", Type: "text"}},
	}
	d.DB.CreateTableORM(userTable)
	d.DB.OrmInsert("my_stuff", "me", "", map[string]any{"note": "keep me"})
	userSchemaJSON, _ := json.Marshal(userTable)
	os.MkdirAll(filepath.Join(dir, "schemas"), 0o755)
	os.WriteFile(filepath.Join(dir, "schemas", "my_stuff.json"), userSchemaJSON, 0o644)

	// Apply Beta — should drop Alpha tables+files, keep user table+file
	if err := applyTemplateFiles(d, templateBeta, "", nil, "Beta", []string{"games"}, false); err != nil {
		t.Fatal(err)
	}

	// Alpha tables gone
	if d.DB.IsORM("posts") {
		t.Fatal("posts should be gone after Beta apply")
	}
	if d.DB.IsORM("config") {
		t.Fatal("config should be gone after Beta apply")
	}
	if _, err := os.Stat(filepath.Join(dir, "schemas", "posts.json")); !os.IsNotExist(err) {
		t.Fatal("posts.json should be removed from disk")
	}
	if _, err := os.Stat(filepath.Join(dir, "schemas", "config.json")); !os.IsNotExist(err) {
		t.Fatal("config.json should be removed from disk")
	}

	// Beta tables present
	if !d.DB.IsORM("games") {
		t.Fatal("games should exist after Beta apply")
	}
	if _, err := os.Stat(filepath.Join(dir, "schemas", "games.json")); os.IsNotExist(err) {
		t.Fatal("games.json should exist on disk")
	}

	// User table + file intact
	if !d.DB.IsORM("my_stuff") {
		t.Fatal("user table my_stuff should survive")
	}
	rows, _ := d.DB.OrmList("my_stuff", 0)
	if len(rows) != 1 || rows[0]["note"] != "keep me" {
		t.Fatal("user data should be intact")
	}
	if _, err := os.Stat(filepath.Join(dir, "schemas", "my_stuff.json")); os.IsNotExist(err) {
		t.Fatal("user schema file my_stuff.json should survive")
	}
}

func TestTemplateAtoB_backToA(t *testing.T) {
	d, dir := testDeps(t)

	schemaA := ormschema.Table{
		Name: "alpha_data", SystemKey: true,
		Columns: []ormschema.Column{{Name: "val", Type: "text"}},
		Access:  &ormschema.Access{Read: "open", Insert: "owner", Update: "owner", Delete: "owner"},
	}
	schemaB := ormschema.Table{
		Name: "beta_data", SystemKey: true,
		Columns: []ormschema.Column{{Name: "val", Type: "text"}},
		Access:  &ormschema.Access{Read: "open", Insert: "open", Update: "owner", Delete: "owner"},
	}
	aJSON, _ := json.Marshal(schemaA)
	bJSON, _ := json.Marshal(schemaB)

	tplA := map[string][]byte{
		"index.html":              []byte("<h1>A</h1>"),
		"schemas/alpha_data.json": aJSON,
	}
	tplB := map[string][]byte{
		"index.html":             []byte("<h1>B</h1>"),
		"schemas/beta_data.json": bJSON,
	}

	schemasDir := filepath.Join(dir, "schemas")

	// 1. Apply A
	if err := applyTemplateFiles(d, tplA, "", nil, "A", []string{"alpha_data"}, false); err != nil {
		t.Fatal(err)
	}
	if !d.DB.IsORM("alpha_data") {
		t.Fatal("alpha_data should exist after A")
	}
	if _, err := os.Stat(filepath.Join(schemasDir, "alpha_data.json")); os.IsNotExist(err) {
		t.Fatal("alpha_data.json should be on disk after A")
	}

	// 2. Apply B — A's table and schema file should be gone
	if err := applyTemplateFiles(d, tplB, "", nil, "B", []string{"beta_data"}, false); err != nil {
		t.Fatal(err)
	}
	if d.DB.IsORM("alpha_data") {
		t.Fatal("alpha_data should be gone after B")
	}
	if _, err := os.Stat(filepath.Join(schemasDir, "alpha_data.json")); !os.IsNotExist(err) {
		t.Fatal("alpha_data.json should be removed after B")
	}
	if !d.DB.IsORM("beta_data") {
		t.Fatal("beta_data should exist after B")
	}
	if _, err := os.Stat(filepath.Join(schemasDir, "beta_data.json")); os.IsNotExist(err) {
		t.Fatal("beta_data.json should be on disk after B")
	}

	// 3. Apply A again — B's table and schema file should be gone, A's restored
	if err := applyTemplateFiles(d, tplA, "", nil, "A", []string{"alpha_data"}, false); err != nil {
		t.Fatal(err)
	}
	if d.DB.IsORM("beta_data") {
		t.Fatal("beta_data should be gone after re-apply A")
	}
	if _, err := os.Stat(filepath.Join(schemasDir, "beta_data.json")); !os.IsNotExist(err) {
		t.Fatal("beta_data.json should be removed after re-apply A")
	}
	if !d.DB.IsORM("alpha_data") {
		t.Fatal("alpha_data should exist after re-apply A")
	}
	if _, err := os.Stat(filepath.Join(schemasDir, "alpha_data.json")); os.IsNotExist(err) {
		t.Fatal("alpha_data.json should be on disk after re-apply A")
	}
}

func TestTemplateFailoverSharedTableName(t *testing.T) {
	d, dir := testDeps(t)

	shared := ormschema.Table{
		Name: "data", SystemKey: true,
		Columns: []ormschema.Column{{Name: "val", Type: "text"}},
		Access:  &ormschema.Access{Read: "open", Insert: "owner", Update: "owner", Delete: "owner"},
	}
	onlyA := ormschema.Table{
		Name: "a_only", SystemKey: true,
		Columns: []ormschema.Column{{Name: "x", Type: "text"}},
		Access:  &ormschema.Access{Read: "open", Insert: "owner", Update: "owner", Delete: "owner"},
	}
	onlyB := ormschema.Table{
		Name: "b_only", SystemKey: true,
		Columns: []ormschema.Column{{Name: "y", Type: "text"}},
		Access:  &ormschema.Access{Read: "open", Insert: "open", Update: "owner", Delete: "owner"},
	}
	sharedJSON, _ := json.Marshal(shared)
	aJSON, _ := json.Marshal(onlyA)
	bJSON, _ := json.Marshal(onlyB)

	tplA := map[string][]byte{
		"index.html":          []byte("<h1>A</h1>"),
		"schemas/data.json":   sharedJSON,
		"schemas/a_only.json": aJSON,
	}
	tplB := map[string][]byte{
		"index.html":          []byte("<h1>B</h1>"),
		"schemas/data.json":   sharedJSON,
		"schemas/b_only.json": bJSON,
	}

	schemasDir := filepath.Join(dir, "schemas")

	// Apply A
	if err := applyTemplateFiles(d, tplA, "", nil, "A", []string{"data", "a_only"}, false); err != nil {
		t.Fatal(err)
	}
	d.DB.OrmInsert("data", "me", "", map[string]any{"val": "from_A"})

	// Apply B — both share "data", B should get a fresh "data" table
	if err := applyTemplateFiles(d, tplB, "", nil, "B", []string{"data", "b_only"}, false); err != nil {
		t.Fatal(err)
	}
	if d.DB.IsORM("a_only") {
		t.Fatal("a_only should be gone after B")
	}
	if !d.DB.IsORM("b_only") {
		t.Fatal("b_only should exist after B")
	}
	rows, _ := d.DB.OrmList("data", 0)
	if len(rows) != 0 {
		t.Fatal("shared 'data' table should be fresh (empty) after B, not carry A's rows")
	}

	// Apply A again — should work even though "data" exists from B
	if err := applyTemplateFiles(d, tplA, "", nil, "A", []string{"data", "a_only"}, false); err != nil {
		t.Fatalf("re-apply A should not fail: %v", err)
	}
	if !d.DB.IsORM("data") {
		t.Fatal("data should exist after re-apply A")
	}
	if !d.DB.IsORM("a_only") {
		t.Fatal("a_only should exist after re-apply A")
	}
	if d.DB.IsORM("b_only") {
		t.Fatal("b_only should be gone after re-apply A")
	}
	if _, err := os.Stat(filepath.Join(schemasDir, "b_only.json")); !os.IsNotExist(err) {
		t.Fatal("b_only.json should be removed from disk")
	}
}

func TestTemplateTablesMetaTracking(t *testing.T) {
	d, _ := testDeps(t)

	blogFiles, _ := sitetemplates.SiteFiles("blog")
	if err := applyTemplateFiles(d, blogFiles, "", nil, "Blog", []string{"posts", "blog_config"}, false); err != nil {
		t.Fatal(err)
	}

	meta := d.DB.GetMeta("template_tables")
	if meta == "" {
		t.Fatal("template_tables meta should be set after apply")
	}
	if !strings.Contains(meta, "posts") || !strings.Contains(meta, "blog_config") {
		t.Fatalf("template_tables should contain posts and blog_config, got %q", meta)
	}
}

func TestRequireEmailStoredInMeta(t *testing.T) {
	d, _ := testDeps(t)

	schema := &ormschema.Table{
		Name:    "data",
		Columns: []ormschema.Column{{Name: "id", Type: "integer", Key: true}},
	}
	schemaJSON, _ := json.Marshal(schema)
	files := map[string][]byte{
		"schemas/data.json": schemaJSON,
	}

	if err := applyTemplateFiles(d, files, "", nil, "Test", []string{"data"}, true); err != nil {
		t.Fatal(err)
	}
	if d.DB.GetMeta("template_require_email") != "1" {
		t.Fatal("template_require_email should be '1' when requireEmail=true")
	}

	if err := applyTemplateFiles(d, files, "", nil, "Test", []string{"data"}, false); err != nil {
		t.Fatal(err)
	}
	if d.DB.GetMeta("template_require_email") != "" {
		t.Fatal("template_require_email should be empty when requireEmail=false")
	}
}

func TestNeedsGroupFromUpdateDeleteAccess(t *testing.T) {
	d := testDepsWithGroups(t)

	schema := &ormschema.Table{
		Name:    "items",
		Columns: []ormschema.Column{{Name: "id", Type: "integer", Key: true}},
		Access:  &ormschema.Access{Read: "open", Insert: "owner", Update: "group", Delete: "owner"},
	}
	schemaJSON, _ := json.Marshal(schema)
	files := map[string][]byte{
		"schemas/items.json": schemaJSON,
	}

	if err := applyTemplateFiles(d, files, "", nil, "GroupUpdate", []string{"items"}, false); err != nil {
		t.Fatal(err)
	}

	groupID := d.DB.GetMeta("template_group_id")
	if groupID == "" {
		t.Fatal("group should be created when update access is 'group'")
	}
}

func TestNeedsGroupFromRolesMap(t *testing.T) {
	d := testDepsWithGroups(t)

	schema := &ormschema.Table{
		Name:    "items",
		Columns: []ormschema.Column{{Name: "id", Type: "integer", Key: true}},
		Access:  &ormschema.Access{Read: "open", Insert: "owner", Update: "owner", Delete: "owner"},
		Roles:   map[string]ormschema.RoleAccess{"editor": {Read: true, Insert: true}},
	}
	schemaJSON, _ := json.Marshal(schema)
	files := map[string][]byte{
		"schemas/items.json": schemaJSON,
	}

	if err := applyTemplateFiles(d, files, "", nil, "RolesOnly", []string{"items"}, false); err != nil {
		t.Fatal(err)
	}

	groupID := d.DB.GetMeta("template_group_id")
	if groupID == "" {
		t.Fatal("group should be created when schema has roles defined")
	}
}

func TestNoGroupWithoutGroupAccess(t *testing.T) {
	d := testDepsWithGroups(t)

	schema := &ormschema.Table{
		Name:    "items",
		Columns: []ormschema.Column{{Name: "id", Type: "integer", Key: true}},
		Access:  &ormschema.Access{Read: "open", Insert: "owner", Update: "owner", Delete: "owner"},
	}
	schemaJSON, _ := json.Marshal(schema)
	files := map[string][]byte{
		"schemas/items.json": schemaJSON,
	}

	if err := applyTemplateFiles(d, files, "", nil, "NoGroup", []string{"items"}, false); err != nil {
		t.Fatal(err)
	}

	groupID := d.DB.GetMeta("template_group_id")
	if groupID != "" {
		t.Fatal("group should NOT be created when no access uses 'group' and no roles defined")
	}
}
