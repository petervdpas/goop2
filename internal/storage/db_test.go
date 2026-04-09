package storage

import (
	"testing"
)

func TestOpenAndClose(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if db.Path() == "" {
		t.Fatal("path should not be empty")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateTableAndList(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{
		{Name: "title", Type: "TEXT", NotNull: true},
		{Name: "count", Type: "INTEGER", Default: "0"},
	}
	if err := db.CreateTable("items", cols); err != nil {
		t.Fatal(err)
	}

	tables, err := db.ListTables()
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].Name != "items" {
		t.Fatalf("table name = %q, want 'items'", tables[0].Name)
	}
	if tables[0].InsertPolicy != "owner" {
		t.Fatalf("insert_policy = %q, want 'owner'", tables[0].InsertPolicy)
	}
}

func TestCreateTableDuplicate(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	if err := db.CreateTable("dup", cols); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateTable("dup", cols); err == nil {
		t.Fatal("expected error for duplicate table")
	}
}

func TestCreateTableInvalidName(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	if err := db.CreateTable("bad name!", cols); err == nil {
		t.Fatal("expected error for invalid table name")
	}
}

func TestCreateTableInvalidColumn(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "bad col!", Type: "TEXT"}}
	if err := db.CreateTable("t", cols); err == nil {
		t.Fatal("expected error for invalid column name")
	}
}

func TestDeleteTable(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("removeme", cols)

	if err := db.DeleteTable("removeme"); err != nil {
		t.Fatal(err)
	}
	tables, _ := db.ListTables()
	if len(tables) != 0 {
		t.Fatalf("expected 0 tables after delete, got %d", len(tables))
	}
}

func TestRenameTable(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("oldname", cols)

	if err := db.RenameTable("oldname", "newname"); err != nil {
		t.Fatal(err)
	}
	tables, _ := db.ListTables()
	if len(tables) != 1 || tables[0].Name != "newname" {
		t.Fatalf("expected renamed table 'newname', got %v", tables)
	}
}

func TestRenameTableInvalidName(t *testing.T) {
	db := testDB(t)

	if err := db.RenameTable("bad name!", "also bad!"); err == nil {
		t.Fatal("expected error for invalid name")
	}
}

func TestInsertAndSelect(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{
		{Name: "title", Type: "TEXT", NotNull: true},
		{Name: "score", Type: "INTEGER", Default: "0"},
	}
	db.CreateTable("posts", cols)

	id, err := db.Insert("posts", "peer1", "a@b.com", map[string]any{"title": "Hello", "score": 42})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	rows, err := db.Select("posts", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["title"] != "Hello" {
		t.Fatalf("title = %v, want 'Hello'", rows[0]["title"])
	}
	if rows[0]["_owner"] != "peer1" {
		t.Fatalf("_owner = %v, want 'peer1'", rows[0]["_owner"])
	}
}

func TestInsertInvalidTable(t *testing.T) {
	db := testDB(t)

	_, err := db.Insert("bad name!", "p", "", map[string]any{"x": 1})
	if err == nil {
		t.Fatal("expected error for invalid table name")
	}
}

func TestInsertInvalidColumn(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)

	_, err := db.Insert("t", "p", "", map[string]any{"bad col!": "x"})
	if err == nil {
		t.Fatal("expected error for invalid column name")
	}
}

func TestUpdateRow(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)
	id, _ := db.Insert("t", "p1", "", map[string]any{"val": "old"})

	if err := db.UpdateRow("t", id, map[string]any{"val": "new"}); err != nil {
		t.Fatal(err)
	}

	rows, _ := db.Select("t", nil, "_id = ?", id)
	if rows[0]["val"] != "new" {
		t.Fatalf("val = %v, want 'new'", rows[0]["val"])
	}
}

func TestUpdateRowInvalidTable(t *testing.T) {
	db := testDB(t)

	if err := db.UpdateRow("bad!", 1, map[string]any{"x": 1}); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateRowInvalidColumn(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)
	id, _ := db.Insert("t", "p", "", map[string]any{"val": "x"})

	if err := db.UpdateRow("t", id, map[string]any{"bad!": "y"}); err == nil {
		t.Fatal("expected error for invalid column name")
	}
}

func TestUpdateRowOwner(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)
	id, _ := db.Insert("t", "owner1", "", map[string]any{"val": "x"})

	if err := db.UpdateRowOwner("t", id, "owner1", map[string]any{"val": "y"}); err != nil {
		t.Fatal(err)
	}

	if err := db.UpdateRowOwner("t", id, "someone_else", map[string]any{"val": "z"}); err == nil {
		t.Fatal("expected error when non-owner updates")
	}
}

func TestUpdateRowOwnerInvalidTable(t *testing.T) {
	db := testDB(t)

	if err := db.UpdateRowOwner("bad!", 1, "p", map[string]any{"x": 1}); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateRowOwnerInvalidColumn(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)
	id, _ := db.Insert("t", "p", "", map[string]any{"val": "x"})

	if err := db.UpdateRowOwner("t", id, "p", map[string]any{"bad!": "y"}); err == nil {
		t.Fatal("expected error for invalid column name")
	}
}

func TestDeleteRow(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)
	id, _ := db.Insert("t", "p", "", map[string]any{"val": "x"})

	if err := db.DeleteRow("t", id); err != nil {
		t.Fatal(err)
	}
	rows, _ := db.Select("t", nil, "")
	if len(rows) != 0 {
		t.Fatal("expected 0 rows after delete")
	}
}

func TestDeleteRowInvalidTable(t *testing.T) {
	db := testDB(t)

	if err := db.DeleteRow("bad!", 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteRowOwner(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)
	id, _ := db.Insert("t", "owner1", "", map[string]any{"val": "x"})

	if err := db.DeleteRowOwner("t", id, "someone_else"); err == nil {
		t.Fatal("expected error when non-owner deletes")
	}
	if err := db.DeleteRowOwner("t", id, "owner1"); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteRowOwnerInvalidTable(t *testing.T) {
	db := testDB(t)

	if err := db.DeleteRowOwner("bad!", 1, "p"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSetGetMeta(t *testing.T) {
	db := testDB(t)

	if got := db.GetMeta("missing"); got != "" {
		t.Fatalf("expected empty for missing key, got %q", got)
	}

	db.SetMeta("key1", "value1")
	if got := db.GetMeta("key1"); got != "value1" {
		t.Fatalf("got %q, want 'value1'", got)
	}

	db.SetMeta("key1", "updated")
	if got := db.GetMeta("key1"); got != "updated" {
		t.Fatalf("got %q, want 'updated'", got)
	}
}

func TestDescribeTable(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{
		{Name: "title", Type: "TEXT", NotNull: true},
		{Name: "count", Type: "INTEGER"},
	}
	db.CreateTable("t", cols)

	info, err := db.DescribeTable("t")
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool)
	for _, c := range info {
		names[c.Name] = true
	}
	for _, want := range []string{"_id", "_owner", "title", "count"} {
		if !names[want] {
			t.Fatalf("missing column %q in describe output", want)
		}
	}
}

func TestDescribeTableInvalidName(t *testing.T) {
	db := testDB(t)

	_, err := db.DescribeTable("bad name!")
	if err == nil {
		t.Fatal("expected error for invalid table name")
	}
}

func TestAddColumn(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)

	if err := db.AddColumn("t", ColumnDef{Name: "extra", Type: "TEXT"}); err != nil {
		t.Fatal(err)
	}

	id, _ := db.Insert("t", "p", "", map[string]any{"val": "x", "extra": "bonus"})
	rows, _ := db.Select("t", nil, "_id = ?", id)
	if rows[0]["extra"] != "bonus" {
		t.Fatalf("extra = %v, want 'bonus'", rows[0]["extra"])
	}
}

func TestAddColumnInvalidTable(t *testing.T) {
	db := testDB(t)

	if err := db.AddColumn("bad!", ColumnDef{Name: "x", Type: "TEXT"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestAddColumnInvalidName(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)

	if err := db.AddColumn("t", ColumnDef{Name: "bad col!", Type: "TEXT"}); err == nil {
		t.Fatal("expected error for invalid column name")
	}
}

func TestDropColumn(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{
		{Name: "keep", Type: "TEXT"},
		{Name: "removeme", Type: "TEXT"},
	}
	db.CreateTable("t", cols)

	if err := db.DropColumn("t", "removeme"); err != nil {
		t.Fatal(err)
	}

	info, _ := db.DescribeTable("t")
	for _, c := range info {
		if c.Name == "removeme" {
			t.Fatal("column 'removeme' should have been dropped")
		}
	}
}

func TestDropColumnInvalidTable(t *testing.T) {
	db := testDB(t)

	if err := db.DropColumn("bad!", "x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDropColumnInvalidName(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)

	if err := db.DropColumn("t", "bad col!"); err == nil {
		t.Fatal("expected error for invalid column name")
	}
}

func TestSetGetTableInsertPolicy(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)

	policy, err := db.GetTableInsertPolicy("t")
	if err != nil {
		t.Fatal(err)
	}
	if policy != "owner" {
		t.Fatalf("default policy = %q, want 'owner'", policy)
	}

	db.SetTableInsertPolicy("t", "open")
	policy, _ = db.GetTableInsertPolicy("t")
	if policy != "open" {
		t.Fatalf("updated policy = %q, want 'open'", policy)
	}
}

func TestGetTableInsertPolicyMissing(t *testing.T) {
	db := testDB(t)

	policy, err := db.GetTableInsertPolicy("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing table")
	}
	if policy != "owner" {
		t.Fatalf("fallback policy = %q, want 'owner'", policy)
	}
}

func TestValidIdent(t *testing.T) {
	good := []string{"a", "table_name", "Col1", "_private"}
	for _, s := range good {
		if !validIdent(s) {
			t.Fatalf("%q should be valid", s)
		}
	}
	bad := []string{"", "bad name", "1starts_with_digit", "has-dash", "SELECT;DROP"}
	for _, s := range bad {
		if validIdent(s) {
			t.Fatalf("%q should be invalid", s)
		}
	}
}
