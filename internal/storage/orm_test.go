package storage

import (
	"context"
	"testing"

	"github.com/petervdpas/goop2/internal/orm/schema"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateTableORMSystemKey(t *testing.T) {
	db := testDB(t)

	tbl := &schema.Table{
		Name:      "posts",
		SystemKey: true,
		Columns: []schema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "body", Type: "text"},
		},
		Access: &schema.Access{Read: "open", Insert: "group", Update: "owner", Delete: "owner"},
	}

	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	if !db.IsORM("posts") {
		t.Fatal("posts should be ORM-managed")
	}

	stored, err := db.GetSchema("posts")
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil {
		t.Fatal("stored schema should not be nil")
	}
	if stored.Access == nil {
		t.Fatal("stored schema should have Access")
	}
	if stored.Access.Insert != "group" {
		t.Fatalf("stored access.insert = %q, want 'group'", stored.Access.Insert)
	}
	if !stored.SystemKey {
		t.Fatal("stored schema should preserve SystemKey")
	}
}

func TestGetAccessORM(t *testing.T) {
	db := testDB(t)

	tbl := &schema.Table{
		Name:      "data",
		SystemKey: true,
		Columns:   []schema.Column{{Name: "val", Type: "text"}},
		Access:    &schema.Access{Read: "group", Insert: "email", Update: "owner", Delete: "owner"},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	access := db.GetAccess("data")
	if access.Read != "group" {
		t.Fatalf("access.read = %q, want 'group'", access.Read)
	}
	if access.Insert != "email" {
		t.Fatalf("access.insert = %q, want 'email'", access.Insert)
	}
}

func TestGetAccessFallsBackToInsertPolicy(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "title", Type: "TEXT", NotNull: true}}
	if err := db.CreateTable("legacy", cols); err != nil {
		t.Fatal(err)
	}
	db.SetTableInsertPolicy("legacy", "open")

	access := db.GetAccess("legacy")
	if access.Insert != "open" {
		t.Fatalf("access.insert = %q, want 'open' (from insert_policy fallback)", access.Insert)
	}
	if access.Read != "open" {
		t.Fatalf("access.read = %q, want 'open'", access.Read)
	}
}

func TestUpdateSchemaAccess(t *testing.T) {
	db := testDB(t)

	tbl := &schema.Table{
		Name:      "data",
		SystemKey: true,
		Columns:   []schema.Column{{Name: "val", Type: "text"}},
		Access:    &schema.Access{Read: "open", Insert: "owner", Update: "owner", Delete: "owner"},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	newAccess := schema.Access{Read: "group", Insert: "group", Update: "owner", Delete: "owner"}
	db.UpdateSchemaAccess("data", &newAccess)

	access := db.GetAccess("data")
	if access.Read != "group" {
		t.Fatalf("after update: access.read = %q, want 'group'", access.Read)
	}
	if access.Insert != "group" {
		t.Fatalf("after update: access.insert = %q, want 'group'", access.Insert)
	}
}

func TestDeleteTableCleansORM(t *testing.T) {
	db := testDB(t)

	tbl := &schema.Table{
		Name:      "temp",
		SystemKey: true,
		Columns:   []schema.Column{{Name: "val", Type: "text"}},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	if !db.IsORM("temp") {
		t.Fatal("temp should be ORM before delete")
	}

	if err := db.DeleteTable("temp"); err != nil {
		t.Fatal(err)
	}

	if db.IsORM("temp") {
		t.Fatal("temp should NOT be ORM after delete")
	}
}

func TestOrmInsertAndList(t *testing.T) {
	db := testDB(t)

	tbl := &schema.Table{
		Name:      "items",
		SystemKey: true,
		Columns: []schema.Column{
			{Name: "name", Type: "text", Required: true},
			{Name: "count", Type: "integer", Default: 0},
		},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	id, err := db.OrmInsert("items", "peer1", "a@b.com", map[string]any{"name": "apple", "count": 5})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	rows, err := db.OrmList("items", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	if row["name"] != "apple" {
		t.Fatalf("name = %v, want 'apple'", row["name"])
	}
	if row["_owner"] != "peer1" {
		t.Fatalf("_owner = %v, want 'peer1'", row["_owner"])
	}
}

func TestOrmValidateRejectsInvalidType(t *testing.T) {
	db := testDB(t)

	tbl := &schema.Table{
		Name:      "typed",
		SystemKey: true,
		Columns: []schema.Column{
			{Name: "count", Type: "integer", Required: true},
		},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	_, err := db.OrmInsert("typed", "peer1", "", map[string]any{"count": "not_a_number"})
	if err == nil {
		t.Fatal("expected validation error for string in integer column")
	}
}

func TestExportSchemaORM(t *testing.T) {
	db := testDB(t)

	tbl := &schema.Table{
		Name:      "data",
		SystemKey: true,
		Columns:   []schema.Column{{Name: "val", Type: "text"}},
		Access:    &schema.Access{Read: "open", Insert: "owner", Update: "owner", Delete: "owner"},
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	exported, err := db.ExportSchema(context.Background(), "data")
	if err != nil {
		t.Fatal(err)
	}
	if exported == nil {
		t.Fatal("export should return schema")
	}
	if exported.Access == nil || exported.Access.Insert != "owner" {
		t.Fatal("exported schema should preserve Access")
	}
}
