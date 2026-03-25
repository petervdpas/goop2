package gql

import (
	"strings"
	"testing"

	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/storage"
)

func testDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func createTestTable(t *testing.T, db *storage.DB, context bool) {
	t.Helper()
	tbl := &schema.Table{
		Name: "Person",
		Columns: []schema.Column{
			{Name: "Id", Type: "guid", Key: true, Auto: true},
			{Name: "Name", Type: "text", Required: true},
			{Name: "credits", Type: "integer"},
		},
		Context: context,
	}
	if err := db.CreateTableORM(tbl); err != nil {
		t.Fatalf("create table: %v", err)
	}
}

func TestEngine_NoContextTables(t *testing.T) {
	db := testDB(t)
	createTestTable(t, db, false)

	e := New(db, "self", func() string { return "test@test.com" })
	if err := e.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	result := e.Execute(`{ Person { Name } }`, nil, "")
	if len(result.Errors) == 0 {
		t.Fatal("expected error for no context tables")
	}
	if !strings.Contains(result.Errors[0].Message, "no tables in context") {
		t.Fatalf("unexpected error: %s", result.Errors[0].Message)
	}
}

func TestEngine_RebuildWithContext(t *testing.T) {
	db := testDB(t)
	createTestTable(t, db, true)

	e := New(db, "self", func() string { return "test@test.com" })
	if err := e.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	names := e.ContextTableNames()
	if len(names) != 1 || names[0] != "Person" {
		t.Fatalf("expected [Person], got %v", names)
	}
}

func TestEngine_QueryEmpty(t *testing.T) {
	db := testDB(t)
	createTestTable(t, db, true)

	e := New(db, "self", func() string { return "test@test.com" })
	if err := e.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	result := e.Execute(`{ Person { Name credits } }`, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result.Data)
	}
	persons, ok := data["Person"].([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", data["Person"])
	}
	if len(persons) != 0 {
		t.Fatalf("expected empty list, got %d", len(persons))
	}
}

func TestEngine_InsertAndQuery(t *testing.T) {
	db := testDB(t)
	createTestTable(t, db, true)

	e := New(db, "self", func() string { return "test@test.com" })
	if err := e.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	insertQ := `mutation { insert_Person(object: {Name: "Alice", credits: 100}) { _id Name credits } }`
	result := e.Execute(insertQ, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("insert errors: %v", result.Errors)
	}

	data := result.Data.(map[string]any)
	person := data["insert_Person"].(map[string]any)
	if person["Name"] != "Alice" {
		t.Fatalf("expected Alice, got %v", person["Name"])
	}

	queryQ := `{ Person { Name credits } }`
	result = e.Execute(queryQ, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("query errors: %v", result.Errors)
	}
	data = result.Data.(map[string]any)
	persons := data["Person"].([]any)
	if len(persons) != 1 {
		t.Fatalf("expected 1 person, got %d", len(persons))
	}
}

func TestEngine_QueryWithFilter(t *testing.T) {
	db := testDB(t)
	createTestTable(t, db, true)

	e := New(db, "self", func() string { return "test@test.com" })
	if err := e.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	e.Execute(`mutation { insert_Person(object: {Name: "Alice", credits: 50}) { _id } }`, nil, "")
	e.Execute(`mutation { insert_Person(object: {Name: "Bob", credits: 200}) { _id } }`, nil, "")

	result := e.Execute(`{ Person(where: {credits_gt: 100}) { Name credits } }`, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("query errors: %v", result.Errors)
	}
	data := result.Data.(map[string]any)
	persons := data["Person"].([]any)
	if len(persons) != 1 {
		t.Fatalf("expected 1 person (Bob), got %d", len(persons))
	}
	bob := persons[0].(map[string]any)
	if bob["Name"] != "Bob" {
		t.Fatalf("expected Bob, got %v", bob["Name"])
	}
}

func TestEngine_QueryByPK(t *testing.T) {
	db := testDB(t)
	createTestTable(t, db, true)

	e := New(db, "self", func() string { return "test@test.com" })
	if err := e.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	e.Execute(`mutation { insert_Person(object: {Name: "Alice", credits: 50}) { _id } }`, nil, "")

	result := e.Execute(`{ Person_by_pk(_id: 1) { Name credits } }`, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("query errors: %v", result.Errors)
	}
	data := result.Data.(map[string]any)
	person := data["Person_by_pk"].(map[string]any)
	if person["Name"] != "Alice" {
		t.Fatalf("expected Alice, got %v", person["Name"])
	}
}

func TestEngine_Update(t *testing.T) {
	db := testDB(t)
	createTestTable(t, db, true)

	e := New(db, "self", func() string { return "test@test.com" })
	if err := e.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	e.Execute(`mutation { insert_Person(object: {Name: "Alice", credits: 50}) { _id } }`, nil, "")

	updateQ := `mutation { update_Person_by_pk(_id: 1, _set: {Name: "Alice", credits: 999}) { Name credits } }`
	result := e.Execute(updateQ, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("update errors: %v", result.Errors)
	}
	data := result.Data.(map[string]any)
	person := data["update_Person_by_pk"].(map[string]any)
	credits := person["credits"]
	var creditsVal int64
	switch v := credits.(type) {
	case int64:
		creditsVal = v
	case int:
		creditsVal = int64(v)
	case float64:
		creditsVal = int64(v)
	}
	if creditsVal != 999 {
		t.Fatalf("expected 999, got %v (%T)", credits, credits)
	}
}

func TestEngine_Delete(t *testing.T) {
	db := testDB(t)
	createTestTable(t, db, true)

	e := New(db, "self", func() string { return "test@test.com" })
	if err := e.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	e.Execute(`mutation { insert_Person(object: {Name: "Alice", credits: 50}) { _id } }`, nil, "")

	deleteQ := `mutation { delete_Person_by_pk(_id: 1) { affected_rows } }`
	result := e.Execute(deleteQ, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("delete errors: %v", result.Errors)
	}

	queryResult := e.Execute(`{ Person { Name } }`, nil, "")
	data := queryResult.Data.(map[string]any)
	persons := data["Person"].([]any)
	if len(persons) != 0 {
		t.Fatalf("expected 0 persons after delete, got %d", len(persons))
	}
}

func TestEngine_Pagination(t *testing.T) {
	db := testDB(t)
	createTestTable(t, db, true)

	e := New(db, "self", func() string { return "test@test.com" })
	if err := e.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	for i := 0; i < 5; i++ {
		e.Execute(`mutation { insert_Person(object: {Name: "P", credits: 0}) { _id } }`, nil, "")
	}

	result := e.Execute(`{ Person(limit: 2) { _id } }`, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("errors: %v", result.Errors)
	}
	data := result.Data.(map[string]any)
	persons := data["Person"].([]any)
	if len(persons) != 2 {
		t.Fatalf("expected 2, got %d", len(persons))
	}

	result = e.Execute(`{ Person(limit: 2, offset: 3) { _id } }`, nil, "")
	data = result.Data.(map[string]any)
	persons = data["Person"].([]any)
	if len(persons) != 2 {
		t.Fatalf("expected 2, got %d", len(persons))
	}
}

func TestEngine_ContextTableToggle(t *testing.T) {
	db := testDB(t)
	createTestTable(t, db, false)

	e := New(db, "self", func() string { return "test@test.com" })
	_ = e.Rebuild()
	if len(e.ContextTableNames()) != 0 {
		t.Fatal("expected no context tables")
	}

	db.UpdateSchemaContext("Person", true)
	_ = e.Rebuild()
	if len(e.ContextTableNames()) != 1 {
		t.Fatal("expected 1 context table after toggle")
	}

	db.UpdateSchemaContext("Person", false)
	_ = e.Rebuild()
	if len(e.ContextTableNames()) != 0 {
		t.Fatal("expected 0 context tables after untoggle")
	}
}

func TestEngine_MultipleTables(t *testing.T) {
	db := testDB(t)
	createTestTable(t, db, true)

	orderTbl := &schema.Table{
		Name: "Invoice",
		Columns: []schema.Column{
			{Name: "Id", Type: "guid", Key: true, Auto: true},
			{Name: "product", Type: "text", Required: true},
			{Name: "amount", Type: "real"},
		},
		Context: true,
	}
	if err := db.CreateTableORM(orderTbl); err != nil {
		t.Fatalf("create Invoice: %v", err)
	}

	e := New(db, "self", func() string { return "test@test.com" })
	if err := e.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	names := e.ContextTableNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 tables, got %d: %v", len(names), names)
	}

	e.Execute(`mutation { insert_Person(object: {Name: "Alice", credits: 10}) { _id } }`, nil, "")
	e.Execute(`mutation { insert_Invoice(object: {product: "Widget", amount: 9.99}) { _id } }`, nil, "")

	result := e.Execute(`{ Person { Name } Invoice { product amount } }`, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("errors: %v", result.Errors)
	}
	data := result.Data.(map[string]any)
	persons := data["Person"].([]any)
	orders := data["Invoice"].([]any)
	if len(persons) != 1 || len(orders) != 1 {
		t.Fatalf("expected 1 person and 1 order, got %d and %d", len(persons), len(orders))
	}
}

func TestEngine_FederatedRebuild(t *testing.T) {
	db := testDB(t)
	createTestTable(t, db, true)

	e := New(db, "self", func() string { return "test@test.com" })

	remotePerson := schema.Table{
		Name: "Person",
		Columns: []schema.Column{
			{Name: "Id", Type: "guid", Key: true},
			{Name: "Name", Type: "text"},
			{Name: "phone", Type: "text"},
		},
	}

	peers := []PeerSource{{PeerID: "peer-b", Tables: []schema.Table{remotePerson}}}

	err := e.RebuildFederated(e.ContextTables(), peers, nil)
	if err != nil {
		t.Fatalf("federated rebuild: %v", err)
	}

	result := e.Execute(`{ Person { Name } }`, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("errors: %v", result.Errors)
	}

	result = e.Execute(`{ Person { phone } }`, nil, "")
	if len(result.Errors) == 0 {
		t.Fatal("expected error querying phone (not in intersection)")
	}
}

func TestEngine_FederatedUniqueTable(t *testing.T) {
	db := testDB(t)

	e := New(db, "self", func() string { return "test@test.com" })

	remoteOnly := schema.Table{
		Name: "Inventory",
		Columns: []schema.Column{
			{Name: "Id", Type: "guid", Key: true},
			{Name: "product", Type: "text"},
			{Name: "stock", Type: "integer"},
		},
	}

	peers := []PeerSource{{PeerID: "peer-b", Tables: []schema.Table{remoteOnly}}}

	err := e.RebuildFederated(nil, peers, nil)
	if err != nil {
		t.Fatalf("federated rebuild: %v", err)
	}

	result := e.Execute(`{ Inventory { product stock } }`, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("errors: %v", result.Errors)
	}
}

func TestEngine_SanitizeName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Person", "Person"},
		{"my-table", "my_table"},
		{"123abc", "_23abc"},
		{"a b c", "a_b_c"},
		{"_valid_name", "_valid_name"},
	}
	for _, tt := range tests {
		got := sanitizeName(tt.in)
		if got != tt.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
