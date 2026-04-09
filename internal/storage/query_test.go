package storage

import (
	"testing"
)

func TestSelectPaged(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)
	for _, v := range []string{"a", "b", "c", "d", "e"} {
		db.Insert("t", "p", "", map[string]any{"val": v})
	}

	rows, err := db.SelectPaged(SelectOpts{
		Table: "t",
		Order: "_id ASC",
		Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["val"] != "a" {
		t.Fatalf("first row val = %v, want 'a'", rows[0]["val"])
	}

	rows, err = db.SelectPaged(SelectOpts{
		Table:  "t",
		Order:  "_id ASC",
		Limit:  2,
		Offset: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["val"] != "c" {
		t.Fatalf("offset row val = %v, want 'c'", rows[0]["val"])
	}
}

func TestSelectWithColumns(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{
		{Name: "title", Type: "TEXT"},
		{Name: "body", Type: "TEXT"},
	}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"title": "hi", "body": "world"})

	rows, err := db.SelectPaged(SelectOpts{
		Table:   "t",
		Columns: []string{"title"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatal("expected 1 row")
	}
	if _, ok := rows[0]["body"]; ok {
		t.Fatal("body should not be in result when only title selected")
	}
}

func TestSelectWithWhere(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "INTEGER"}}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"val": 10})
	db.Insert("t", "p", "", map[string]any{"val": 20})
	db.Insert("t", "p", "", map[string]any{"val": 30})

	rows, err := db.Select("t", nil, "val > ?", 15)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows where val > 15, got %d", len(rows))
	}
}

func TestAggregate(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "score", Type: "INTEGER"}}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"score": 10})
	db.Insert("t", "p", "", map[string]any{"score": 20})
	db.Insert("t", "p", "", map[string]any{"score": 30})

	rows, err := db.Aggregate("t", "SUM(score) as total, COUNT(*) as cnt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	total, _ := rows[0]["total"].(int64)
	if total != 60 {
		t.Fatalf("total = %v, want 60", rows[0]["total"])
	}
}

func TestAggregateWithWhere(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "score", Type: "INTEGER"}}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"score": 10})
	db.Insert("t", "p", "", map[string]any{"score": 20})

	rows, err := db.Aggregate("t", "SUM(score) as total", "score > ?", 15)
	if err != nil {
		t.Fatal(err)
	}
	total, _ := rows[0]["total"].(int64)
	if total != 20 {
		t.Fatalf("total = %v, want 20", rows[0]["total"])
	}
}

func TestAggregateInvalidTable(t *testing.T) {
	db := testDB(t)

	_, err := db.Aggregate("bad!", "COUNT(*)", "")
	if err == nil {
		t.Fatal("expected error for invalid table name")
	}
}

func TestAggregateGroupBy(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{
		{Name: "category", Type: "TEXT"},
		{Name: "score", Type: "INTEGER"},
	}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"category": "a", "score": 10})
	db.Insert("t", "p", "", map[string]any{"category": "a", "score": 20})
	db.Insert("t", "p", "", map[string]any{"category": "b", "score": 5})

	rows, err := db.AggregateGroupBy("t", "category, SUM(score) as total", "category", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(rows))
	}
}

func TestAggregateGroupByInvalidTable(t *testing.T) {
	db := testDB(t)

	_, err := db.AggregateGroupBy("bad!", "COUNT(*)", "", "")
	if err == nil {
		t.Fatal("expected error for invalid table name")
	}
}

func TestAggregateGroupByWithWhere(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{
		{Name: "category", Type: "TEXT"},
		{Name: "score", Type: "INTEGER"},
	}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"category": "a", "score": 10})
	db.Insert("t", "p", "", map[string]any{"category": "a", "score": 20})
	db.Insert("t", "p", "", map[string]any{"category": "b", "score": 5})

	rows, err := db.AggregateGroupBy("t", "category, SUM(score) as total", "category", "score > ?", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 group (only 'a' has scores > 8), got %d", len(rows))
	}
}

func TestUpdateWhere(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{
		{Name: "status", Type: "TEXT"},
		{Name: "score", Type: "INTEGER"},
	}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"status": "pending", "score": 10})
	db.Insert("t", "p", "", map[string]any{"status": "pending", "score": 20})
	db.Insert("t", "p", "", map[string]any{"status": "done", "score": 30})

	n, err := db.UpdateWhere("t", map[string]any{"status": "done"}, "status = ?", "pending")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("affected = %d, want 2", n)
	}

	rows, _ := db.Select("t", nil, "status = ?", "done")
	if len(rows) != 3 {
		t.Fatalf("expected 3 done rows, got %d", len(rows))
	}
}

func TestUpdateWhereInvalidTable(t *testing.T) {
	db := testDB(t)

	_, err := db.UpdateWhere("bad!", map[string]any{"x": 1}, "1=1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateWhereInvalidColumn(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)

	_, err := db.UpdateWhere("t", map[string]any{"bad!": "x"}, "1=1")
	if err == nil {
		t.Fatal("expected error for invalid column name")
	}
}

func TestUpdateWhereSQLExpr(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "counter", Type: "INTEGER", Default: "0"}}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"counter": 5})

	n, err := db.UpdateWhere("t", map[string]any{"counter": SQLExpr{Expr: "counter + 1"}}, "1=1")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("affected = %d, want 1", n)
	}

	rows, _ := db.Select("t", nil, "")
	counter, _ := rows[0]["counter"].(int64)
	if counter != 6 {
		t.Fatalf("counter = %v, want 6", rows[0]["counter"])
	}
}

func TestDeleteWhere(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "status", Type: "TEXT"}}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"status": "keep"})
	db.Insert("t", "p", "", map[string]any{"status": "remove"})
	db.Insert("t", "p", "", map[string]any{"status": "remove"})

	n, err := db.DeleteWhere("t", "status = ?", "remove")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("affected = %d, want 2", n)
	}

	rows, _ := db.Select("t", nil, "")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestDeleteWhereInvalidTable(t *testing.T) {
	db := testDB(t)

	_, err := db.DeleteWhere("bad!", "1=1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpsertInsertAndUpdate(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{
		{Name: "key", Type: "TEXT", NotNull: true},
		{Name: "value", Type: "TEXT"},
	}
	db.CreateTable("config", cols)

	id1, err := db.Upsert("config", "key", "p", "", map[string]any{"key": "color", "value": "red"})
	if err != nil {
		t.Fatal(err)
	}
	if id1 == 0 {
		t.Fatal("expected non-zero id")
	}

	id2, err := db.Upsert("config", "key", "p", "", map[string]any{"key": "color", "value": "blue"})
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id1 {
		t.Fatalf("upsert should return same id, got %d vs %d", id1, id2)
	}

	rows, _ := db.Select("config", nil, "")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after upsert, got %d", len(rows))
	}
	if rows[0]["value"] != "blue" {
		t.Fatalf("value = %v, want 'blue'", rows[0]["value"])
	}
}

func TestUpsertInvalidIdent(t *testing.T) {
	db := testDB(t)

	_, err := db.Upsert("bad!", "key", "p", "", map[string]any{"key": "x"})
	if err == nil {
		t.Fatal("expected error for invalid table name")
	}
}

func TestUpsertMissingKeyColumn(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "key", Type: "TEXT"}, {Name: "value", Type: "TEXT"}}
	db.CreateTable("t", cols)

	_, err := db.Upsert("t", "key", "p", "", map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected error for missing key column in data")
	}
}

func TestDistinct(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "color", Type: "TEXT"}}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"color": "red"})
	db.Insert("t", "p", "", map[string]any{"color": "blue"})
	db.Insert("t", "p", "", map[string]any{"color": "red"})

	vals, err := db.Distinct("t", "color", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 2 {
		t.Fatalf("expected 2 distinct values, got %d", len(vals))
	}
}

func TestDistinctWithWhere(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{
		{Name: "color", Type: "TEXT"},
		{Name: "active", Type: "INTEGER"},
	}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"color": "red", "active": 1})
	db.Insert("t", "p", "", map[string]any{"color": "blue", "active": 0})
	db.Insert("t", "p", "", map[string]any{"color": "red", "active": 1})

	vals, err := db.Distinct("t", "color", "active = ?", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 distinct value, got %d", len(vals))
	}
}

func TestDistinctInvalidIdent(t *testing.T) {
	db := testDB(t)

	_, err := db.Distinct("bad!", "col", "")
	if err == nil {
		t.Fatal("expected error")
	}
}
