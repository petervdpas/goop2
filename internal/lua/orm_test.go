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

func TestOrmNotORM(t *testing.T) {
	e, _ := setupEngineWithDB(t, map[string]string{
		"functions/bad.lua": `
-- @rate_limit 0
function call(req)
  local handle, err = goop.orm("nonexistent")
  if err then return {error = err} end
  return {name = handle.name}
end
`,
	})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "bad", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["error"] == nil {
		t.Fatal("expected error for non-ORM table")
	}
}

func TestOrmProperties(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/info.lua": `
-- @rate_limit 0
function call(req)
  local posts = goop.orm("posts")
  return {
    name = posts.name,
    system_key = posts.system_key,
    read = posts.access.read,
    insert = posts.access.insert,
    update = posts.access.update,
    delete = posts.access.delete,
    col_count = #posts.columns,
    first_col = posts.columns[1].name,
    first_type = posts.columns[1].type,
    first_required = posts.columns[1].required,
    second_col = posts.columns[2].name,
  }
end
`,
	})

	db.CreateTableORM(&schema.Table{
		Name:      "posts",
		SystemKey: true,
		Columns: []schema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "body", Type: "text"},
		},
		Access: &schema.Access{Read: "open", Insert: "group", Update: "owner", Delete: "owner"},
	})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "info", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["name"] != "posts" {
		t.Fatalf("expected name 'posts', got %v", m["name"])
	}
	if m["system_key"] != true {
		t.Fatalf("expected system_key=true, got %v", m["system_key"])
	}
	if m["read"] != "open" {
		t.Fatalf("expected access.read='open', got %v", m["read"])
	}
	if m["insert"] != "group" {
		t.Fatalf("expected access.insert='group', got %v", m["insert"])
	}
	if m["update"] != "owner" {
		t.Fatalf("expected access.update='owner', got %v", m["update"])
	}
	if m["delete"] != "owner" {
		t.Fatalf("expected access.delete='owner', got %v", m["delete"])
	}
	if m["col_count"] != float64(2) {
		t.Fatalf("expected 2 columns, got %v", m["col_count"])
	}
	if m["first_col"] != "title" {
		t.Fatalf("expected first column 'title', got %v", m["first_col"])
	}
	if m["first_type"] != "text" {
		t.Fatalf("expected first type 'text', got %v", m["first_type"])
	}
	if m["first_required"] != true {
		t.Fatalf("expected first_required=true, got %v", m["first_required"])
	}
	if m["second_col"] != "body" {
		t.Fatalf("expected second column 'body', got %v", m["second_col"])
	}
}

func TestOrmCount(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/counter.lua": `
-- @rate_limit 0
function call(req)
  local items = goop.orm("items")
  local n, err = items:count()
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
	db.CreateTableORM(tbl)

	result, err := e.CallFunction(context.Background(), "self-peer-id", "counter", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
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

func TestOrmSeed(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/seed.lua": `
-- @rate_limit 0
function call(req)
  local items = goop.orm("items")
  return items:seed({
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
	db.CreateTableORM(tbl)

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

func TestOrmSeedIdempotent(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/seed.lua": `
-- @rate_limit 0
function call(req)
  local items = goop.orm("items")
  return items:seed({
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
	db.CreateTableORM(tbl)

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

func TestOrmSeedOwnership(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/seed.lua": `
-- @rate_limit 0
function call(req)
  local items = goop.orm("items")
  return items:seed({
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
	db.CreateTableORM(tbl)

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

func TestOrmInsert(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/inserter.lua": `
-- @rate_limit 0
function call(req)
  local items = goop.orm("items")
  local id, err = items:insert({name = req.params.name})
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
	db.CreateTableORM(tbl)

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

func TestOrmFind(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/finder.lua": `
-- @rate_limit 0
function call(req)
  local posts = goop.orm("posts")
  local rows = posts:find({
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
	db.CreateTableORM(tbl)

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

func TestOrmFindEmpty(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/finder.lua": `
-- @rate_limit 0
function call(req)
  local posts = goop.orm("posts")
  local rows = posts:find({
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
	db.CreateTableORM(tbl)

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

func TestOrmFindOne(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/getter.lua": `
-- @rate_limit 0
function call(req)
  local posts = goop.orm("posts")
  local row = posts:find_one({
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
	db.CreateTableORM(tbl)

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

func TestOrmFindOneNotFound(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/getter.lua": `
-- @rate_limit 0
function call(req)
  local posts = goop.orm("posts")
  local row = posts:find_one({
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
	db.CreateTableORM(tbl)

	result, err := e.CallFunction(context.Background(), "self-peer-id", "getter", map[string]any{"slug": "nope"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["found"] != false {
		t.Fatal("expected found=false for missing slug")
	}
}

func TestOrmFindWithOrder(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/ordered.lua": `
-- @rate_limit 0
function call(req)
  local items = goop.orm("items")
  local rows = items:find({
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
	db.CreateTableORM(tbl)

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

func TestOrmAggregate(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/stats.lua": `
-- @rate_limit 0
function call(req)
  local scores = goop.orm("scores")
  local rows = scores:aggregate("COUNT(*) as n, SUM(score) as total")
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

func TestOrmAggregateGroupBy(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/grouped.lua": `
-- @rate_limit 0
function call(req)
  local scores = goop.orm("scores")
  return scores:aggregate("player, SUM(score) as total", {
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

func TestOrmDistinct(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/cats.lua": `
-- @rate_limit 0
function call(req)
  local notes = goop.orm("notes")
  return notes:distinct("category")
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

func TestOrmUpdateWhere(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/mover.lua": `
-- @rate_limit 0
function call(req)
  local cards = goop.orm("cards")
  return cards:update_where({position = 99}, {
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

func TestOrmDeleteWhere(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/cleaner.lua": `
-- @rate_limit 0
function call(req)
  local cards = goop.orm("cards")
  return cards:delete_where({
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

func TestOrmUpsert(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/config.lua": `
-- @rate_limit 0
function call(req)
  local cfg = goop.orm("config")
  return cfg:upsert("key", {
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

func TestOrmGetBy(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/lookup.lua": `
-- @rate_limit 0
function call(req)
  local posts = goop.orm("posts")
  local row = posts:get_by("slug", req.params.slug)
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

func TestOrmGetByID(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/byid.lua": `
-- @rate_limit 0
function call(req)
  local items = goop.orm("items")
  local row = items:get_by("_id", req.params.id)
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

func TestOrmExists(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/checker.lua": `
-- @rate_limit 0
function call(req)
  local posts = goop.orm("posts")
  local any_exist = posts:exists()
  local published = posts:exists({
    where = "published = ?", args = { 1 },
  })
  local drafts = posts:exists({
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

func TestOrmPluck(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/titles.lua": `
-- @rate_limit 0
function call(req)
  local posts = goop.orm("posts")
  return posts:pluck("title", {
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

func TestOrmPluckEmpty(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/empty.lua": `
-- @rate_limit 0
function call(req)
  local items = goop.orm("items")
  return items:pluck("name")
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

func TestOrmUpdateWhereRequiresWhere(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/bad.lua": `
-- @rate_limit 0
function call(req)
  local items = goop.orm("items")
  local n, err = items:update_where({name = "x"}, {})
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

func TestOrmDeleteWhereRequiresWhere(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/bad.lua": `
-- @rate_limit 0
function call(req)
  local items = goop.orm("items")
  local n, err = items:delete_where({})
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

func TestOrmValidate(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/val.lua": `
-- @rate_limit 0
function call(req)
  local items = goop.orm("items")
  local ok1, err1 = items:validate({name = "hello", count = 5})
  local ok2, err2 = items:validate({name = "hello", count = "not_a_number"})
  return {
    valid_ok = ok1,
    valid_err = err1,
    invalid_ok = ok2,
    invalid_err = err2,
  }
end
`,
	})

	tbl := &schema.Table{
		Name: "items", SystemKey: true,
		Columns: []schema.Column{
			{Name: "name", Type: "text", Required: true},
			{Name: "count", Type: "integer"},
		},
	}
	db.CreateTableORM(tbl)

	result, err := e.CallFunction(context.Background(), "self-peer-id", "val", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["valid_ok"] != true {
		t.Fatalf("expected valid data ok=true, got %v", m["valid_ok"])
	}
	if m["invalid_ok"] != false {
		t.Fatalf("expected invalid data ok=false, got %v", m["invalid_ok"])
	}
	if m["invalid_err"] == nil {
		t.Fatal("expected error message for invalid data")
	}
}

func TestOrmCRUDIntegration(t *testing.T) {
	e, db := setupEngineWithDB(t, map[string]string{
		"functions/crud.lua": `
-- @rate_limit 0
function call(req)
  local posts = goop.orm("posts")

  local id = posts:insert({title = "Hello", body = "World", published = 1})
  posts:insert({title = "Draft", body = "Hidden", published = 0})

  local rows = posts:find({where = "published = ?", args = {1}})
  local row = posts:find_one({where = "title = ?", args = {"Hello"}})
  local n = posts:count()
  local got = posts:get(id)
  local by = posts:get_by("title", "Hello")
  local ex = posts:exists({where = "published = ?", args = {1}})
  local titles = posts:pluck("title")
  local all = posts:list(10)

  posts:update(id, {title = "Updated"})
  local after = posts:get(id)

  posts:delete(id + 1)
  local final_count = posts:count()

  return {
    insert_id = id,
    find_count = #rows,
    find_one_title = row and row.title,
    total = n,
    got_title = got and got.title,
    by_title = by and by.title,
    exists = ex,
    titles_count = #titles,
    list_count = #all,
    updated_title = after and after.title,
    final_count = final_count,
  }
end
`,
	})

	db.CreateTableORM(&schema.Table{
		Name:      "posts",
		SystemKey: true,
		Columns: []schema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "body", Type: "text"},
			{Name: "published", Type: "integer", Default: 1},
		},
		Access: &schema.Access{Read: "open", Insert: "group", Update: "owner", Delete: "owner"},
	})

	result, err := e.CallFunction(context.Background(), "self-peer-id", "crud", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["insert_id"] == nil || m["insert_id"] == float64(0) {
		t.Fatal("expected non-zero insert id")
	}
	if m["find_count"] != float64(1) {
		t.Fatalf("expected 1 published, got %v", m["find_count"])
	}
	if m["find_one_title"] != "Hello" {
		t.Fatalf("expected find_one 'Hello', got %v", m["find_one_title"])
	}
	if m["total"] != float64(2) {
		t.Fatalf("expected 2 total, got %v", m["total"])
	}
	if m["got_title"] != "Hello" {
		t.Fatalf("expected get 'Hello', got %v", m["got_title"])
	}
	if m["by_title"] != "Hello" {
		t.Fatalf("expected get_by 'Hello', got %v", m["by_title"])
	}
	if m["exists"] != true {
		t.Fatalf("expected exists=true, got %v", m["exists"])
	}
	if m["titles_count"] != float64(2) {
		t.Fatalf("expected 2 titles, got %v", m["titles_count"])
	}
	if m["list_count"] != float64(2) {
		t.Fatalf("expected 2 in list, got %v", m["list_count"])
	}
	if m["updated_title"] != "Updated" {
		t.Fatalf("expected updated 'Updated', got %v", m["updated_title"])
	}
	if m["final_count"] != float64(1) {
		t.Fatalf("expected 1 after delete, got %v", m["final_count"])
	}
}

