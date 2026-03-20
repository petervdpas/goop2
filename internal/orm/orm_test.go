package orm

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

type TestEntity struct {
	ID   int
	Name string
	Age  int
}

type testCodec struct{}

func (c testCodec) TableName() string     { return "test_entities" }
func (c testCodec) Columns() []string     { return []string{"id", "name", "age"} }
func (c testCodec) KeyColumns() []string  { return []string{"id"} }
func (c testCodec) New() TestEntity       { return TestEntity{} }
func (c testCodec) Values(e TestEntity) []any   { return []any{e.ID, e.Name, e.Age} }
func (c testCodec) KeyValues(e TestEntity) []any { return []any{e.ID} }
func (c testCodec) ScanTargets(e *TestEntity) []any {
	return []any{&e.ID, &e.Name, &e.Age}
}
func (c testCodec) Schema() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS test_entities (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			age INTEGER NOT NULL
		);`,
	}
}
func (c testCodec) UpsertColumns() []string {
	return []string{"name", "age"}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestRepo(t *testing.T) *Repository[TestEntity] {
	t.Helper()
	db := openTestDB(t)
	repo := NewRepository[TestEntity](db, testCodec{})
	if err := repo.CreateSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestCreateSchema(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository[TestEntity](db, testCodec{})
	if err := repo.CreateSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.TableName() != "test_entities" {
		t.Fatalf("expected table name test_entities, got %s", repo.TableName())
	}
}

func TestInsert_And_GetByKey(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	err := repo.Insert(ctx, TestEntity{ID: 1, Name: "Alice", Age: 30})
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByKey(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Alice" || got.Age != 30 {
		t.Fatalf("got %+v, want Alice/30", got)
	}
}

func TestGetByKey_NotFound(t *testing.T) {
	repo := newTestRepo(t)
	_, err := repo.GetByKey(context.Background(), 999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetByKey_InvalidKeyCount(t *testing.T) {
	repo := newTestRepo(t)
	_, err := repo.GetByKey(context.Background(), 1, 2)
	if err != ErrInvalidKeyValueCount {
		t.Fatalf("expected ErrInvalidKeyValueCount, got %v", err)
	}
}

func TestUpdate(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	repo.Insert(ctx, TestEntity{ID: 1, Name: "Alice", Age: 30})
	err := repo.Update(ctx, TestEntity{ID: 1, Name: "Bob", Age: 25})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := repo.GetByKey(ctx, 1)
	if got.Name != "Bob" || got.Age != 25 {
		t.Fatalf("got %+v, want Bob/25", got)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	repo := newTestRepo(t)
	err := repo.Update(context.Background(), TestEntity{ID: 999, Name: "X", Age: 0})
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpsert_Insert(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	err := repo.Upsert(ctx, TestEntity{ID: 1, Name: "Alice", Age: 30})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := repo.GetByKey(ctx, 1)
	if got.Name != "Alice" {
		t.Fatalf("expected Alice, got %s", got.Name)
	}
}

func TestUpsert_Update(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	repo.Upsert(ctx, TestEntity{ID: 1, Name: "Alice", Age: 30})
	repo.Upsert(ctx, TestEntity{ID: 1, Name: "Bob", Age: 25})

	got, _ := repo.GetByKey(ctx, 1)
	if got.Name != "Bob" || got.Age != 25 {
		t.Fatalf("got %+v, want Bob/25", got)
	}
}

func TestListAll(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	repo.Insert(ctx, TestEntity{ID: 1, Name: "Alice", Age: 30})
	repo.Insert(ctx, TestEntity{ID: 2, Name: "Bob", Age: 25})

	all, err := repo.ListAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(all))
	}
}

func TestListAll_Empty(t *testing.T) {
	repo := newTestRepo(t)
	all, err := repo.ListAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 entities, got %d", len(all))
	}
}

func TestDeleteByKey(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	repo.Insert(ctx, TestEntity{ID: 1, Name: "Alice", Age: 30})
	err := repo.DeleteByKey(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.GetByKey(ctx, 1)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteByKey_NotFound(t *testing.T) {
	repo := newTestRepo(t)
	err := repo.DeleteByKey(context.Background(), 999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestExistsByKey(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	repo.Insert(ctx, TestEntity{ID: 1, Name: "Alice", Age: 30})

	exists, err := repo.ExistsByKey(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected exists=true")
	}

	exists, _ = repo.ExistsByKey(ctx, 999)
	if exists {
		t.Fatal("expected exists=false for missing key")
	}
}

func TestCount(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	n, _ := repo.Count(ctx)
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}

	repo.Insert(ctx, TestEntity{ID: 1, Name: "Alice", Age: 30})
	repo.Insert(ctx, TestEntity{ID: 2, Name: "Bob", Age: 25})

	n, _ = repo.Count(ctx)
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
}

func TestWithTx_Commit(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository[TestEntity](db, testCodec{})
	repo.CreateSchema(context.Background())

	err := WithTx(context.Background(), db, func(tx *sql.Tx) error {
		txRepo := NewRepository[TestEntity](tx, testCodec{})
		return txRepo.Insert(context.Background(), TestEntity{ID: 1, Name: "Alice", Age: 30})
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByKey(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Alice" {
		t.Fatalf("expected Alice, got %s", got.Name)
	}
}

func TestWithTx_Rollback(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository[TestEntity](db, testCodec{})
	repo.CreateSchema(context.Background())

	repo.Insert(context.Background(), TestEntity{ID: 1, Name: "Alice", Age: 30})

	err := WithTx(context.Background(), db, func(tx *sql.Tx) error {
		txRepo := NewRepository[TestEntity](tx, testCodec{})
		txRepo.Insert(context.Background(), TestEntity{ID: 2, Name: "Bob", Age: 25})
		return ErrNotFound // simulate error → rollback
	})
	if err != ErrNotFound {
		t.Fatalf("expected error, got %v", err)
	}

	n, _ := repo.Count(context.Background())
	if n != 1 {
		t.Fatalf("expected 1 after rollback, got %d", n)
	}
}
