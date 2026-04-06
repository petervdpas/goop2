package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ormschema "github.com/petervdpas/goop2/internal/orm/schema"
)

func dataTestMux(t *testing.T) *http.ServeMux {
	t.Helper()
	d, _ := testDeps(t)

	tbl := &ormschema.Table{
		Name:      "items",
		SystemKey: true,
		Columns: []ormschema.Column{
			{Name: "title", Type: "text", Required: true},
			{Name: "status", Type: "text"},
		},
		Access: &ormschema.Access{Read: "open", Insert: "open", Update: "owner", Delete: "owner"},
	}
	if err := d.DB.CreateTableORM(tbl); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterData(mux, d.DB, "test-peer", func() string { return "test@example.com" }, nil)
	return mux
}

func postJSON(t *testing.T, mux *http.ServeMux, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", path, strings.NewReader(string(b)))
	r.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, r)
	return w
}

func getJSON(t *testing.T, mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", path, nil)
	mux.ServeHTTP(w, r)
	return w
}

func TestDataListTables(t *testing.T) {
	mux := dataTestMux(t)
	w := getJSON(t, mux, "/api/data/tables")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var tables []struct {
		Name string `json:"name"`
		Mode string `json:"mode"`
	}
	json.NewDecoder(w.Body).Decode(&tables)

	found := false
	for _, tbl := range tables {
		if tbl.Name == "items" {
			found = true
			if tbl.Mode != "orm" {
				t.Fatalf("items mode = %q, want orm", tbl.Mode)
			}
		}
	}
	if !found {
		t.Fatal("items table not found")
	}
}

func TestDataListTables_rejectsPost(t *testing.T) {
	mux := dataTestMux(t)
	w := postJSON(t, mux, "/api/data/tables", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestDataInsertAndFind(t *testing.T) {
	mux := dataTestMux(t)

	w := postJSON(t, mux, "/api/data/insert", map[string]any{
		"table": "items",
		"data":  map[string]any{"title": "first", "status": "draft"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("insert status = %d, body = %s", w.Code, w.Body.String())
	}

	w = postJSON(t, mux, "/api/data/find", map[string]any{
		"table": "items",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("find status = %d", w.Code)
	}

	var rows []map[string]any
	json.NewDecoder(w.Body).Decode(&rows)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["title"] != "first" {
		t.Fatalf("title = %v, want first", rows[0]["title"])
	}
}

func TestDataInsert_missingTable(t *testing.T) {
	mux := dataTestMux(t)
	w := postJSON(t, mux, "/api/data/insert", map[string]any{
		"data": map[string]any{"title": "x"},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDataFindOne(t *testing.T) {
	mux := dataTestMux(t)

	postJSON(t, mux, "/api/data/insert", map[string]any{
		"table": "items",
		"data":  map[string]any{"title": "alpha", "status": "done"},
	})
	postJSON(t, mux, "/api/data/insert", map[string]any{
		"table": "items",
		"data":  map[string]any{"title": "beta", "status": "draft"},
	})

	w := postJSON(t, mux, "/api/data/find-one", map[string]any{
		"table": "items",
		"where": "title = ?",
		"args":  []any{"beta"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var row map[string]any
	json.NewDecoder(w.Body).Decode(&row)
	if row["title"] != "beta" {
		t.Fatalf("title = %v, want beta", row["title"])
	}
}

func TestDataFindOne_noMatch(t *testing.T) {
	mux := dataTestMux(t)
	w := postJSON(t, mux, "/api/data/find-one", map[string]any{
		"table": "items",
		"where": "title = ?",
		"args":  []any{"nonexistent"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if body := strings.TrimSpace(w.Body.String()); body != "null" {
		t.Fatalf("body = %q, want null", body)
	}
}

func TestDataCount(t *testing.T) {
	mux := dataTestMux(t)
	postJSON(t, mux, "/api/data/insert", map[string]any{
		"table": "items",
		"data":  map[string]any{"title": "one"},
	})
	postJSON(t, mux, "/api/data/insert", map[string]any{
		"table": "items",
		"data":  map[string]any{"title": "two"},
	})

	w := postJSON(t, mux, "/api/data/count", map[string]any{
		"table": "items",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var result map[string]int64
	json.NewDecoder(w.Body).Decode(&result)
	if result["count"] != 2 {
		t.Fatalf("count = %d, want 2", result["count"])
	}
}

func TestDataExists(t *testing.T) {
	mux := dataTestMux(t)
	postJSON(t, mux, "/api/data/insert", map[string]any{
		"table": "items",
		"data":  map[string]any{"title": "present"},
	})

	w := postJSON(t, mux, "/api/data/exists", map[string]any{
		"table": "items",
		"where": "title = ?",
		"args":  []any{"present"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var result map[string]bool
	json.NewDecoder(w.Body).Decode(&result)
	if !result["exists"] {
		t.Fatal("exists should be true")
	}

	w = postJSON(t, mux, "/api/data/exists", map[string]any{
		"table": "items",
		"where": "title = ?",
		"args":  []any{"missing"},
	})
	json.NewDecoder(w.Body).Decode(&result)
	if result["exists"] {
		t.Fatal("exists should be false for missing row")
	}
}

func TestDataPluck(t *testing.T) {
	mux := dataTestMux(t)
	postJSON(t, mux, "/api/data/insert", map[string]any{
		"table": "items",
		"data":  map[string]any{"title": "aaa", "status": "done"},
	})
	postJSON(t, mux, "/api/data/insert", map[string]any{
		"table": "items",
		"data":  map[string]any{"title": "bbb", "status": "draft"},
	})

	w := postJSON(t, mux, "/api/data/pluck", map[string]any{
		"table":  "items",
		"column": "title",
		"order":  "title ASC",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var vals []any
	json.NewDecoder(w.Body).Decode(&vals)
	if len(vals) != 2 {
		t.Fatalf("got %d values, want 2", len(vals))
	}
	if vals[0] != "aaa" || vals[1] != "bbb" {
		t.Fatalf("got %v, want [aaa bbb]", vals)
	}
}

func TestDataGetBy(t *testing.T) {
	mux := dataTestMux(t)
	postJSON(t, mux, "/api/data/insert", map[string]any{
		"table": "items",
		"data":  map[string]any{"title": "target", "status": "active"},
	})

	w := postJSON(t, mux, "/api/data/get-by", map[string]any{
		"table":  "items",
		"column": "status",
		"value":  "active",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var row map[string]any
	json.NewDecoder(w.Body).Decode(&row)
	if row["title"] != "target" {
		t.Fatalf("title = %v, want target", row["title"])
	}
}

func TestDataCreateAndDeleteTable(t *testing.T) {
	mux := dataTestMux(t)

	w := postJSON(t, mux, "/api/data/tables/create", map[string]any{
		"name": "new_tbl",
		"columns": []map[string]any{
			{"name": "id", "type": "guid", "key": true, "required": true, "auto": true},
			{"name": "val", "type": "text"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var result map[string]string
	json.NewDecoder(w.Body).Decode(&result)
	if result["mode"] != "orm" {
		t.Fatalf("mode = %q, want orm", result["mode"])
	}

	w = postJSON(t, mux, "/api/data/tables/delete", map[string]any{
		"table": "new_tbl",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d", w.Code)
	}
}

func TestDataCreateTable_missingName(t *testing.T) {
	mux := dataTestMux(t)
	w := postJSON(t, mux, "/api/data/tables/create", map[string]any{
		"columns": []map[string]any{{"name": "x", "type": "text"}},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDataCreateTable_missingColumns(t *testing.T) {
	mux := dataTestMux(t)
	w := postJSON(t, mux, "/api/data/tables/create", map[string]any{
		"name": "empty_tbl",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDataDescribeTable(t *testing.T) {
	mux := dataTestMux(t)
	w := postJSON(t, mux, "/api/data/tables/describe", map[string]any{
		"table": "items",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	if result["mode"] != "orm" {
		t.Fatalf("mode = %v, want orm", result["mode"])
	}
}

func TestDataSetPolicy(t *testing.T) {
	mux := dataTestMux(t)

	w := postJSON(t, mux, "/api/data/tables/set-policy", map[string]any{
		"table":  "items",
		"policy": "group",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	w = postJSON(t, mux, "/api/data/tables/set-policy", map[string]any{
		"table":  "items",
		"policy": "invalid",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDataRole(t *testing.T) {
	mux := dataTestMux(t)
	w := postJSON(t, mux, "/api/data/role", map[string]any{
		"table": "items",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	if result["role"] != "owner" {
		t.Fatalf("role = %v, want owner", result["role"])
	}
}

func TestDataUpdateWhere(t *testing.T) {
	mux := dataTestMux(t)
	postJSON(t, mux, "/api/data/insert", map[string]any{
		"table": "items",
		"data":  map[string]any{"title": "old", "status": "draft"},
	})

	w := postJSON(t, mux, "/api/data/update-where", map[string]any{
		"table": "items",
		"data":  map[string]any{"status": "published"},
		"where": "title = ?",
		"args":  []any{"old"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var result map[string]int64
	json.NewDecoder(w.Body).Decode(&result)
	if result["affected"] != 1 {
		t.Fatalf("affected = %d, want 1", result["affected"])
	}
}

func TestDataDeleteWhere(t *testing.T) {
	mux := dataTestMux(t)
	postJSON(t, mux, "/api/data/insert", map[string]any{
		"table": "items",
		"data":  map[string]any{"title": "remove-me"},
	})

	w := postJSON(t, mux, "/api/data/delete-where", map[string]any{
		"table": "items",
		"where": "title = ?",
		"args":  []any{"remove-me"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var result map[string]int64
	json.NewDecoder(w.Body).Decode(&result)
	if result["affected"] != 1 {
		t.Fatalf("affected = %d, want 1", result["affected"])
	}
}

func TestDataUpsert(t *testing.T) {
	mux := dataTestMux(t)

	postJSON(t, mux, "/api/data/insert", map[string]any{
		"table": "items",
		"data":  map[string]any{"title": "upsert-key", "status": "v1"},
	})

	w := postJSON(t, mux, "/api/data/upsert", map[string]any{
		"table":   "items",
		"key_col": "title",
		"data":    map[string]any{"title": "upsert-key", "status": "v2"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	w = postJSON(t, mux, "/api/data/find-one", map[string]any{
		"table": "items",
		"where": "title = ?",
		"args":  []any{"upsert-key"},
	})
	var row map[string]any
	json.NewDecoder(w.Body).Decode(&row)
	if row["status"] != "v2" {
		t.Fatalf("status = %v, want v2", row["status"])
	}
}

func TestDataOrmSchema(t *testing.T) {
	mux := dataTestMux(t)
	w := getJSON(t, mux, "/api/data/orm-schema")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestDataExportSchema(t *testing.T) {
	mux := dataTestMux(t)
	w := postJSON(t, mux, "/api/data/tables/export-schema", map[string]any{
		"table": "items",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
