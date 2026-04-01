package lua

import (
	"context"
	_ "embed"
	"os"
	"path/filepath"
	"testing"

	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
)

//go:embed testdata/templates/blog_api.lua
var blogAPILua string

func setupBlogEngine(t *testing.T) (*Engine, *storage.DB) {
	t.Helper()

	dir := t.TempDir()
	funcDir := filepath.Join(dir, "site", "lua", "functions")
	os.MkdirAll(funcDir, 0755)
	os.WriteFile(filepath.Join(funcDir, "blog.lua"), []byte(blogAPILua), 0644)

	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	db.CreateTableORM(&schema.Table{
		Name:      "posts",
		SystemKey: true,
		Columns: []schema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "body", Type: "text", Required: true},
			{Name: "author_name", Type: "text", Default: ""},
			{Name: "slug", Type: "text"},
			{Name: "published", Type: "integer", Default: 1},
		},
		Access: &schema.Access{Read: "open", Insert: "group", Update: "owner", Delete: "owner"},
	})
	db.CreateTableORM(&schema.Table{
		Name:      "blog_config",
		SystemKey: true,
		Columns: []schema.Column{
			{Name: "key", Type: "text", Required: true},
			{Name: "value", Type: "text", Default: ""},
		},
		Access: &schema.Access{Read: "open", Insert: "owner", Update: "owner", Delete: "owner"},
	})

	cfg := testConfig()
	peers := state.NewPeerTable()
	e, err := NewEngine(cfg, dir, "self-peer-id", func() string { return "TestPeer" }, peers)
	if err != nil {
		t.Fatal(err)
	}
	e.SetDB(db)
	t.Cleanup(func() { e.Close() })
	return e, db
}

func blogCall(t *testing.T, e *Engine, callerID string, params map[string]any) map[string]any {
	t.Helper()
	result, err := e.CallFunction(context.Background(), callerID, "blog", params)
	if err != nil {
		t.Fatalf("blog(%v) error: %v", params["action"], err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("blog(%v) expected map, got %T: %v", params["action"], result, result)
	}
	return m
}

func TestBlogPageEmpty(t *testing.T) {
	e, _ := setupBlogEngine(t)
	m := blogCall(t, e, "self-peer-id", map[string]any{"action": "page"})

	posts := m["posts"].([]any)
	if len(posts) != 0 {
		t.Fatalf("expected 0 posts, got %d", len(posts))
	}
	if m["can_write"] != true {
		t.Fatalf("owner should have can_write=true, got %v", m["can_write"])
	}
	cfg := m["config"].(map[string]any)
	if cfg["title"] != "My Blog" {
		t.Fatalf("expected default title, got %v", cfg["title"])
	}
}

func TestBlogPageNonOwner(t *testing.T) {
	e, _ := setupBlogEngine(t)
	m := blogCall(t, e, "other-peer-id", map[string]any{"action": "page"})
	if m["can_write"] != false {
		t.Fatalf("non-owner should have can_write=false, got %v", m["can_write"])
	}
}

func TestBlogSavePostInsert(t *testing.T) {
	e, db := setupBlogEngine(t)
	m := blogCall(t, e, "self-peer-id", map[string]any{
		"action": "save_post", "title": "Hello World", "body": "First post.",
	})
	if m["id"] == nil || m["id"] == float64(0) {
		t.Fatal("expected non-zero id")
	}
	rows, _ := db.OrmList("posts", 0)
	if len(rows) != 1 {
		t.Fatalf("expected 1 post, got %d", len(rows))
	}
	if rows[0]["slug"] != "hello-world" {
		t.Fatalf("expected slug 'hello-world', got %v", rows[0]["slug"])
	}
}

func TestBlogSavePostUpdate(t *testing.T) {
	e, db := setupBlogEngine(t)
	m := blogCall(t, e, "self-peer-id", map[string]any{
		"action": "save_post", "title": "Original", "body": "Content",
	})
	blogCall(t, e, "self-peer-id", map[string]any{
		"action": "save_post", "id": m["id"], "title": "Updated", "body": "New",
	})
	rows, _ := db.OrmList("posts", 0)
	if len(rows) != 1 {
		t.Fatalf("expected 1 post (update), got %d", len(rows))
	}
	if rows[0]["title"] != "Updated" {
		t.Fatalf("expected 'Updated', got %v", rows[0]["title"])
	}
}

func TestBlogDeletePost(t *testing.T) {
	e, db := setupBlogEngine(t)
	m := blogCall(t, e, "self-peer-id", map[string]any{
		"action": "save_post", "title": "Gone", "body": "Soon",
	})
	blogCall(t, e, "self-peer-id", map[string]any{"action": "delete_post", "id": m["id"]})
	rows, _ := db.OrmList("posts", 0)
	if len(rows) != 0 {
		t.Fatalf("expected 0 posts after delete, got %d", len(rows))
	}
}

func TestBlogGetPost(t *testing.T) {
	e, _ := setupBlogEngine(t)
	blogCall(t, e, "self-peer-id", map[string]any{
		"action": "save_post", "title": "My Article", "body": "Content here",
	})
	m := blogCall(t, e, "self-peer-id", map[string]any{"action": "get_post", "slug": "my-article"})
	if m["found"] != true {
		t.Fatal("expected found=true")
	}
	post := m["post"].(map[string]any)
	if post["title"] != "My Article" {
		t.Fatalf("expected 'My Article', got %v", post["title"])
	}
}

func TestBlogGetPostNotFound(t *testing.T) {
	e, _ := setupBlogEngine(t)
	m := blogCall(t, e, "self-peer-id", map[string]any{"action": "get_post", "slug": "nope"})
	if m["found"] != false {
		t.Fatal("expected found=false")
	}
}

func TestBlogListPosts(t *testing.T) {
	e, _ := setupBlogEngine(t)
	blogCall(t, e, "self-peer-id", map[string]any{"action": "save_post", "title": "A", "body": "1"})
	blogCall(t, e, "self-peer-id", map[string]any{"action": "save_post", "title": "B", "body": "2"})
	m := blogCall(t, e, "self-peer-id", map[string]any{"action": "page"})
	posts := m["posts"].([]any)
	if len(posts) != 2 {
		t.Fatalf("expected 2, got %d", len(posts))
	}
	if posts[0].(map[string]any)["title"] != "B" {
		t.Fatal("expected newest first")
	}
}

func TestBlogSaveConfig(t *testing.T) {
	e, _ := setupBlogEngine(t)
	blogCall(t, e, "self-peer-id", map[string]any{"action": "save_config", "key": "theme", "value": "dark"})
	m := blogCall(t, e, "self-peer-id", map[string]any{"action": "get_config"})
	if m["theme"] != "dark" {
		t.Fatalf("expected theme 'dark', got %v", m["theme"])
	}
	if m["title"] != "My Blog" {
		t.Fatalf("expected default title, got %v", m["title"])
	}
}

func TestBlogPageIntegration(t *testing.T) {
	e, _ := setupBlogEngine(t)
	blogCall(t, e, "self-peer-id", map[string]any{"action": "save_post", "title": "First", "body": "Content"})
	blogCall(t, e, "self-peer-id", map[string]any{"action": "save_config", "key": "theme", "value": "ocean"})

	m := blogCall(t, e, "self-peer-id", map[string]any{"action": "page"})
	posts := m["posts"].([]any)
	if len(posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(posts))
	}
	cfg := m["config"].(map[string]any)
	if cfg["theme"] != "ocean" {
		t.Fatalf("expected theme 'ocean', got %v", cfg["theme"])
	}
}
