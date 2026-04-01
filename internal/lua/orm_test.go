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

func setupEngineWithDB(t *testing.T, scripts map[string]string) (*Engine, *storage.DB) {
	t.Helper()
	dir := t.TempDir()
	luaDir := filepath.Join(dir, "site", "lua")
	funcDir := filepath.Join(luaDir, "functions")
	os.MkdirAll(funcDir, 0755)

	for name, src := range scripts {
		var path string
		if filepath.Dir(name) == "functions" {
			path = filepath.Join(funcDir, filepath.Base(name))
		} else {
			path = filepath.Join(luaDir, name)
		}
		os.WriteFile(path, []byte(src), 0644)
	}

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
	t.Cleanup(func() { e.Close() })
	return e, db
}

func TestSchemaCount(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/counter.lua": `
-- @rate_limit 0
function call(req)
  local n, err = goop.schema.count("items")
  if err then return {error = err} end
  return {count = n}
end
`,
	})

	tbl := &schema.Table{
		Name:      "items",
		SystemKey: true,
		Columns:   []schema.Column{{Name: "name", Type: "text", Required: true}},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	result, err := e.CallFunction(context.Background(), "self-peer-id", "counter", nil)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["count"] != float64(0) {
		t.Fatalf("expected count=0, got %v", m["count"])
	}

	db.OrmInsert("items", "peer1", "", map[string]any{"name": "apple"})
	db.OrmInsert("items", "peer1", "", map[string]any{"name": "banana"})

	result, err = e.CallFunction(context.Background(), "self-peer-id", "counter", nil)
	if err != nil {
		t.Fatal(err)
	}
	m = result.(map[string]any)
	if m["count"] != float64(2) {
		t.Fatalf("expected count=2, got %v", m["count"])
	}
}

func TestSchemaSeed(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/seed.lua": `
-- @rate_limit 0
function call(req)
  return goop.schema.seed("items", {
    {name = "apple"},
    {name = "banana"},
    {name = "cherry"},
  })
end
`,
	})

	tbl := &schema.Table{
		Name:      "items",
		SystemKey: true,
		Columns:   []schema.Column{{Name: "name", Type: "text", Required: true}},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	result, err := e.CallFunction(context.Background(), "self-peer-id", "seed", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != float64(3) {
		t.Fatalf("expected 3 rows seeded, got %v", result)
	}

	rows, err := db.OrmList("items", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows in DB, got %d", len(rows))
	}
}

func TestSchemaSeedIdempotent(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/seed.lua": `
-- @rate_limit 0
function call(req)
  return goop.schema.seed("items", {
    {name = "only_once"},
  })
end
`,
	})

	tbl := &schema.Table{
		Name:      "items",
		SystemKey: true,
		Columns:   []schema.Column{{Name: "name", Type: "text", Required: true}},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	result1, _ := e.CallFunction(context.Background(), "self-peer-id", "seed", nil)
	if result1 != float64(1) {
		t.Fatalf("first call: expected 1, got %v", result1)
	}

	result2, _ := e.CallFunction(context.Background(), "self-peer-id", "seed", nil)
	if result2 != float64(0) {
		t.Fatalf("second call: expected 0 (idempotent), got %v", result2)
	}

	rows, _ := db.OrmList("items", 0)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (not doubled), got %d", len(rows))
	}
}

func TestSchemaSeedOwnership(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/seed.lua": `
-- @rate_limit 0
function call(req)
  return goop.schema.seed("items", {
    {name = "seeded_item"},
  })
end
`,
	})

	tbl := &schema.Table{
		Name:      "items",
		SystemKey: true,
		Columns:   []schema.Column{{Name: "name", Type: "text", Required: true}},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	e.CallFunction(context.Background(), "self-peer-id", "seed", nil)

	rows, _ := db.OrmList("items", 0)
	if len(rows) != 1 {
		t.Fatal("expected 1 row")
	}
	owner, _ := rows[0]["_owner"].(string)
	if owner != "self-peer-id" {
		t.Fatalf("expected _owner='self-peer-id', got %q", owner)
	}
}

func TestSchemaInsertViaLua(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/inserter.lua": `
-- @rate_limit 0
function call(req)
  local id, err = goop.schema.insert("items", {name = req.params.name})
  if err then return {error = err} end
  return {id = id}
end
`,
	})

	tbl := &schema.Table{
		Name:      "items",
		SystemKey: true,
		Columns:   []schema.Column{{Name: "name", Type: "text", Required: true}},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	result, err := e.CallFunction(context.Background(), "self-peer-id", "inserter", map[string]any{"name": "test"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["error"] != nil {
		t.Fatalf("insert error: %v", m["error"])
	}
	if m["id"] == nil || m["id"] == float64(0) {
		t.Fatal("expected non-zero id")
	}
}

func TestSchemaFind(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/finder.lua": `
-- @rate_limit 0
function call(req)
  local rows = goop.schema.find("posts", {
    where = "published = ?",
    args = { 1 },
    fields = { "title", "slug" },
    order = "_id DESC",
    limit = 10,
  })
  return { posts = rows or {} }
end
`,
	})

	tbl := &schema.Table{
		Name:      "posts",
		SystemKey: true,
		Columns: []schema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "slug", Type: "text"},
			{Name: "published", Type: "integer", Default: 1},
		},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	db.OrmInsert("posts", "p1", "", map[string]any{"title": "First", "slug": "first", "published": 1})
	db.OrmInsert("posts", "p1", "", map[string]any{"title": "Draft", "slug": "draft", "published": 0})
	db.OrmInsert("posts", "p1", "", map[string]any{"title": "Second", "slug": "second", "published": 1})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "finder", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	posts, ok := m["posts"].([]any)
	if !ok {
		t.Fatalf("expected posts to be array, got %T: %v", m["posts"], m["posts"])
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 published posts, got %d", len(posts))
	}
	first := posts[0].(map[string]any)
	if first["title"] != "Second" {
		t.Fatalf("expected first post 'Second' (DESC order), got %q", first["title"])
	}
	if first["slug"] != "second" {
		t.Fatalf("expected slug 'second', got %q", first["slug"])
	}
}

func TestSchemaFindEmpty(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/finder.lua": `
-- @rate_limit 0
function call(req)
  local rows = goop.schema.find("posts", {
    where = "published = 1",
  })
  return { posts = rows or {} }
end
`,
	})

	tbl := &schema.Table{
		Name:      "posts",
		SystemKey: true,
		Columns: []schema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "published", Type: "integer", Default: 1},
		},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	result, err := e.CallFunction(context.Background(), "self-peer-id", "finder", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	posts, ok := m["posts"].([]any)
	if !ok {
		t.Fatalf("empty result should be [] array, got %T: %v", m["posts"], m["posts"])
	}
	if len(posts) != 0 {
		t.Fatalf("expected 0 posts, got %d", len(posts))
	}
}

func TestSchemaFindOne(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/getter.lua": `
-- @rate_limit 0
function call(req)
  local row = goop.schema.find_one("posts", {
    where = "slug = ?",
    args = { req.params.slug },
    fields = { "_id", "title", "slug" },
  })
  if not row then return { found = false } end
  return { found = true, post = row }
end
`,
	})

	tbl := &schema.Table{
		Name:      "posts",
		SystemKey: true,
		Columns: []schema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "slug", Type: "text"},
			{Name: "published", Type: "integer", Default: 1},
		},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	db.OrmInsert("posts", "p1", "", map[string]any{"title": "Hello", "slug": "hello"})
	db.OrmInsert("posts", "p1", "", map[string]any{"title": "World", "slug": "world"})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "getter", map[string]any{"slug": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["found"] != true {
		t.Fatal("expected found=true")
	}
	post := m["post"].(map[string]any)
	if post["title"] != "Hello" {
		t.Fatalf("expected title 'Hello', got %q", post["title"])
	}
	if post["slug"] != "hello" {
		t.Fatalf("expected slug 'hello', got %q", post["slug"])
	}
}

func TestSchemaFindOneNotFound(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/getter.lua": `
-- @rate_limit 0
function call(req)
  local row = goop.schema.find_one("posts", {
    where = "slug = ?",
    args = { req.params.slug },
  })
  if not row then return { found = false } end
  return { found = true, post = row }
end
`,
	})

	tbl := &schema.Table{
		Name:      "posts",
		SystemKey: true,
		Columns:   []schema.Column{{Name: "title", Type: "text"}, {Name: "slug", Type: "text"}},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	result, err := e.CallFunction(context.Background(), "self-peer-id", "getter", map[string]any{"slug": "nope"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["found"] != false {
		t.Fatal("expected found=false for missing slug")
	}
}

func TestSchemaFindWithOrder(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/ordered.lua": `
-- @rate_limit 0
function call(req)
  local rows = goop.schema.find("items", {
    order = "priority DESC, name ASC",
    limit = 5,
  })
  return rows or {}
end
`,
	})

	tbl := &schema.Table{
		Name:      "items",
		SystemKey: true,
		Columns: []schema.Column{
			{Name: "name", Type: "text", Required: true},
			{Name: "priority", Type: "integer", Default: 0},
		},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	db.OrmInsert("items", "p1", "", map[string]any{"name": "low", "priority": 1})
	db.OrmInsert("items", "p1", "", map[string]any{"name": "high", "priority": 10})
	db.OrmInsert("items", "p1", "", map[string]any{"name": "mid", "priority": 5})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "ordered", nil)
	if err != nil {
		t.Fatal(err)
	}
	arr := result.([]any)
	if len(arr) != 3 {
		t.Fatalf("expected 3 items, got %d", len(arr))
	}
	first := arr[0].(map[string]any)
	if first["name"] != "high" {
		t.Fatalf("expected first item 'high' (priority DESC), got %q", first["name"])
	}
}

func TestSchemaAggregate(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/stats.lua": `
-- @rate_limit 0
function call(req)
  local rows = goop.schema.aggregate("scores", "COUNT(*) as n, SUM(score) as total")
  if not rows or #rows == 0 then return {n = 0, total = 0} end
  return rows[1]
end
`,
	})

	tbl := &schema.Table{
		Name: "scores", SystemKey: true,
		Columns: []schema.Column{
			{Name: "score", Type: "integer", Required: true},
			{Name: "player", Type: "text"},
		},
	}
	db.CreateTableORM(tbl)
	db.OrmInsert("scores", "p1", "", map[string]any{"score": 100, "player": "alice"})
	db.OrmInsert("scores", "p1", "", map[string]any{"score": 250, "player": "bob"})
	db.OrmInsert("scores", "p1", "", map[string]any{"score": 50, "player": "alice"})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "stats", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["n"] != float64(3) {
		t.Fatalf("expected count=3, got %v", m["n"])
	}
	if m["total"] != float64(400) {
		t.Fatalf("expected total=400, got %v", m["total"])
	}
}

func TestSchemaAggregateGroupBy(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/grouped.lua": `
-- @rate_limit 0
function call(req)
  return goop.schema.aggregate("scores", "player, SUM(score) as total", {
    group_by = "player",
  })
end
`,
	})

	tbl := &schema.Table{
		Name: "scores", SystemKey: true,
		Columns: []schema.Column{
			{Name: "score", Type: "integer", Required: true},
			{Name: "player", Type: "text", Required: true},
		},
	}
	db.CreateTableORM(tbl)
	db.OrmInsert("scores", "p1", "", map[string]any{"score": 100, "player": "alice"})
	db.OrmInsert("scores", "p1", "", map[string]any{"score": 250, "player": "bob"})
	db.OrmInsert("scores", "p1", "", map[string]any{"score": 50, "player": "alice"})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "grouped", nil)
	if err != nil {
		t.Fatal(err)
	}
	arr := result.([]any)
	if len(arr) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(arr))
	}
}

func TestSchemaDistinct(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/cats.lua": `
-- @rate_limit 0
function call(req)
  return goop.schema.distinct("notes", "category")
end
`,
	})

	tbl := &schema.Table{
		Name: "notes", SystemKey: true,
		Columns: []schema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "category", Type: "text", Default: "general"},
		},
	}
	db.CreateTableORM(tbl)
	db.OrmInsert("notes", "p1", "", map[string]any{"title": "a", "category": "selling"})
	db.OrmInsert("notes", "p1", "", map[string]any{"title": "b", "category": "event"})
	db.OrmInsert("notes", "p1", "", map[string]any{"title": "c", "category": "selling"})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "cats", nil)
	if err != nil {
		t.Fatal(err)
	}
	arr := result.([]any)
	if len(arr) != 2 {
		t.Fatalf("expected 2 distinct categories, got %d: %v", len(arr), arr)
	}
}

func TestSchemaUpdateWhere(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/mover.lua": `
-- @rate_limit 0
function call(req)
  return goop.schema.update_where("cards", {position = 99}, {
    where = "column_id = ?",
    args = { req.params.col },
  })
end
`,
	})

	tbl := &schema.Table{
		Name: "cards", SystemKey: true,
		Columns: []schema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "column_id", Type: "integer", Required: true},
			{Name: "position", Type: "integer", Default: 0},
		},
	}
	db.CreateTableORM(tbl)
	db.OrmInsert("cards", "p1", "", map[string]any{"title": "a", "column_id": 1, "position": 0})
	db.OrmInsert("cards", "p1", "", map[string]any{"title": "b", "column_id": 1, "position": 1})
	db.OrmInsert("cards", "p1", "", map[string]any{"title": "c", "column_id": 2, "position": 0})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "mover", map[string]any{"col": 1})
	if err != nil {
		t.Fatal(err)
	}
	if result != float64(2) {
		t.Fatalf("expected 2 rows updated, got %v", result)
	}

	rows, _ := db.OrmList("cards", 0)
	for _, r := range rows {
		if r["column_id"] == int64(1) && r["position"] != int64(99) {
			t.Fatalf("column_id=1 card should have position=99, got %v", r["position"])
		}
		if r["column_id"] == int64(2) && r["position"] != int64(0) {
			t.Fatal("column_id=2 card should be unchanged")
		}
	}
}

func TestSchemaDeleteWhere(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/cleaner.lua": `
-- @rate_limit 0
function call(req)
  return goop.schema.delete_where("cards", {
    where = "column_id = ?",
    args = { req.params.col },
  })
end
`,
	})

	tbl := &schema.Table{
		Name: "cards", SystemKey: true,
		Columns: []schema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "column_id", Type: "integer", Required: true},
		},
	}
	db.CreateTableORM(tbl)
	db.OrmInsert("cards", "p1", "", map[string]any{"title": "a", "column_id": 1})
	db.OrmInsert("cards", "p1", "", map[string]any{"title": "b", "column_id": 1})
	db.OrmInsert("cards", "p1", "", map[string]any{"title": "c", "column_id": 2})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "cleaner", map[string]any{"col": 1})
	if err != nil {
		t.Fatal(err)
	}
	if result != float64(2) {
		t.Fatalf("expected 2 rows deleted, got %v", result)
	}

	rows, _ := db.OrmList("cards", 0)
	if len(rows) != 1 {
		t.Fatalf("expected 1 remaining row, got %d", len(rows))
	}
	if rows[0]["title"] != "c" {
		t.Fatalf("remaining row should be 'c', got %v", rows[0]["title"])
	}
}

func TestSchemaUpsert(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/config.lua": `
-- @rate_limit 0
function call(req)
  return goop.schema.upsert("config", "key", {
    key = req.params.key,
    value = req.params.value,
  })
end
`,
	})

	tbl := &schema.Table{
		Name: "config", SystemKey: true,
		Columns: []schema.Column{
			{Name: "key", Type: "text", Required: true},
			{Name: "value", Type: "text", Default: ""},
		},
	}
	db.CreateTableORM(tbl)

	// First call — insert
	_, err := e.CallFunction(context.Background(), "self-peer-id", "config", map[string]any{"key": "title", "value": "My Blog"})
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := db.OrmList("config", 0)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after insert, got %d", len(rows))
	}
	if rows[0]["value"] != "My Blog" {
		t.Fatalf("expected value 'My Blog', got %v", rows[0]["value"])
	}

	// Second call — update (same key)
	_, err = e.CallFunction(context.Background(), "self-peer-id", "config", map[string]any{"key": "title", "value": "New Title"})
	if err != nil {
		t.Fatal(err)
	}

	rows, _ = db.OrmList("config", 0)
	if len(rows) != 1 {
		t.Fatalf("expected still 1 row after upsert, got %d", len(rows))
	}
	if rows[0]["value"] != "New Title" {
		t.Fatalf("expected value 'New Title', got %v", rows[0]["value"])
	}
}

func TestSchemaGetBy(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/lookup.lua": `
-- @rate_limit 0
function call(req)
  local row = goop.schema.get_by("posts", "slug", req.params.slug)
  if not row then return { found = false } end
  return { found = true, title = row.title }
end
`,
	})

	tbl := &schema.Table{
		Name: "posts", SystemKey: true,
		Columns: []schema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "slug", Type: "text"},
		},
	}
	db.CreateTableORM(tbl)
	db.OrmInsert("posts", "p1", "", map[string]any{"title": "Hello", "slug": "hello"})
	db.OrmInsert("posts", "p1", "", map[string]any{"title": "World", "slug": "world"})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "lookup", map[string]any{"slug": "world"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["found"] != true {
		t.Fatal("expected found=true")
	}
	if m["title"] != "World" {
		t.Fatalf("expected 'World', got %q", m["title"])
	}

	result, _ = e.CallFunction(context.Background(), "self-peer-id", "lookup", map[string]any{"slug": "nope"})
	m = result.(map[string]any)
	if m["found"] != false {
		t.Fatal("expected found=false for missing slug")
	}
}

func TestSchemaGetByID(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/byid.lua": `
-- @rate_limit 0
function call(req)
  local row = goop.schema.get_by("items", "_id", req.params.id)
  if not row then return { found = false } end
  return { found = true, name = row.name }
end
`,
	})

	tbl := &schema.Table{
		Name: "items", SystemKey: true,
		Columns: []schema.Column{{Name: "name", Type: "text", Required: true}},
	}
	db.CreateTableORM(tbl)
	db.OrmInsert("items", "p1", "", map[string]any{"name": "first"})
	db.OrmInsert("items", "p1", "", map[string]any{"name": "second"})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "byid", map[string]any{"id": 2})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["name"] != "second" {
		t.Fatalf("expected 'second', got %v", m["name"])
	}
}

func TestSchemaExists(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/checker.lua": `
-- @rate_limit 0
function call(req)
  local any_exist = goop.schema.exists("posts")
  local published = goop.schema.exists("posts", {
    where = "published = ?", args = { 1 },
  })
  local drafts = goop.schema.exists("posts", {
    where = "published = ?", args = { 0 },
  })
  return { any = any_exist, published = published, drafts = drafts }
end
`,
	})

	tbl := &schema.Table{
		Name: "posts", SystemKey: true,
		Columns: []schema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "published", Type: "integer", Default: 1},
		},
	}
	db.CreateTableORM(tbl)

	result, _ := e.CallFunction(context.Background(), "self-peer-id", "checker", nil)
	m := result.(map[string]any)
	if m["any"] != false {
		t.Fatal("empty table: exists should be false")
	}

	db.OrmInsert("posts", "p1", "", map[string]any{"title": "pub", "published": 1})

	result, _ = e.CallFunction(context.Background(), "self-peer-id", "checker", nil)
	m = result.(map[string]any)
	if m["any"] != true {
		t.Fatal("after insert: exists should be true")
	}
	if m["published"] != true {
		t.Fatal("published=1 row exists")
	}
	if m["drafts"] != false {
		t.Fatal("no published=0 rows")
	}
}

func TestSchemaPluck(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/titles.lua": `
-- @rate_limit 0
function call(req)
  return goop.schema.pluck("posts", "title", {
    where = "published = 1",
    order = "_id ASC",
  })
end
`,
	})

	tbl := &schema.Table{
		Name: "posts", SystemKey: true,
		Columns: []schema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "published", Type: "integer", Default: 1},
		},
	}
	db.CreateTableORM(tbl)
	db.OrmInsert("posts", "p1", "", map[string]any{"title": "First", "published": 1})
	db.OrmInsert("posts", "p1", "", map[string]any{"title": "Draft", "published": 0})
	db.OrmInsert("posts", "p1", "", map[string]any{"title": "Second", "published": 1})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "titles", nil)
	if err != nil {
		t.Fatal(err)
	}
	arr := result.([]any)
	if len(arr) != 2 {
		t.Fatalf("expected 2 titles, got %d", len(arr))
	}
	if arr[0] != "First" || arr[1] != "Second" {
		t.Fatalf("expected [First, Second], got %v", arr)
	}
}

func TestSchemaPluckEmpty(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/empty.lua": `
-- @rate_limit 0
function call(req)
  return goop.schema.pluck("items", "name")
end
`,
	})

	tbl := &schema.Table{
		Name: "items", SystemKey: true,
		Columns: []schema.Column{{Name: "name", Type: "text"}},
	}
	db.CreateTableORM(tbl)

	result, err := e.CallFunction(context.Background(), "self-peer-id", "empty", nil)
	if err != nil {
		t.Fatal(err)
	}
	arr, ok := result.([]any)
	if !ok {
		t.Fatalf("expected [] array, got %T: %v", result, result)
	}
	if len(arr) != 0 {
		t.Fatalf("expected empty array, got %d", len(arr))
	}
}

func TestSchemaUpdateWhereRequiresWhere(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/bad.lua": `
-- @rate_limit 0
function call(req)
  local n, err = goop.schema.update_where("items", {name = "x"}, {})
  if err then return {error = err} end
  return {n = n}
end
`,
	})

	tbl := &schema.Table{
		Name: "items", SystemKey: true,
		Columns: []schema.Column{{Name: "name", Type: "text"}},
	}
	db.CreateTableORM(tbl)
	db.OrmInsert("items", "p1", "", map[string]any{"name": "a"})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "bad", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["error"] == nil {
		t.Fatal("update_where without where clause should return error")
	}
}

func TestSchemaDeleteWhereRequiresWhere(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/bad.lua": `
-- @rate_limit 0
function call(req)
  local n, err = goop.schema.delete_where("items", {})
  if err then return {error = err} end
  return {n = n}
end
`,
	})

	tbl := &schema.Table{
		Name: "items", SystemKey: true,
		Columns: []schema.Column{{Name: "name", Type: "text"}},
	}
	db.CreateTableORM(tbl)
	db.OrmInsert("items", "p1", "", map[string]any{"name": "a"})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "bad", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["error"] == nil {
		t.Fatal("delete_where without where clause should return error")
	}

	rows, _ := db.OrmList("items", 0)
	if len(rows) != 1 {
		t.Fatal("row should still exist — delete was blocked")
	}
}
