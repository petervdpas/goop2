package storage

import (
	"strings"
	"testing"
)

func TestValidateReadOnly(t *testing.T) {
	valid := []string{
		"SELECT * FROM t",
		"select count(*) from t",
		"  SELECT 1",
		"WITH cte AS (SELECT 1) SELECT * FROM cte",
		"SELECT 1;",
		"SELECT 1 ;  ",
	}
	for _, q := range valid {
		if err := validateReadOnly(q); err != nil {
			t.Fatalf("validateReadOnly(%q) should pass, got: %v", q, err)
		}
	}

	invalid := []string{
		"INSERT INTO t VALUES (1)",
		"UPDATE t SET x = 1",
		"DELETE FROM t",
		"DROP TABLE t",
		"SELECT 1; DROP TABLE t",
	}
	for _, q := range invalid {
		if err := validateReadOnly(q); err == nil {
			t.Fatalf("validateReadOnly(%q) should fail", q)
		}
	}
}

func TestValidateWrite(t *testing.T) {
	valid := []string{
		"INSERT INTO t VALUES (1)",
		"UPDATE t SET x = 1",
		"DELETE FROM t WHERE 1=1",
		"REPLACE INTO t VALUES (1)",
	}
	for _, q := range valid {
		if err := validateWrite(q); err != nil {
			t.Fatalf("validateWrite(%q) should pass, got: %v", q, err)
		}
	}

	invalid := []string{
		"SELECT * FROM t",
		"DROP TABLE t",
		"CREATE TABLE t (x TEXT)",
		"INSERT INTO t VALUES (1); DROP TABLE t",
	}
	for _, q := range invalid {
		if err := validateWrite(q); err == nil {
			t.Fatalf("validateWrite(%q) should fail", q)
		}
	}
}

func TestLuaQuery(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"val": "hello"})
	db.Insert("t", "p", "", map[string]any{"val": "world"})

	rows, err := db.LuaQuery("SELECT val FROM t ORDER BY val ASC")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["val"] != "hello" {
		t.Fatalf("first val = %v, want 'hello'", rows[0]["val"])
	}
}

func TestLuaQueryWithArgs(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "INTEGER"}}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"val": 10})
	db.Insert("t", "p", "", map[string]any{"val": 20})

	rows, err := db.LuaQuery("SELECT val FROM t WHERE val > ?", 15)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestLuaQueryRejectsWrite(t *testing.T) {
	db := testDB(t)

	_, err := db.LuaQuery("INSERT INTO t VALUES (1)")
	if err == nil {
		t.Fatal("expected error for non-SELECT query")
	}
}

func TestLuaExec(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "val", Type: "TEXT"}}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"val": "old"})

	n, err := db.LuaExec("UPDATE t SET val = ? WHERE val = ?", "new", "old")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("affected = %d, want 1", n)
	}

	rows, _ := db.Select("t", nil, "")
	if rows[0]["val"] != "new" {
		t.Fatalf("val = %v, want 'new'", rows[0]["val"])
	}
}

func TestLuaExecRejectsSelect(t *testing.T) {
	db := testDB(t)

	_, err := db.LuaExec("SELECT * FROM t")
	if err == nil {
		t.Fatal("expected error for SELECT in LuaExec")
	}
}

func TestLuaScalar(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{{Name: "score", Type: "INTEGER"}}
	db.CreateTable("t", cols)
	db.Insert("t", "p", "", map[string]any{"score": 10})
	db.Insert("t", "p", "", map[string]any{"score": 20})

	val, err := db.LuaScalar("SELECT SUM(score) FROM t")
	if err != nil {
		t.Fatal(err)
	}
	n, ok := val.(int64)
	if !ok || n != 30 {
		t.Fatalf("scalar = %v (%T), want 30", val, val)
	}
}

func TestLuaScalarRejectsWrite(t *testing.T) {
	db := testDB(t)

	_, err := db.LuaScalar("DELETE FROM t")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestChatMessageRoundtrip(t *testing.T) {
	db := testDB(t)

	if err := db.StoreChatMessage("peer2", "me", "hello", 1000); err != nil {
		t.Fatal(err)
	}
	if err := db.StoreChatMessage("peer2", "peer2", "hi back", 2000); err != nil {
		t.Fatal(err)
	}

	msgs, err := db.GetChatHistory("peer2", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Fatalf("first message = %q, want 'hello'", msgs[0].Content)
	}
	if msgs[0].From != "me" {
		t.Fatalf("from = %q, want 'me'", msgs[0].From)
	}
	if msgs[1].Content != "hi back" {
		t.Fatalf("second message = %q, want 'hi back'", msgs[1].Content)
	}
}

func TestChatHistoryOrdering(t *testing.T) {
	db := testDB(t)

	db.StoreChatMessage("peer2", "me", "first", 1000)
	db.StoreChatMessage("peer2", "me", "second", 2000)
	db.StoreChatMessage("peer2", "me", "third", 3000)

	msgs, _ := db.GetChatHistory("peer2", 2)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages with limit, got %d", len(msgs))
	}
	if msgs[0].Content != "second" {
		t.Fatalf("first = %q, want 'second' (oldest of last 2)", msgs[0].Content)
	}
}

func TestChatHistoryDefaultLimit(t *testing.T) {
	db := testDB(t)

	db.StoreChatMessage("p", "me", "x", 1000)
	msgs, _ := db.GetChatHistory("p", 0)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message with default limit, got %d", len(msgs))
	}
}

func TestChatHistoryEmpty(t *testing.T) {
	db := testDB(t)

	msgs, err := db.GetChatHistory("nobody", 50)
	if err != nil {
		t.Fatal(err)
	}
	if msgs == nil {
		t.Fatal("expected empty slice, not nil")
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
}

func TestChatHistoryIsolation(t *testing.T) {
	db := testDB(t)

	db.StoreChatMessage("peer_a", "me", "to a", 1000)
	db.StoreChatMessage("peer_b", "me", "to b", 1000)

	msgs, _ := db.GetChatHistory("peer_a", 50)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for peer_a, got %d", len(msgs))
	}
}

func TestClearChatHistory(t *testing.T) {
	db := testDB(t)

	db.StoreChatMessage("peer2", "me", "hello", 1000)
	db.StoreChatMessage("peer2", "me", "world", 2000)

	if err := db.ClearChatHistory("peer2"); err != nil {
		t.Fatal(err)
	}

	msgs, _ := db.GetChatHistory("peer2", 50)
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages after clear, got %d", len(msgs))
	}
}

func TestChatMessageFIFOCap(t *testing.T) {
	db := testDB(t)

	for i := range chatHistoryCap + 10 {
		db.StoreChatMessage("peer2", "me", "msg", int64(i))
	}

	msgs, _ := db.GetChatHistory("peer2", 0)
	if len(msgs) > chatHistoryCap {
		t.Fatalf("expected at most %d messages, got %d", chatHistoryCap, len(msgs))
	}
}

func TestDumpSQL(t *testing.T) {
	db := testDB(t)

	cols := []ColumnDef{
		{Name: "title", Type: "TEXT", NotNull: true},
		{Name: "count", Type: "INTEGER", Default: "0"},
	}
	db.CreateTable("posts", cols)
	db.Insert("posts", "p", "a@b.com", map[string]any{"title": "Hello", "count": 42})

	dump, err := db.DumpSQL()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(dump, "CREATE TABLE") {
		t.Fatal("dump should contain CREATE TABLE")
	}
	if !strings.Contains(dump, "INSERT INTO") {
		t.Fatal("dump should contain INSERT INTO")
	}
	if !strings.Contains(dump, "Hello") {
		t.Fatal("dump should contain data")
	}
}

func TestDumpSQLEmpty(t *testing.T) {
	db := testDB(t)

	dump, err := db.DumpSQL()
	if err != nil {
		t.Fatal(err)
	}
	if dump != "" {
		t.Fatalf("expected empty dump, got %q", dump)
	}
}

func TestSqlEscapeValue(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, "NULL"},
		{int64(42), "42"},
		{float64(3.14), "3.14"},
		{"hello", "'hello'"},
		{"it's", "'it''s'"},
		{[]byte{0xDE, 0xAD}, "X'dead'"},
		{true, "1"},
		{false, "0"},
	}
	for _, tc := range cases {
		got := sqlEscapeValue(tc.in)
		if got != tc.want {
			t.Fatalf("sqlEscapeValue(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
