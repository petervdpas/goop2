package template

import (
	"encoding/json"
	"testing"

	ormschema "github.com/petervdpas/goop2/internal/orm/schema"
)

func TestAnalyzeSchemasGroupAccess(t *testing.T) {
	schema := ormschema.Table{
		Name:    "posts",
		Columns: []ormschema.Column{{Name: "title", Type: "text"}},
		Access:  &ormschema.Access{Read: "open", Insert: "group", Update: "owner", Delete: "owner"},
	}
	data, _ := json.Marshal(schema)
	info := AnalyzeSchemas(map[string][]byte{"schemas/posts.json": data}, nil)
	if !info.NeedsGroup {
		t.Fatal("should need group when insert access is 'group'")
	}
}

func TestAnalyzeSchemasUpdateDeleteAccess(t *testing.T) {
	schema := ormschema.Table{
		Name:    "items",
		Columns: []ormschema.Column{{Name: "id", Type: "integer"}},
		Access:  &ormschema.Access{Read: "open", Insert: "owner", Update: "group", Delete: "owner"},
	}
	data, _ := json.Marshal(schema)
	info := AnalyzeSchemas(map[string][]byte{"schemas/items.json": data}, nil)
	if !info.NeedsGroup {
		t.Fatal("should need group when update access is 'group'")
	}
}

func TestAnalyzeSchemasRolesMap(t *testing.T) {
	schema := ormschema.Table{
		Name:    "items",
		Columns: []ormschema.Column{{Name: "id", Type: "integer"}},
		Access:  &ormschema.Access{Read: "open", Insert: "owner", Update: "owner", Delete: "owner"},
		Roles:   map[string]ormschema.RoleAccess{"editor": {Read: true, Insert: true}},
	}
	data, _ := json.Marshal(schema)
	info := AnalyzeSchemas(map[string][]byte{"schemas/items.json": data}, nil)
	if !info.NeedsGroup {
		t.Fatal("should need group when schema has roles")
	}
	if len(info.Roles) != 1 || info.Roles[0] != "editor" {
		t.Fatalf("expected roles [editor], got %v", info.Roles)
	}
}

func TestAnalyzeSchemasNoGroup(t *testing.T) {
	schema := ormschema.Table{
		Name:    "items",
		Columns: []ormschema.Column{{Name: "id", Type: "integer"}},
		Access:  &ormschema.Access{Read: "open", Insert: "owner", Update: "owner", Delete: "owner"},
	}
	data, _ := json.Marshal(schema)
	info := AnalyzeSchemas(map[string][]byte{"schemas/items.json": data}, nil)
	if info.NeedsGroup {
		t.Fatal("should NOT need group when no access uses 'group' and no roles")
	}
}

func TestAnalyzeSchemasLegacyPolicy(t *testing.T) {
	info := AnalyzeSchemas(nil, map[string]string{"posts": "group"})
	if !info.NeedsGroup {
		t.Fatal("should need group when legacy table policy is 'group'")
	}
}

func TestAnalyzeSchemasMultipleRoles(t *testing.T) {
	schema := ormschema.Table{
		Name:    "posts",
		Columns: []ormschema.Column{{Name: "title", Type: "text"}},
		Access:  &ormschema.Access{Read: "open", Insert: "group", Update: "group", Delete: "group"},
		Roles: map[string]ormschema.RoleAccess{
			"coauthor": {Read: true, Insert: true, Update: true, Delete: true},
			"viewer":   {Read: true},
		},
	}
	data, _ := json.Marshal(schema)
	info := AnalyzeSchemas(map[string][]byte{"schemas/posts.json": data}, nil)
	if len(info.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d: %v", len(info.Roles), info.Roles)
	}
}
