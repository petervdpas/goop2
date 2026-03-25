package schema

import "testing"

func TestIntersectColumns_Empty(t *testing.T) {
	if cols := IntersectColumns(nil); cols != nil {
		t.Fatalf("expected nil, got %v", cols)
	}
	if cols := IntersectColumns([]Table{}); cols != nil {
		t.Fatalf("expected nil, got %v", cols)
	}
}

func TestIntersectColumns_Single(t *testing.T) {
	tbl := Table{Name: "T", Columns: []Column{
		{Name: "Id", Type: "guid", Key: true},
		{Name: "Name", Type: "text"},
	}}
	cols := IntersectColumns([]Table{tbl})
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}
	if cols[0].Name != "Id" || cols[1].Name != "Name" {
		t.Fatalf("unexpected columns: %v", cols)
	}
}

func TestIntersectColumns_IdenticalTables(t *testing.T) {
	tbl := Table{Name: "Person", Columns: []Column{
		{Name: "Id", Type: "guid", Key: true},
		{Name: "Name", Type: "text", Required: true},
		{Name: "credits", Type: "integer"},
	}}
	cols := IntersectColumns([]Table{tbl, tbl})
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
}

func TestIntersectColumns_PartialOverlap(t *testing.T) {
	a := Table{Name: "Person", Columns: []Column{
		{Name: "Id", Type: "guid", Key: true},
		{Name: "Name", Type: "text"},
		{Name: "email", Type: "text"},
	}}
	b := Table{Name: "Person", Columns: []Column{
		{Name: "Id", Type: "guid", Key: true},
		{Name: "Name", Type: "text"},
		{Name: "phone", Type: "text"},
	}}
	cols := IntersectColumns([]Table{a, b})
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns (Id, Name), got %d", len(cols))
	}
	if cols[0].Name != "Id" || cols[1].Name != "Name" {
		t.Fatalf("unexpected columns: %v", cols)
	}
}

func TestIntersectColumns_TypeMismatch(t *testing.T) {
	a := Table{Name: "T", Columns: []Column{
		{Name: "Id", Type: "guid", Key: true},
		{Name: "score", Type: "integer"},
	}}
	b := Table{Name: "T", Columns: []Column{
		{Name: "Id", Type: "guid", Key: true},
		{Name: "score", Type: "text"},
	}}
	cols := IntersectColumns([]Table{a, b})
	if len(cols) != 1 {
		t.Fatalf("expected 1 column (Id only, score type mismatch), got %d", len(cols))
	}
	if cols[0].Name != "Id" {
		t.Fatalf("expected Id, got %s", cols[0].Name)
	}
}

func TestIntersectColumns_NoOverlap(t *testing.T) {
	a := Table{Name: "T", Columns: []Column{{Name: "a", Type: "text"}}}
	b := Table{Name: "T", Columns: []Column{{Name: "b", Type: "text"}}}
	cols := IntersectColumns([]Table{a, b})
	if len(cols) != 0 {
		t.Fatalf("expected 0 columns, got %d", len(cols))
	}
}

func TestIntersectColumns_ThreeTables(t *testing.T) {
	a := Table{Name: "T", Columns: []Column{
		{Name: "Id", Type: "guid", Key: true},
		{Name: "Name", Type: "text"},
		{Name: "a_only", Type: "text"},
	}}
	b := Table{Name: "T", Columns: []Column{
		{Name: "Id", Type: "guid", Key: true},
		{Name: "Name", Type: "text"},
		{Name: "b_only", Type: "integer"},
	}}
	c := Table{Name: "T", Columns: []Column{
		{Name: "Id", Type: "guid", Key: true},
		{Name: "Name", Type: "text"},
		{Name: "c_only", Type: "real"},
	}}
	cols := IntersectColumns([]Table{a, b, c})
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns (Id, Name), got %d", len(cols))
	}
}

func TestIntersectColumns_PreservesFlags(t *testing.T) {
	a := Table{Name: "T", Columns: []Column{
		{Name: "Id", Type: "guid", Key: true, Auto: true},
		{Name: "Name", Type: "text", Required: true},
	}}
	b := Table{Name: "T", Columns: []Column{
		{Name: "Id", Type: "guid"},
		{Name: "Name", Type: "text"},
	}}
	cols := IntersectColumns([]Table{a, b})
	if len(cols) != 2 {
		t.Fatalf("expected 2, got %d", len(cols))
	}
	if !cols[0].Key || !cols[0].Auto {
		t.Fatal("expected Key and Auto flags preserved from first table")
	}
	if !cols[1].Required {
		t.Fatal("expected Required flag preserved from first table")
	}
}

func TestMergeTable_Basic(t *testing.T) {
	a := Table{Name: "Person", Columns: []Column{
		{Name: "Id", Type: "guid", Key: true},
		{Name: "Name", Type: "text"},
		{Name: "email", Type: "text"},
	}}
	b := Table{Name: "Person", Columns: []Column{
		{Name: "Id", Type: "guid", Key: true},
		{Name: "Name", Type: "text"},
		{Name: "phone", Type: "text"},
	}}
	merged := MergeTable("Person", []Table{a, b})
	if merged == nil {
		t.Fatal("expected merged table, got nil")
	}
	if merged.Name != "Person" {
		t.Fatalf("expected name Person, got %s", merged.Name)
	}
	if len(merged.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(merged.Columns))
	}
	if !merged.Context {
		t.Fatal("expected Context=true on merged table")
	}
}

func TestMergeTable_NoKeyColumn(t *testing.T) {
	a := Table{Name: "T", Columns: []Column{
		{Name: "Id", Type: "guid", Key: true},
		{Name: "Name", Type: "text"},
	}}
	b := Table{Name: "T", Columns: []Column{
		{Name: "Name", Type: "text"},
	}}
	merged := MergeTable("T", []Table{a, b})
	if merged != nil {
		t.Fatal("expected nil (no key in intersection), got table")
	}
}

func TestMergeTable_NoCommonColumns(t *testing.T) {
	a := Table{Name: "T", Columns: []Column{{Name: "a", Type: "text", Key: true}}}
	b := Table{Name: "T", Columns: []Column{{Name: "b", Type: "text", Key: true}}}
	merged := MergeTable("T", []Table{a, b})
	if merged != nil {
		t.Fatal("expected nil, got table")
	}
}
