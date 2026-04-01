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

//go:embed testdata/templates/blog.lua
var blogLuaSrc string

// setupBlogEngine creates an engine loaded with the real blog.lua template,
// with the standard posts + blog_config ORM tables.
func setupBlogEngine(t *testing.T) (*Engine, *storage.DB) {
	t.Helper()

	dir := t.TempDir()
	funcDir := filepath.Join(dir, "site", "lua", "functions")
	os.MkdirAll(funcDir, 0755)
	os.WriteFile(filepath.Join(funcDir, "blog.lua"), []byte(blogLuaSrc), 0644)

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
			{Name: "image", Type: "text", Default: ""},
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

	posts, ok := m["posts"].([]any)
	if !ok {
		t.Fatalf("expected posts array, got %T", m["posts"])
	}
	if len(posts) != 0 {
		t.Fatalf("expected 0 posts, got %d", len(posts))
	}
	if m["can_write"] != true {
		t.Fatalf("owner should have can_write=true, got %v", m["can_write"])
	}
	if m["can_admin"] != true {
		t.Fatalf("owner should have can_admin=true, got %v", m["can_admin"])
	}
}

func TestBlogPageNonOwner(t *testing.T) {
	e, _ := setupBlogEngine(t)

	m := blogCall(t, e, "other-peer-id", map[string]any{"action": "page"})

	if m["can_write"] != false {
		t.Fatalf("non-owner should have can_write=false, got %v", m["can_write"])
	}
	if m["can_admin"] != false {
		t.Fatalf("non-owner should have can_admin=false, got %v", m["can_admin"])
	}
}

func TestBlogSavePostInsert(t *testing.T) {
	e, db := setupBlogEngine(t)

	m := blogCall(t, e, "self-peer-id", map[string]any{
		"action": "save_post",
		"title":  "Hello World",
		"body":   "This is my first post.",
	})

	if m["id"] == nil || m["id"] == float64(0) {
		t.Fatal("expected non-zero id from save_post")
	}

	rows, _ := db.OrmList("posts", 0)
	if len(rows) != 1 {
		t.Fatalf("expected 1 post in DB, got %d", len(rows))
	}
	if rows[0]["title"] != "Hello World" {
		t.Fatalf("expected title 'Hello World', got %v", rows[0]["title"])
	}
	if rows[0]["slug"] != "hello-world" {
		t.Fatalf("expected slug 'hello-world', got %v", rows[0]["slug"])
	}
	if rows[0]["published"] != int64(1) {
		t.Fatalf("expected published=1, got %v", rows[0]["published"])
	}
}

func TestBlogSavePostUpdate(t *testing.T) {
	e, db := setupBlogEngine(t)

	m := blogCall(t, e, "self-peer-id", map[string]any{
		"action": "save_post",
		"title":  "Original",
		"body":   "Content",
	})
	id := m["id"]

	blogCall(t, e, "self-peer-id", map[string]any{
		"action": "save_post",
		"id":     id,
		"title":  "Updated Title",
		"body":   "Updated Body",
	})

	rows, _ := db.OrmList("posts", 0)
	if len(rows) != 1 {
		t.Fatalf("expected 1 post (update, not insert), got %d", len(rows))
	}
	if rows[0]["title"] != "Updated Title" {
		t.Fatalf("expected 'Updated Title', got %v", rows[0]["title"])
	}
	if rows[0]["slug"] != "updated-title" {
		t.Fatalf("expected slug 'updated-title', got %v", rows[0]["slug"])
	}
}

func TestBlogDeletePost(t *testing.T) {
	e, db := setupBlogEngine(t)

	m := blogCall(t, e, "self-peer-id", map[string]any{
		"action": "save_post",
		"title":  "To Delete",
		"body":   "Will be removed",
		"image":  "photo.jpg",
	})
	id := m["id"]

	del := blogCall(t, e, "self-peer-id", map[string]any{
		"action": "delete_post",
		"id":     id,
	})
	if del["ok"] != true {
		t.Fatal("expected ok=true from delete")
	}
	if del["image"] != "photo.jpg" {
		t.Fatalf("expected image='photo.jpg' returned, got %v", del["image"])
	}

	rows, _ := db.OrmList("posts", 0)
	if len(rows) != 0 {
		t.Fatalf("expected 0 posts after delete, got %d", len(rows))
	}
}

func TestBlogGetPost(t *testing.T) {
	e, _ := setupBlogEngine(t)

	blogCall(t, e, "self-peer-id", map[string]any{
		"action": "save_post",
		"title":  "My Article",
		"body":   "Some content here",
	})

	m := blogCall(t, e, "self-peer-id", map[string]any{
		"action": "get_post",
		"slug":   "my-article",
	})
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

	m := blogCall(t, e, "self-peer-id", map[string]any{
		"action": "get_post",
		"slug":   "nonexistent",
	})
	if m["found"] != false {
		t.Fatal("expected found=false for missing slug")
	}
}

func TestBlogGetPostByID(t *testing.T) {
	e, _ := setupBlogEngine(t)

	ins := blogCall(t, e, "self-peer-id", map[string]any{
		"action": "save_post",
		"title":  "ID Lookup",
		"body":   "Find by ID",
	})

	m := blogCall(t, e, "self-peer-id", map[string]any{
		"action": "get_post",
		"slug":   ins["id"],
	})
	if m["found"] != true {
		t.Fatal("expected found=true when looking up by _id")
	}
}

func TestBlogListPosts(t *testing.T) {
	e, _ := setupBlogEngine(t)

	blogCall(t, e, "self-peer-id", map[string]any{"action": "save_post", "title": "Post 1", "body": "Body 1"})
	blogCall(t, e, "self-peer-id", map[string]any{"action": "save_post", "title": "Post 2", "body": "Body 2"})

	m := blogCall(t, e, "self-peer-id", map[string]any{"action": "list_posts"})
	posts := m["posts"].([]any)
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}
	first := posts[0].(map[string]any)
	if first["title"] != "Post 2" {
		t.Fatalf("expected newest first (DESC), got %v", first["title"])
	}
}

func TestBlogSaveConfig(t *testing.T) {
	e, _ := setupBlogEngine(t)

	blogCall(t, e, "self-peer-id", map[string]any{
		"action": "save_config",
		"key":    "layout",
		"value":  "grid",
	})
	blogCall(t, e, "self-peer-id", map[string]any{
		"action": "save_config",
		"key":    "accent",
		"value":  "#2d6a9f",
	})

	cfg := blogCall(t, e, "self-peer-id", map[string]any{"action": "get_config"})
	if cfg["layout"] != "grid" {
		t.Fatalf("expected layout='grid', got %v", cfg["layout"])
	}
	if cfg["accent"] != "#2d6a9f" {
		t.Fatalf("expected accent='#2d6a9f', got %v", cfg["accent"])
	}
}

func TestBlogSaveConfigUpsert(t *testing.T) {
	e, db := setupBlogEngine(t)

	blogCall(t, e, "self-peer-id", map[string]any{"action": "save_config", "key": "theme", "value": "dark"})
	blogCall(t, e, "self-peer-id", map[string]any{"action": "save_config", "key": "theme", "value": "light"})

	rows, _ := db.OrmList("blog_config", 0)
	count := 0
	for _, r := range rows {
		if r["key"] == "theme" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 'theme' row (upsert), got %d", count)
	}
}

func TestBlogPageIntegration(t *testing.T) {
	e, _ := setupBlogEngine(t)

	blogCall(t, e, "self-peer-id", map[string]any{"action": "save_post", "title": "First", "body": "Content"})
	blogCall(t, e, "self-peer-id", map[string]any{"action": "save_config", "key": "layout", "value": "grid"})

	m := blogCall(t, e, "self-peer-id", map[string]any{"action": "page"})

	posts := m["posts"].([]any)
	if len(posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(posts))
	}

	config := m["config"].(map[string]any)
	if config["layout"] != "grid" {
		t.Fatalf("expected config.layout='grid', got %v", config["layout"])
	}

	if m["can_write"] != true {
		t.Fatal("owner should have can_write=true")
	}
	if m["can_admin"] != true {
		t.Fatal("owner should have can_admin=true")
	}
}
