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

//go:embed testdata/templates/crud.lua
var crudLua string

//go:embed testdata/templates/config_kv.lua
var configKVLua string

//go:embed testdata/templates/config_row.lua
var configRowLua string

//go:embed testdata/templates/game.lua
var gameLua string

func setupScriptEngine(t *testing.T, name, src string, tables []*schema.Table) (*Engine, *storage.DB) {
	t.Helper()
	dir := t.TempDir()
	funcDir := filepath.Join(dir, "site", "lua", "functions")
	os.MkdirAll(funcDir, 0755)
	os.WriteFile(filepath.Join(funcDir, name+".lua"), []byte(src), 0644)

	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, tbl := range tables {
		if err := db.CreateTableORM(tbl); err != nil {
			t.Fatal(err)
		}
	}

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

func call(t *testing.T, e *Engine, fn string, callerID string, params map[string]any) any {
	t.Helper()
	result, err := e.CallFunction(context.Background(), callerID, fn, params)
	if err != nil {
		t.Fatalf("%s error: %v", fn, err)
	}
	return result
}

func callMap(t *testing.T, e *Engine, fn string, callerID string, params map[string]any) map[string]any {
	t.Helper()
	r := call(t, e, fn, callerID, params)
	m, ok := r.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", r)
	}
	return m
}

// ── CRUD tests ──

var itemsTable = &schema.Table{
	Name: "items", SystemKey: true,
	Columns: []schema.Column{
		{Name: "name", Type: "text", Required: true},
		{Name: "priority", Type: "integer", Default: 0},
	},
	Access: &schema.Access{Read: "open", Insert: "group", Update: "owner", Delete: "owner"},
}

func TestCrudInsertAndGet(t *testing.T) {
	e, _ := setupScriptEngine(t, "crud", crudLua, []*schema.Table{itemsTable})

	m := callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "insert", "name": "apple", "priority": 3})
	id := m["id"]

	m = callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "get", "id": id})
	if m["name"] != "apple" {
		t.Fatalf("expected 'apple', got %v", m["name"])
	}
}

func TestCrudGetBy(t *testing.T) {
	e, _ := setupScriptEngine(t, "crud", crudLua, []*schema.Table{itemsTable})

	callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "insert", "name": "banana"})
	m := callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "get_by", "name": "banana"})
	if m["name"] != "banana" {
		t.Fatalf("expected 'banana', got %v", m["name"])
	}
}

func TestCrudFindAndCount(t *testing.T) {
	e, _ := setupScriptEngine(t, "crud", crudLua, []*schema.Table{itemsTable})

	callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "insert", "name": "a", "priority": 1})
	callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "insert", "name": "b", "priority": 5})
	callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "insert", "name": "c", "priority": 3})

	rows := call(t, e, "crud", "self-peer-id", map[string]any{
		"action": "find",
		"opts":   map[string]any{"order": "priority DESC", "limit": 2},
	}).([]any)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].(map[string]any)["name"] != "b" {
		t.Fatal("expected highest priority first")
	}

	m := callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "count"})
	if m["n"] != float64(3) {
		t.Fatalf("expected count=3, got %v", m["n"])
	}
}

func TestCrudUpdateAndDelete(t *testing.T) {
	e, _ := setupScriptEngine(t, "crud", crudLua, []*schema.Table{itemsTable})

	m := callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "insert", "name": "x"})
	id := m["id"]

	callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "update", "id": id, "data": map[string]any{"name": "y"}})
	m = callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "get", "id": id})
	if m["name"] != "y" {
		t.Fatalf("expected 'y', got %v", m["name"])
	}

	callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "delete", "id": id})
	m = callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "count"})
	if m["n"] != float64(0) {
		t.Fatalf("expected 0 after delete, got %v", m["n"])
	}
}

func TestCrudPluckAndExists(t *testing.T) {
	e, _ := setupScriptEngine(t, "crud", crudLua, []*schema.Table{itemsTable})

	callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "insert", "name": "x"})
	callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "insert", "name": "y"})

	names := call(t, e, "crud", "self-peer-id", map[string]any{"action": "pluck", "column": "name"}).([]any)
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}

	m := callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "exists"})
	if m["yes"] != true {
		t.Fatal("expected exists=true")
	}
}

func TestCrudSeed(t *testing.T) {
	e, db := setupScriptEngine(t, "crud", crudLua, []*schema.Table{itemsTable})

	m := callMap(t, e, "crud", "self-peer-id", map[string]any{
		"action": "seed",
		"rows":   []any{map[string]any{"name": "a"}, map[string]any{"name": "b"}},
	})
	if m["n"] != float64(2) {
		t.Fatalf("expected 2 seeded, got %v", m["n"])
	}

	rows, _ := db.OrmList("items", 0)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestCrudSchema(t *testing.T) {
	e, _ := setupScriptEngine(t, "crud", crudLua, []*schema.Table{itemsTable})

	m := callMap(t, e, "crud", "self-peer-id", map[string]any{"action": "schema"})
	if m["name"] != "items" {
		t.Fatalf("expected name 'items', got %v", m["name"])
	}
	if m["col_count"] != float64(2) {
		t.Fatalf("expected 2 columns, got %v", m["col_count"])
	}
	if m["insert_policy"] != "group" {
		t.Fatalf("expected insert policy 'group', got %v", m["insert_policy"])
	}
}

// ── Config key-value tests ──

var kvTable = &schema.Table{
	Name: "settings", SystemKey: true,
	Columns: []schema.Column{
		{Name: "key", Type: "text", Required: true},
		{Name: "value", Type: "text", Default: ""},
	},
}

func TestConfigKVDefaults(t *testing.T) {
	e, _ := setupScriptEngine(t, "config_kv", configKVLua, []*schema.Table{kvTable})

	m := callMap(t, e, "config_kv", "self-peer-id", map[string]any{"action": "read"})
	if m["theme"] != "light" {
		t.Fatalf("expected default 'light', got %v", m["theme"])
	}
	if m["lang"] != "en" {
		t.Fatalf("expected default 'en', got %v", m["lang"])
	}
}

func TestConfigKVSetAndRead(t *testing.T) {
	e, _ := setupScriptEngine(t, "config_kv", configKVLua, []*schema.Table{kvTable})

	callMap(t, e, "config_kv", "self-peer-id", map[string]any{"action": "set", "key": "theme", "value": "dark"})
	m := callMap(t, e, "config_kv", "self-peer-id", map[string]any{"action": "read"})
	if m["theme"] != "dark" {
		t.Fatalf("expected 'dark', got %v", m["theme"])
	}
}

func TestConfigKVReadAfterSet(t *testing.T) {
	e, _ := setupScriptEngine(t, "config_kv", configKVLua, []*schema.Table{kvTable})

	m := callMap(t, e, "config_kv", "self-peer-id", map[string]any{"action": "read_after_set"})
	if m["theme"] != "ocean" {
		t.Fatalf("expected 'ocean' after in-place set, got %v", m["theme"])
	}
}

func TestConfigKVPreloaded(t *testing.T) {
	e, db := setupScriptEngine(t, "config_kv", configKVLua, []*schema.Table{kvTable})

	db.OrmInsert("settings", "peer1", "", map[string]any{"key": "theme", "value": "neon"})

	m := callMap(t, e, "config_kv", "self-peer-id", map[string]any{"action": "read"})
	if m["theme"] != "neon" {
		t.Fatalf("expected DB value 'neon', got %v", m["theme"])
	}
}

// ── Config single-row tests ──

var rowTable = &schema.Table{
	Name: "app_settings", SystemKey: true,
	Columns: []schema.Column{
		{Name: "api_url", Type: "text", Default: ""},
		{Name: "timeout", Type: "integer", Default: 30},
		{Name: "debug", Type: "text", Default: "false"},
	},
}

func TestConfigRowDefaults(t *testing.T) {
	e, _ := setupScriptEngine(t, "config_row", configRowLua, []*schema.Table{rowTable})

	m := callMap(t, e, "config_row", "self-peer-id", map[string]any{"action": "read"})
	if m["api_url"] != "https://default.example.com" {
		t.Fatalf("expected default url, got %v", m["api_url"])
	}
	if m["timeout"] != float64(30) {
		t.Fatalf("expected timeout 30, got %v", m["timeout"])
	}
}

func TestConfigRowSet(t *testing.T) {
	e, _ := setupScriptEngine(t, "config_row", configRowLua, []*schema.Table{rowTable})

	callMap(t, e, "config_row", "self-peer-id", map[string]any{"action": "set", "key": "debug", "value": "true"})
	m := callMap(t, e, "config_row", "self-peer-id", map[string]any{"action": "read_all"})
	if m["debug"] != "true" {
		t.Fatalf("expected 'true', got %v", m["debug"])
	}
}

func TestConfigRowSave(t *testing.T) {
	e, db := setupScriptEngine(t, "config_row", configRowLua, []*schema.Table{rowTable})

	callMap(t, e, "config_row", "self-peer-id", map[string]any{
		"action": "save",
		"data":   map[string]any{"api_url": "https://new.example.com", "timeout": 120},
	})
	m := callMap(t, e, "config_row", "self-peer-id", map[string]any{"action": "read_all"})
	if m["api_url"] != "https://new.example.com" {
		t.Fatalf("expected new url, got %v", m["api_url"])
	}

	rows, _ := db.OrmList("app_settings", 0)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

// ── Game pattern tests ──

var gamesTable = &schema.Table{
	Name: "games", SystemKey: true,
	Columns: []schema.Column{
		{Name: "challenger", Type: "text"},
		{Name: "challenger_label", Type: "text", Default: ""},
		{Name: "mode", Type: "text", Default: "pvp"},
		{Name: "status", Type: "text", Default: "waiting"},
		{Name: "board", Type: "text", Default: "---------"},
		{Name: "turn", Type: "text", Default: "X"},
		{Name: "winner", Type: "text", Default: ""},
	},
	Access: &schema.Access{Read: "open", Insert: "open", Update: "owner", Delete: "owner"},
}

func TestGameNewAndState(t *testing.T) {
	e, _ := setupScriptEngine(t, "game", gameLua, []*schema.Table{gamesTable})

	m := callMap(t, e, "game", "self-peer-id", map[string]any{"action": "new", "mode": "pvp"})
	if m["status"] != "waiting" {
		t.Fatalf("expected 'waiting', got %v", m["status"])
	}
	gid := m["game_id"]

	m = callMap(t, e, "game", "self-peer-id", map[string]any{"action": "state", "game_id": gid})
	if m["board"] != "---------" {
		t.Fatalf("expected empty board, got %v", m["board"])
	}
}

func TestGameMoveAndCancel(t *testing.T) {
	e, _ := setupScriptEngine(t, "game", gameLua, []*schema.Table{gamesTable})

	m := callMap(t, e, "game", "self-peer-id", map[string]any{"action": "new"})
	gid := m["game_id"]

	callMap(t, e, "game", "self-peer-id", map[string]any{
		"action": "move", "game_id": gid, "board": "X--------", "turn": "O", "status": "playing",
	})

	m = callMap(t, e, "game", "self-peer-id", map[string]any{"action": "state", "game_id": gid})
	if m["board"] != "X--------" {
		t.Fatalf("expected 'X--------', got %v", m["board"])
	}
	if m["status"] != "playing" {
		t.Fatalf("expected 'playing', got %v", m["status"])
	}

	callMap(t, e, "game", "self-peer-id", map[string]any{"action": "cancel", "game_id": gid})
	m = callMap(t, e, "game", "self-peer-id", map[string]any{"action": "state", "game_id": gid})
	if m["status"] != "cancelled" {
		t.Fatalf("expected 'cancelled', got %v", m["status"])
	}
}

func TestGameLobbyWithAggregates(t *testing.T) {
	e, _ := setupScriptEngine(t, "game", gameLua, []*schema.Table{gamesTable})

	callMap(t, e, "game", "self-peer-id", map[string]any{"action": "new"})
	callMap(t, e, "game", "self-peer-id", map[string]any{"action": "new"})

	m := callMap(t, e, "game", "self-peer-id", map[string]any{"action": "lobby"})
	games := m["games"].([]any)
	if len(games) != 2 {
		t.Fatalf("expected 2 games, got %d", len(games))
	}
	if m["wins"] != float64(0) {
		t.Fatalf("expected 0 wins, got %v", m["wins"])
	}
}
