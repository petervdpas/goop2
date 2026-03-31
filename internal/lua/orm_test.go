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
