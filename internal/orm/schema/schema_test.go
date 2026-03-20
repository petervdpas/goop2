package schema

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/petervdpas/goop2/internal/orm"

	_ "modernc.org/sqlite"
)

var tasksJSON = []byte(`{
  "name": "tasks",
  "columns": [
    {"name": "id",     "type": "integer", "key": true},
    {"name": "title",  "type": "text",    "required": true},
    {"name": "status", "type": "text",    "default": "pending"},
    {"name": "score",  "type": "real"}
  ]
}`)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1) // in-memory databases are per-connection
	t.Cleanup(func() { db.Close() })
	return db
}

func TestParseTable(t *testing.T) {
	tbl, err := ParseTable(tasksJSON)
	if err != nil {
		t.Fatal(err)
	}
	if tbl.Name != "tasks" {
		t.Fatalf("expected tasks, got %s", tbl.Name)
	}
	if len(tbl.Columns) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(tbl.Columns))
	}
	if tbl.Columns[0].Key != true {
		t.Fatal("id should be key")
	}
	if tbl.Columns[1].Required != true {
		t.Fatal("title should be required")
	}
}

func TestParseTable_InvalidJSON(t *testing.T) {
	_, err := ParseTable([]byte(`{broken`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseTable_NoName(t *testing.T) {
	_, err := ParseTable([]byte(`{"columns":[{"name":"id","type":"integer","key":true}]}`))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseTable_NoColumns(t *testing.T) {
	_, err := ParseTable([]byte(`{"name":"x","columns":[]}`))
	if err == nil {
		t.Fatal("expected error for empty columns")
	}
}

func TestParseTable_NoKey(t *testing.T) {
	_, err := ParseTable([]byte(`{"name":"x","columns":[{"name":"id","type":"integer"}]}`))
	if err == nil {
		t.Fatal("expected error for no key column")
	}
}

func TestParseTable_DuplicateColumn(t *testing.T) {
	_, err := ParseTable([]byte(`{"name":"x","columns":[{"name":"id","type":"integer","key":true},{"name":"id","type":"text"}]}`))
	if err == nil {
		t.Fatal("expected error for duplicate column")
	}
}

func TestParseTable_InvalidType(t *testing.T) {
	_, err := ParseTable([]byte(`{"name":"x","columns":[{"name":"id","type":"varchar","key":true}]}`))
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestTable_DDL(t *testing.T) {
	tbl, _ := ParseTable(tasksJSON)
	ddl := tbl.DDL()

	if !contains(ddl, "CREATE TABLE IF NOT EXISTS tasks") {
		t.Fatalf("DDL missing table name: %s", ddl)
	}
	if !contains(ddl, "PRIMARY KEY") {
		t.Fatalf("DDL missing PRIMARY KEY: %s", ddl)
	}
	if !contains(ddl, "NOT NULL") {
		t.Fatalf("DDL missing NOT NULL: %s", ddl)
	}
	if !contains(ddl, "DEFAULT 'pending'") {
		t.Fatalf("DDL missing DEFAULT: %s", ddl)
	}
}

func TestTable_DDL_CompositeKey(t *testing.T) {
	data := []byte(`{
		"name": "scores",
		"columns": [
			{"name": "user_id", "type": "integer", "key": true},
			{"name": "game_id", "type": "integer", "key": true},
			{"name": "score",   "type": "real"}
		]
	}`)
	tbl, _ := ParseTable(data)
	ddl := tbl.DDL()

	if !contains(ddl, "PRIMARY KEY (user_id, game_id)") {
		t.Fatalf("DDL should use composite key: %s", ddl)
	}
}

func TestTable_JSON_RoundTrip(t *testing.T) {
	tbl, _ := ParseTable(tasksJSON)
	out, err := tbl.JSON()
	if err != nil {
		t.Fatal(err)
	}
	tbl2, err := ParseTable(out)
	if err != nil {
		t.Fatal(err)
	}
	if tbl2.Name != tbl.Name || len(tbl2.Columns) != len(tbl.Columns) {
		t.Fatal("round-trip mismatch")
	}
}

func TestCodec_CRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tbl, _ := ParseTable(tasksJSON)
	codec := NewCodec(tbl)
	repo := orm.NewRepository(db, codec)

	if err := repo.CreateSchema(ctx); err != nil {
		t.Fatal(err)
	}

	err := repo.Insert(ctx, Row{"id": 1, "title": "Build ORM", "status": "done", "score": 9.5})
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByKey(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got["title"] != "Build ORM" {
		t.Fatalf("expected 'Build ORM', got %v", got["title"])
	}
	if got["score"] != 9.5 {
		t.Fatalf("expected 9.5, got %v", got["score"])
	}
}

func TestCodec_Update(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tbl, _ := ParseTable(tasksJSON)
	repo := orm.NewRepository(db, NewCodec(tbl))
	repo.CreateSchema(ctx)

	repo.Insert(ctx, Row{"id": 1, "title": "Draft", "status": "new"})
	err := repo.Update(ctx, Row{"id": 1, "title": "Final", "status": "done"})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := repo.GetByKey(ctx, 1)
	if got["title"] != "Final" {
		t.Fatalf("expected Final, got %v", got["title"])
	}
}

func TestCodec_Upsert(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tbl, _ := ParseTable(tasksJSON)
	repo := orm.NewRepository(db, NewCodec(tbl))
	repo.CreateSchema(ctx)

	repo.Upsert(ctx, Row{"id": 1, "title": "V1", "status": "new"})
	repo.Upsert(ctx, Row{"id": 1, "title": "V2", "status": "done"})

	got, _ := repo.GetByKey(ctx, 1)
	if got["title"] != "V2" {
		t.Fatalf("expected V2, got %v", got["title"])
	}
}

func TestCodec_ListAll(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tbl, _ := ParseTable(tasksJSON)
	repo := orm.NewRepository(db, NewCodec(tbl))
	repo.CreateSchema(ctx)

	repo.Insert(ctx, Row{"id": 1, "title": "A"})
	repo.Insert(ctx, Row{"id": 2, "title": "B"})

	all, err := repo.ListAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(all))
	}
}

func TestCodec_Delete(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tbl, _ := ParseTable(tasksJSON)
	repo := orm.NewRepository(db, NewCodec(tbl))
	repo.CreateSchema(ctx)

	repo.Insert(ctx, Row{"id": 1, "title": "X"})
	repo.DeleteByKey(ctx, 1)

	_, err := repo.GetByKey(ctx, 1)
	if err != orm.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCodec_NullValues(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tbl, _ := ParseTable(tasksJSON)
	repo := orm.NewRepository(db, NewCodec(tbl))
	repo.CreateSchema(ctx)

	repo.Insert(ctx, Row{"id": 1, "title": "Minimal"})

	got, _ := repo.GetByKey(ctx, 1)
	if got["score"] != nil {
		t.Fatalf("expected nil score, got %v", got["score"])
	}
}

func TestExportTable(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tbl, _ := ParseTable(tasksJSON)
	repo := orm.NewRepository(db, NewCodec(tbl))
	repo.CreateSchema(ctx)

	exported, err := ExportTable(ctx, db, "tasks")
	if err != nil {
		t.Fatal(err)
	}
	if exported.Name != "tasks" {
		t.Fatalf("expected tasks, got %s", exported.Name)
	}
	if len(exported.Columns) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(exported.Columns))
	}

	idCol := exported.Columns[0]
	if idCol.Name != "id" || idCol.Type != "integer" || !idCol.Key {
		t.Fatalf("id column mismatch: %+v", idCol)
	}
}

func TestExportTable_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := ExportTable(context.Background(), db, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent table")
	}
}

func TestExportAll(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, ddl := range []string{
		"CREATE TABLE alpha (id INTEGER PRIMARY KEY, val TEXT);",
		"CREATE TABLE beta (id INTEGER PRIMARY KEY, num REAL);",
	} {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}

	tables, err := ExportAll(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
}

func TestExport_JSON_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tbl, _ := ParseTable(tasksJSON)
	repo := orm.NewRepository(db, NewCodec(tbl))
	repo.CreateSchema(ctx)

	exported, _ := ExportTable(ctx, db, "tasks")
	exportedJSON, _ := exported.JSON()

	reimported, err := ParseTable(exportedJSON)
	if err != nil {
		t.Fatalf("re-import failed: %v\nJSON: %s", err, exportedJSON)
	}
	if reimported.Name != "tasks" || len(reimported.Columns) != 4 {
		t.Fatal("round-trip mismatch")
	}

	codec := NewCodec(reimported)
	repo2 := orm.NewRepository(db, codec)
	repo2.Insert(ctx, Row{"id": 99, "title": "RoundTrip"})
	got, _ := repo2.GetByKey(ctx, 99)
	if got["title"] != "RoundTrip" {
		t.Fatalf("expected RoundTrip, got %v", got["title"])
	}
}

func TestExport_DefaultPreserved(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tbl, _ := ParseTable(tasksJSON)
	orm.NewRepository(db, NewCodec(tbl)).CreateSchema(ctx)

	exported, _ := ExportTable(ctx, db, "tasks")
	for _, col := range exported.Columns {
		if col.Name == "status" && col.Default == nil {
			t.Fatal("status column should have default preserved")
		}
	}
}

func TestCodec_BlobColumn(t *testing.T) {
	data := []byte(`{
		"name": "files",
		"columns": [
			{"name": "id",   "type": "integer", "key": true},
			{"name": "data", "type": "blob"}
		]
	}`)
	db := openTestDB(t)
	ctx := context.Background()

	tbl, _ := ParseTable(data)
	repo := orm.NewRepository(db, NewCodec(tbl))
	repo.CreateSchema(ctx)

	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	repo.Insert(ctx, Row{"id": 1, "data": payload})

	got, _ := repo.GetByKey(ctx, 1)
	gotBytes, ok := got["data"].([]byte)
	if !ok {
		t.Fatalf("expected []byte, got %T", got["data"])
	}
	if len(gotBytes) != 4 || gotBytes[0] != 0xDE {
		t.Fatalf("blob mismatch: %x", gotBytes)
	}
}

func TestCodec_CompositeKey(t *testing.T) {
	data := []byte(`{
		"name": "scores",
		"columns": [
			{"name": "user_id", "type": "integer", "key": true},
			{"name": "game_id", "type": "integer", "key": true},
			{"name": "score",   "type": "real"}
		]
	}`)
	db := openTestDB(t)
	ctx := context.Background()

	tbl, _ := ParseTable(data)
	repo := orm.NewRepository(db, NewCodec(tbl))
	repo.CreateSchema(ctx)

	repo.Insert(ctx, Row{"user_id": 1, "game_id": 10, "score": 95.5})
	repo.Insert(ctx, Row{"user_id": 1, "game_id": 20, "score": 88.0})

	got, err := repo.GetByKey(ctx, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got["score"] != 95.5 {
		t.Fatalf("expected 95.5, got %v", got["score"])
	}

	n, _ := repo.Count(ctx)
	if n != 2 {
		t.Fatalf("expected 2 rows, got %d", n)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
