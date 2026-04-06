package orm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/storage"
	"github.com/petervdpas/goop2/internal/viewer/routes"
)

type ormWorld struct {
	mux            *http.ServeMux
	db             *storage.DB
	dir            string
	lastStatus     int
	lastBody       []byte
	lastInsertID   float64
	rows           []map[string]any
	singleResult   any
	existsResult   bool
	countResult    int64
	pluckResult    []any
	distinctResult []any
	aggregateRows  []map[string]any
	affectedCount  int64
}

var world *ormWorld

func aFreshORMDatabase() error {
	dir, err := os.MkdirTemp("", "orm-bdd-*")
	if err != nil {
		return err
	}
	db, err := storage.Open(dir)
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Join(dir, "site"), 0o755)

	mux := http.NewServeMux()
	routes.RegisterData(mux, db, "test-peer", func() string { return "test@example.com" }, nil)

	world = &ormWorld{mux: mux, db: db, dir: dir}
	return nil
}

func anORMTableWithColumns(tableName string, table *godog.Table) error {
	cols := parseColumnTable(table)
	tbl := &schema.Table{Name: tableName, Columns: cols}
	return world.db.CreateTableORM(tbl)
}

func theAccessPolicyIs(tableName, read, insert, update, del string) error {
	access := schema.Access{Read: read, Insert: insert, Update: update, Delete: del}
	world.db.UpdateSchemaAccess(tableName, &access)
	return nil
}

func parseColumnTable(table *godog.Table) []schema.Column {
	var cols []schema.Column
	for _, row := range table.Rows[1:] {
		col := schema.Column{
			Name:     row.Cells[0].Value,
			Type:     row.Cells[1].Value,
			Key:      row.Cells[2].Value == "true",
			Required: row.Cells[3].Value == "true",
			Auto:     row.Cells[4].Value == "true",
		}
		cols = append(cols, col)
	}
	return cols
}

func doPost(path string, body any) error {
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", path, strings.NewReader(string(b)))
	r.Header.Set("Content-Type", "application/json")
	world.mux.ServeHTTP(w, r)
	world.lastStatus = w.Code
	world.lastBody = w.Body.Bytes()
	return nil
}

func doGet(path string) error {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", path, nil)
	world.mux.ServeHTTP(w, r)
	world.lastStatus = w.Code
	world.lastBody = w.Body.Bytes()
	return nil
}

// ── Schema steps ────────────────────────────────────────────────────────────

func iListAllTables() error {
	return doGet("/api/data/tables")
}

func theTableListShouldContainWithMode(name, mode string) error {
	var tables []map[string]any
	json.Unmarshal(world.lastBody, &tables)
	for _, t := range tables {
		if t["name"] == name && t["mode"] == mode {
			return nil
		}
	}
	return fmt.Errorf("table %q with mode %q not found in %s", name, mode, string(world.lastBody))
}

func theTableListShouldNotContain(name string) error {
	var tables []map[string]any
	json.Unmarshal(world.lastBody, &tables)
	for _, t := range tables {
		if t["name"] == name {
			return fmt.Errorf("table %q should not be in list", name)
		}
	}
	return nil
}

func iDescribeTable(name string) error {
	return doPost("/api/data/tables/describe", map[string]any{"table": name})
}

func theDescribeModeShouldBe(expected string) error {
	var result map[string]any
	json.Unmarshal(world.lastBody, &result)
	if result["mode"] != expected {
		return fmt.Errorf("mode = %v, want %s", result["mode"], expected)
	}
	return nil
}

func theSchemaShouldHaveNColumns(n int) error {
	var result map[string]any
	json.Unmarshal(world.lastBody, &result)
	s, ok := result["schema"].(map[string]any)
	if !ok {
		return fmt.Errorf("no schema in response")
	}
	cols, ok := s["columns"].([]any)
	if !ok {
		return fmt.Errorf("no columns in schema")
	}
	if len(cols) != n {
		return fmt.Errorf("got %d columns, want %d", len(cols), n)
	}
	return nil
}

func iCreateTableWithNameAndColumns(name string, table *godog.Table) error {
	cols := parseColumnTable(table)
	colMaps := make([]map[string]any, len(cols))
	for i, c := range cols {
		m := map[string]any{"name": c.Name, "type": c.Type}
		if c.Key {
			m["key"] = true
		}
		if c.Required {
			m["required"] = true
		}
		if c.Auto {
			m["auto"] = true
		}
		colMaps[i] = m
	}
	return doPost("/api/data/tables/create", map[string]any{"name": name, "columns": colMaps})
}

func iCreateTableWithNameAndNoColumns(name string) error {
	return doPost("/api/data/tables/create", map[string]any{"name": name})
}

func iDeleteTable(name string) error {
	return doPost("/api/data/tables/delete", map[string]any{"table": name})
}

func iExportSchemaFor(name string) error {
	return doPost("/api/data/tables/export-schema", map[string]any{"table": name})
}

func iListORMSchemas() error {
	return doGet("/api/data/orm-schema")
}

// ── Insert steps ────────────────────────────────────────────────────────────

func iInsertIntoWithData(table string, dataJSON *godog.DocString) error {
	var data map[string]any
	if err := json.Unmarshal([]byte(dataJSON.Content), &data); err != nil {
		return err
	}
	if err := doPost("/api/data/insert", map[string]any{"table": table, "data": data}); err != nil {
		return err
	}
	if world.lastStatus == http.StatusOK {
		var result map[string]any
		json.Unmarshal(world.lastBody, &result)
		if id, ok := result["id"].(float64); ok {
			world.lastInsertID = id
		}
	}
	return nil
}

func theInsertShouldSucceedWithAnID() error {
	if world.lastStatus != http.StatusOK {
		return fmt.Errorf("status = %d, want 200; body = %s", world.lastStatus, string(world.lastBody))
	}
	var result map[string]any
	json.Unmarshal(world.lastBody, &result)
	if _, ok := result["id"]; !ok {
		return fmt.Errorf("no id in response")
	}
	return nil
}

// ── Find steps ──────────────────────────────────────────────────────────────

func iFindAllRowsIn(table string) error {
	if err := doPost("/api/data/find", map[string]any{"table": table}); err != nil {
		return err
	}
	world.rows = nil
	json.Unmarshal(world.lastBody, &world.rows)
	return nil
}

func iFindRowsInWhereWithArgs(table, where string, argsJSON *godog.DocString) error {
	var args []any
	json.Unmarshal([]byte(argsJSON.Content), &args)
	if err := doPost("/api/data/find", map[string]any{"table": table, "where": where, "args": args}); err != nil {
		return err
	}
	world.rows = nil
	json.Unmarshal(world.lastBody, &world.rows)
	return nil
}

func iFindRowsOrderedByLimit(table, order string, limit int) error {
	if err := doPost("/api/data/find", map[string]any{"table": table, "order": order, "limit": limit}); err != nil {
		return err
	}
	world.rows = nil
	json.Unmarshal(world.lastBody, &world.rows)
	return nil
}

func iFindRowsSelectingFields(table string, fieldsJSON *godog.DocString) error {
	var fields []string
	json.Unmarshal([]byte(fieldsJSON.Content), &fields)
	if err := doPost("/api/data/find", map[string]any{"table": table, "fields": fields}); err != nil {
		return err
	}
	world.rows = nil
	json.Unmarshal(world.lastBody, &world.rows)
	return nil
}

func iFindOneInWhereWithArgs(table, where string, argsJSON *godog.DocString) error {
	var args []any
	json.Unmarshal([]byte(argsJSON.Content), &args)
	if err := doPost("/api/data/find-one", map[string]any{"table": table, "where": where, "args": args}); err != nil {
		return err
	}
	world.singleResult = nil
	if strings.TrimSpace(string(world.lastBody)) != "null" {
		var m map[string]any
		json.Unmarshal(world.lastBody, &m)
		world.singleResult = m
	}
	return nil
}

func iGetFromByColumnValue(table, column, value string) error {
	if err := doPost("/api/data/get-by", map[string]any{"table": table, "column": column, "value": value}); err != nil {
		return err
	}
	world.singleResult = nil
	if strings.TrimSpace(string(world.lastBody)) != "null" {
		var m map[string]any
		json.Unmarshal(world.lastBody, &m)
		world.singleResult = m
	}
	return nil
}

// ── Result assertions ───────────────────────────────────────────────────────

func theResultShouldHaveNRows(n int) error {
	if len(world.rows) != n {
		return fmt.Errorf("got %d rows, want %d", len(world.rows), n)
	}
	return nil
}

func rowNFieldShouldBe(n int, field, expected string) error {
	if n > len(world.rows) {
		return fmt.Errorf("only %d rows, want row %d", len(world.rows), n)
	}
	val := fmt.Sprintf("%v", world.rows[n-1][field])
	if val != expected {
		return fmt.Errorf("row %d %s = %q, want %q", n, field, val, expected)
	}
	return nil
}

func rowNShouldHaveNonEmptyField(n int, field string) error {
	if n > len(world.rows) {
		return fmt.Errorf("only %d rows, want row %d", len(world.rows), n)
	}
	val := world.rows[n-1][field]
	if val == nil || fmt.Sprintf("%v", val) == "" {
		return fmt.Errorf("row %d %s is empty", n, field)
	}
	return nil
}

func theSingleResultFieldShouldBe(field, expected string) error {
	m, ok := world.singleResult.(map[string]any)
	if !ok {
		return fmt.Errorf("single result is not a map: %v", world.singleResult)
	}
	val := fmt.Sprintf("%v", m[field])
	if val != expected {
		return fmt.Errorf("%s = %q, want %q", field, val, expected)
	}
	return nil
}

func theSingleResultShouldBeNull() error {
	if world.singleResult != nil {
		return fmt.Errorf("expected null, got %v", world.singleResult)
	}
	return nil
}

// ── Exists / count steps ────────────────────────────────────────────────────

func iCheckExistsInWhereWithArgs(table, where string, argsJSON *godog.DocString) error {
	var args []any
	json.Unmarshal([]byte(argsJSON.Content), &args)
	if err := doPost("/api/data/exists", map[string]any{"table": table, "where": where, "args": args}); err != nil {
		return err
	}
	var result map[string]bool
	json.Unmarshal(world.lastBody, &result)
	world.existsResult = result["exists"]
	return nil
}

func existsShouldBe(expected string) error {
	want := expected == "true"
	if world.existsResult != want {
		return fmt.Errorf("exists = %v, want %v", world.existsResult, want)
	}
	return nil
}

func iCountRowsIn(table string) error {
	if err := doPost("/api/data/count", map[string]any{"table": table}); err != nil {
		return err
	}
	var result map[string]float64
	json.Unmarshal(world.lastBody, &result)
	world.countResult = int64(result["count"])
	return nil
}

func iCountRowsInWhereWithArgs(table, where string, argsJSON *godog.DocString) error {
	var args []any
	json.Unmarshal([]byte(argsJSON.Content), &args)
	if err := doPost("/api/data/count", map[string]any{"table": table, "where": where, "args": args}); err != nil {
		return err
	}
	var result map[string]float64
	json.Unmarshal(world.lastBody, &result)
	world.countResult = int64(result["count"])
	return nil
}

func theCountShouldBe(expected int) error {
	if world.countResult != int64(expected) {
		return fmt.Errorf("count = %d, want %d", world.countResult, expected)
	}
	return nil
}

// ── Pluck / distinct steps ──────────────────────────────────────────────────

func iPluckFromOrderedBy(column, table, order string) error {
	if err := doPost("/api/data/pluck", map[string]any{"table": table, "column": column, "order": order}); err != nil {
		return err
	}
	world.pluckResult = nil
	json.Unmarshal(world.lastBody, &world.pluckResult)
	return nil
}

func thePluckResultShouldBe(expectedJSON *godog.DocString) error {
	var expected []any
	json.Unmarshal([]byte(expectedJSON.Content), &expected)
	if len(world.pluckResult) != len(expected) {
		return fmt.Errorf("pluck got %d items, want %d", len(world.pluckResult), len(expected))
	}
	for i := range expected {
		if fmt.Sprintf("%v", world.pluckResult[i]) != fmt.Sprintf("%v", expected[i]) {
			return fmt.Errorf("pluck[%d] = %v, want %v", i, world.pluckResult[i], expected[i])
		}
	}
	return nil
}

func iGetDistinctFrom(column, table string) error {
	if err := doPost("/api/data/distinct", map[string]any{"table": table, "column": column}); err != nil {
		return err
	}
	world.distinctResult = nil
	json.Unmarshal(world.lastBody, &world.distinctResult)
	return nil
}

func theDistinctResultShouldHaveNValues(n int) error {
	if len(world.distinctResult) != n {
		return fmt.Errorf("distinct got %d values, want %d", len(world.distinctResult), n)
	}
	return nil
}

// ── Aggregate steps ─────────────────────────────────────────────────────────

func iAggregateOnTable(expr, table string) error {
	if err := doPost("/api/data/aggregate", map[string]any{"table": table, "expr": expr}); err != nil {
		return err
	}
	world.aggregateRows = nil
	json.Unmarshal(world.lastBody, &world.aggregateRows)
	return nil
}

func iAggregateOnTableGroupedBy(expr, table, groupBy string) error {
	if err := doPost("/api/data/aggregate", map[string]any{"table": table, "expr": expr, "group_by": groupBy}); err != nil {
		return err
	}
	world.aggregateRows = nil
	json.Unmarshal(world.lastBody, &world.aggregateRows)
	return nil
}

func theAggregateResultShouldHaveNRows(n int) error {
	if len(world.aggregateRows) != n {
		return fmt.Errorf("aggregate got %d rows, want %d", len(world.aggregateRows), n)
	}
	return nil
}

func aggregateRowNFieldShouldBe(n int, field string, expected int) error {
	if n > len(world.aggregateRows) {
		return fmt.Errorf("only %d aggregate rows", len(world.aggregateRows))
	}
	val := world.aggregateRows[n-1][field]
	var numVal float64
	switch v := val.(type) {
	case float64:
		numVal = v
	case int64:
		numVal = float64(v)
	case json.Number:
		numVal, _ = v.Float64()
	default:
		return fmt.Errorf("aggregate %s is %T (%v), not a number", field, val, val)
	}
	if int(numVal) != expected {
		return fmt.Errorf("aggregate row %d %s = %v, want %d", n, field, val, expected)
	}
	return nil
}

// ── Update / delete steps ───────────────────────────────────────────────────

func iUpdateTheInsertedRowInWithData(table string, dataJSON *godog.DocString) error {
	var data map[string]any
	json.Unmarshal([]byte(dataJSON.Content), &data)
	return doPost("/api/data/update", map[string]any{
		"table": table,
		"id":    world.lastInsertID,
		"data":  data,
	})
}

func iDeleteTheInsertedRowFrom(table string) error {
	return doPost("/api/data/delete", map[string]any{
		"table": table,
		"id":    world.lastInsertID,
	})
}

func iUpdateWhereInWithArgs(table, where string, payloadJSON *godog.DocString) error {
	var payload struct {
		Data map[string]any `json:"data"`
		Args []any          `json:"args"`
	}
	json.Unmarshal([]byte(payloadJSON.Content), &payload)
	if err := doPost("/api/data/update-where", map[string]any{
		"table": table,
		"data":  payload.Data,
		"where": where,
		"args":  payload.Args,
	}); err != nil {
		return err
	}
	if world.lastStatus == http.StatusOK {
		var result map[string]float64
		json.Unmarshal(world.lastBody, &result)
		world.affectedCount = int64(result["affected"])
	}
	return nil
}

func iDeleteWhereFromWithArgs(table, where string, argsJSON *godog.DocString) error {
	var args []any
	json.Unmarshal([]byte(argsJSON.Content), &args)
	if err := doPost("/api/data/delete-where", map[string]any{
		"table": table,
		"where": where,
		"args":  args,
	}); err != nil {
		return err
	}
	if world.lastStatus == http.StatusOK {
		var result map[string]float64
		json.Unmarshal(world.lastBody, &result)
		world.affectedCount = int64(result["affected"])
	}
	return nil
}

func theAffectedCountShouldBe(n int) error {
	if world.affectedCount != int64(n) {
		return fmt.Errorf("affected = %d, want %d", world.affectedCount, n)
	}
	return nil
}

func iUpsertInWithKeyColAndData(table, keyCol string, dataJSON *godog.DocString) error {
	var data map[string]any
	json.Unmarshal([]byte(dataJSON.Content), &data)
	return doPost("/api/data/upsert", map[string]any{
		"table":   table,
		"key_col": keyCol,
		"data":    data,
	})
}

// ── Policy / role steps ─────────────────────────────────────────────────────

func iSetInsertPolicyTo(table, policy string) error {
	return doPost("/api/data/tables/set-policy", map[string]any{
		"table":  table,
		"policy": policy,
	})
}

func iGetMyRoleFor(table string) error {
	return doPost("/api/data/role", map[string]any{"table": table})
}

func myRoleShouldBe(expected string) error {
	var result map[string]any
	json.Unmarshal(world.lastBody, &result)
	if result["role"] != expected {
		return fmt.Errorf("role = %v, want %s", result["role"], expected)
	}
	return nil
}

func iRenameTableTo(oldName, newName string) error {
	return doPost("/api/data/tables/rename", map[string]any{
		"old_name": oldName,
		"new_name": newName,
	})
}

func theResponseStatusShouldBe(expected int) error {
	if world.lastStatus != expected {
		return fmt.Errorf("status = %d, want %d; body = %s", world.lastStatus, expected, string(world.lastBody))
	}
	return nil
}

// ── Test runner ─────────────────────────────────────────────────────────────

func InitializeScenario(ctx *godog.ScenarioContext) {
	ctx.After(func(ctx2 context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if world != nil {
			if world.db != nil {
				world.db.Close()
			}
			os.RemoveAll(world.dir)
		}
		world = nil
		return ctx2, nil
	})

	ctx.Step(`^a fresh ORM database$`, aFreshORMDatabase)
	ctx.Step(`^an ORM table "([^"]*)" with columns:$`, anORMTableWithColumns)
	ctx.Step(`^the "([^"]*)" access policy is read="([^"]*)" insert="([^"]*)" update="([^"]*)" delete="([^"]*)"$`, theAccessPolicyIs)

	ctx.Step(`^I list all tables$`, iListAllTables)
	ctx.Step(`^the table list should contain "([^"]*)" with mode "([^"]*)"$`, theTableListShouldContainWithMode)
	ctx.Step(`^the table list should not contain "([^"]*)"$`, theTableListShouldNotContain)
	ctx.Step(`^I describe table "([^"]*)"$`, iDescribeTable)
	ctx.Step(`^the describe mode should be "([^"]*)"$`, theDescribeModeShouldBe)
	ctx.Step(`^the schema should have (\d+) columns$`, theSchemaShouldHaveNColumns)
	ctx.Step(`^I create a table with name "([^"]*)" and columns:$`, iCreateTableWithNameAndColumns)
	ctx.Step(`^I create a table with name "([^"]*)" and no columns$`, iCreateTableWithNameAndNoColumns)
	ctx.Step(`^I delete table "([^"]*)"$`, iDeleteTable)
	ctx.Step(`^I export schema for "([^"]*)"$`, iExportSchemaFor)
	ctx.Step(`^I list ORM schemas$`, iListORMSchemas)

	ctx.Step(`^I insert into "([^"]*)" with data:$`, iInsertIntoWithData)
	ctx.Step(`^the insert should succeed with an id$`, theInsertShouldSucceedWithAnID)
	ctx.Step(`^I find all rows in "([^"]*)"$`, iFindAllRowsIn)
	ctx.Step(`^I find rows in "([^"]*)" where "([^"]*)" with args:$`, iFindRowsInWhereWithArgs)
	ctx.Step(`^I find rows in "([^"]*)" ordered by "([^"]*)" limit (\d+)$`, iFindRowsOrderedByLimit)
	ctx.Step(`^I find rows in "([^"]*)" selecting fields:$`, iFindRowsSelectingFields)
	ctx.Step(`^I find one in "([^"]*)" where "([^"]*)" with args:$`, iFindOneInWhereWithArgs)
	ctx.Step(`^I get from "([^"]*)" by column "([^"]*)" value "([^"]*)"$`, iGetFromByColumnValue)

	ctx.Step(`^the result should have (\d+) rows?$`, theResultShouldHaveNRows)
	ctx.Step(`^row (\d+) field "([^"]*)" should be "([^"]*)"$`, rowNFieldShouldBe)
	ctx.Step(`^row (\d+) should have a non-empty "([^"]*)" field$`, rowNShouldHaveNonEmptyField)
	ctx.Step(`^the single result field "([^"]*)" should be "([^"]*)"$`, theSingleResultFieldShouldBe)
	ctx.Step(`^the single result should be null$`, theSingleResultShouldBeNull)

	ctx.Step(`^I check exists in "([^"]*)" where "([^"]*)" with args:$`, iCheckExistsInWhereWithArgs)
	ctx.Step(`^exists should be (true|false)$`, existsShouldBe)
	ctx.Step(`^I count rows in "([^"]*)"$`, iCountRowsIn)
	ctx.Step(`^I count rows in "([^"]*)" where "([^"]*)" with args:$`, iCountRowsInWhereWithArgs)
	ctx.Step(`^the count should be (\d+)$`, theCountShouldBe)

	ctx.Step(`^I pluck "([^"]*)" from "([^"]*)" ordered by "([^"]*)"$`, iPluckFromOrderedBy)
	ctx.Step(`^the pluck result should be:$`, thePluckResultShouldBe)
	ctx.Step(`^I get distinct "([^"]*)" from "([^"]*)"$`, iGetDistinctFrom)
	ctx.Step(`^the distinct result should have (\d+) values$`, theDistinctResultShouldHaveNValues)

	ctx.Step(`^I aggregate "([^"]*)" on "([^"]*)"$`, iAggregateOnTable)
	ctx.Step(`^I aggregate "([^"]*)" on "([^"]*)" grouped by "([^"]*)"$`, iAggregateOnTableGroupedBy)
	ctx.Step(`^the aggregate result should have (\d+) rows?$`, theAggregateResultShouldHaveNRows)
	ctx.Step(`^aggregate row (\d+) field "([^"]*)" should be (\d+)$`, aggregateRowNFieldShouldBe)

	ctx.Step(`^I update the inserted row in "([^"]*)" with data:$`, iUpdateTheInsertedRowInWithData)
	ctx.Step(`^I delete the inserted row from "([^"]*)"$`, iDeleteTheInsertedRowFrom)
	ctx.Step(`^I update-where in "([^"]*)" set data where "([^"]*)" with args:$`, iUpdateWhereInWithArgs)
	ctx.Step(`^I delete-where from "([^"]*)" where "([^"]*)" with args:$`, iDeleteWhereFromWithArgs)
	ctx.Step(`^the affected count should be (\d+)$`, theAffectedCountShouldBe)
	ctx.Step(`^I upsert in "([^"]*)" with key_col "([^"]*)" and data:$`, iUpsertInWithKeyColAndData)

	ctx.Step(`^I set the insert policy of "([^"]*)" to "([^"]*)"$`, iSetInsertPolicyTo)
	ctx.Step(`^I get my role for "([^"]*)"$`, iGetMyRoleFor)
	ctx.Step(`^my role should be "([^"]*)"$`, myRoleShouldBe)
	ctx.Step(`^I rename table "([^"]*)" to "([^"]*)"$`, iRenameTableTo)

	ctx.Step(`^the response status should be (\d+)$`, theResponseStatusShouldBe)
}

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"."},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("godog tests failed")
	}
}
