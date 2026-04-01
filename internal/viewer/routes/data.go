package routes

import (
	"encoding/json"
	"net/http"

	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/storage"
)

func hasSchemaFields(cols []schema.Column) bool {
	for _, c := range cols {
		if c.Key || c.Required || c.Auto {
			return true
		}
	}
	return false
}

// RegisterData adds data/storage API endpoints
func RegisterData(mux *http.ServeMux, db *storage.DB, selfID string, selfEmail func() string, onSchemaChange func()) {
	if onSchemaChange == nil {
		onSchemaChange = func() {}
	}
	// List all tables (includes mode: orm/classic)
	handleGet(mux, "/api/data/tables", func(w http.ResponseWriter, r *http.Request) {
		tables, err := db.ListTables()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type tableEntry struct {
			Name         string `json:"name"`
			InsertPolicy string `json:"insert_policy"`
			CreatedAt    string `json:"created_at"`
			Mode         string `json:"mode"`
		}
		result := make([]tableEntry, len(tables))
		for i, t := range tables {
			mode := "classic"
			if db.IsORM(t.Name) {
				mode = "orm"
			}
			policy := t.InsertPolicy
			if mode == "orm" {
				access := db.GetAccess(t.Name)
				policy = access.Insert
			}
			result[i] = tableEntry{
				Name:         t.Name,
				InsertPolicy: policy,
				CreatedAt:    t.CreatedAt,
				Mode:         mode,
			}
		}
		writeJSON(w, result)
	})

	// List all ORM schemas (full column info + access policies)
	handleGet(mux, "/api/data/orm-schema", func(w http.ResponseWriter, r *http.Request) {
		schemas, err := db.GetAllSchemas()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, schemas)
	})

	// Create a new table (supports both classic ColumnDef and ORM schema formats)
	handlePost(mux, "/api/data/tables/create", func(w http.ResponseWriter, r *http.Request, req struct {
		Name    string              `json:"name"`
		Columns json.RawMessage     `json:"columns"`
	}) {
		if req.Name == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		if len(req.Columns) == 0 {
			http.Error(w, "at least one column required", http.StatusBadRequest)
			return
		}

		// Try ORM schema format first: columns have "key" or "required" fields
		var schemaCols []schema.Column
		if json.Unmarshal(req.Columns, &schemaCols) == nil && len(schemaCols) > 0 && hasSchemaFields(schemaCols) {
			tbl := &schema.Table{Name: req.Name, Columns: schemaCols}
			if err := tbl.Validate(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := db.CreateTableORM(tbl); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			onSchemaChange()
			writeJSON(w, map[string]string{
				"status": "created",
				"table":  req.Name,
				"mode":   "orm",
			})
			return
		}

		// Classic format
		var classicCols []storage.ColumnDef
		if err := json.Unmarshal(req.Columns, &classicCols); err != nil {
			http.Error(w, "invalid columns format", http.StatusBadRequest)
			return
		}
		if len(classicCols) == 0 {
			http.Error(w, "at least one column required", http.StatusBadRequest)
			return
		}
		if err := db.CreateTable(req.Name, classicCols); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{
			"status": "created",
			"table":  req.Name,
			"mode":   "classic",
		})
	})

	// Insert data into a table (ORM tables auto-generate guid/date and validate types)
	handlePost(mux, "/api/data/insert", func(w http.ResponseWriter, r *http.Request, req struct {
		Table string         `json:"table"`
		Data  map[string]any `json:"data"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		email := ""
		if selfEmail != nil {
			email = selfEmail()
		}
		id, err := db.OrmInsert(req.Table, selfID, email, req.Data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"status": "inserted",
			"id":     id,
		})
	})

	// Query data from a table
	handlePost(mux, "/api/data/query", func(w http.ResponseWriter, r *http.Request, req struct {
		Table   string   `json:"table"`
		Columns []string `json:"columns"`
		Where   string   `json:"where"`
		Args    []any    `json:"args"`
		Limit   int      `json:"limit"`
		Offset  int      `json:"offset"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		rows, err := db.SelectPaged(storage.SelectOpts{
			Table:   req.Table,
			Columns: req.Columns,
			Where:   req.Where,
			Args:    req.Args,
			Limit:   req.Limit,
			Offset:  req.Offset,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, rows)
	})

	// Describe table schema (ORM tables return typed JSON schema)
	handlePost(mux, "/api/data/tables/describe", func(w http.ResponseWriter, r *http.Request, req struct {
		Table string `json:"table"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		tbl, err := db.GetSchema(req.Table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if tbl != nil {
			writeJSON(w, map[string]any{
				"mode":    "orm",
				"schema":  tbl,
			})
			return
		}
		cols, err := db.DescribeTable(req.Table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"mode":    "classic",
			"columns": cols,
		})
	})

	// Delete a table (also cleans up ORM schema if present)
	handlePost(mux, "/api/data/tables/delete", func(w http.ResponseWriter, r *http.Request, req struct {
		Table string `json:"table"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		db.DeleteSchemaORM(req.Table)
		if err := db.DeleteTable(req.Table); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		onSchemaChange()
		writeJSON(w, map[string]string{
			"status": "deleted",
			"table":  req.Table,
		})
	})

	// Update a row
	handlePost(mux, "/api/data/update", func(w http.ResponseWriter, r *http.Request, req struct {
		Table string         `json:"table"`
		ID    int64          `json:"id"`
		Data  map[string]any `json:"data"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		if req.ID <= 0 {
			http.Error(w, "valid row id required", http.StatusBadRequest)
			return
		}
		if err := db.UpdateRow(req.Table, req.ID, req.Data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "updated"})
	})

	// Delete a row
	handlePost(mux, "/api/data/delete", func(w http.ResponseWriter, r *http.Request, req struct {
		Table string `json:"table"`
		ID    int64  `json:"id"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		if req.ID <= 0 {
			http.Error(w, "valid row id required", http.StatusBadRequest)
			return
		}
		if err := db.DeleteRow(req.Table, req.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
	})

	// ── ORM query endpoints ──

	// Find rows with filtering, ordering, pagination
	handlePost(mux, "/api/data/find", func(w http.ResponseWriter, r *http.Request, req struct {
		Table   string   `json:"table"`
		Where   string   `json:"where"`
		Args    []any    `json:"args"`
		Fields  []string `json:"fields"`
		Order   string   `json:"order"`
		Limit   int      `json:"limit"`
		Offset  int      `json:"offset"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		rows, err := db.SelectPaged(storage.SelectOpts{
			Table:   req.Table,
			Columns: req.Fields,
			Where:   req.Where,
			Args:    req.Args,
			Order:   req.Order,
			Limit:   req.Limit,
			Offset:  req.Offset,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if rows == nil {
			rows = []map[string]any{}
		}
		writeJSON(w, rows)
	})

	// Find single row
	handlePost(mux, "/api/data/find-one", func(w http.ResponseWriter, r *http.Request, req struct {
		Table  string   `json:"table"`
		Where  string   `json:"where"`
		Args   []any    `json:"args"`
		Fields []string `json:"fields"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		rows, err := db.SelectPaged(storage.SelectOpts{
			Table:   req.Table,
			Columns: req.Fields,
			Where:   req.Where,
			Args:    req.Args,
			Limit:   1,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(rows) == 0 {
			writeJSON(w, nil)
			return
		}
		writeJSON(w, rows[0])
	})

	// Get row by any column value
	handlePost(mux, "/api/data/get-by", func(w http.ResponseWriter, r *http.Request, req struct {
		Table  string `json:"table"`
		Column string `json:"column"`
		Value  any    `json:"value"`
	}) {
		if req.Table == "" || req.Column == "" {
			http.Error(w, "table and column required", http.StatusBadRequest)
			return
		}
		rows, err := db.SelectPaged(storage.SelectOpts{
			Table: req.Table,
			Where: req.Column + " = ?",
			Args:  []any{req.Value},
			Limit: 1,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(rows) == 0 {
			writeJSON(w, nil)
			return
		}
		writeJSON(w, rows[0])
	})

	// Check if rows exist
	handlePost(mux, "/api/data/exists", func(w http.ResponseWriter, r *http.Request, req struct {
		Table string `json:"table"`
		Where string `json:"where"`
		Args  []any  `json:"args"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		rows, err := db.SelectPaged(storage.SelectOpts{
			Table:   req.Table,
			Columns: []string{"1"},
			Where:   req.Where,
			Args:    req.Args,
			Limit:   1,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]bool{"exists": len(rows) > 0})
	})

	// Count rows
	handlePost(mux, "/api/data/count", func(w http.ResponseWriter, r *http.Request, req struct {
		Table string `json:"table"`
		Where string `json:"where"`
		Args  []any  `json:"args"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		where := req.Where
		rows, err := db.Aggregate(req.Table, "COUNT(*) as n", where, req.Args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		n := int64(0)
		if len(rows) > 0 {
			if v, ok := rows[0]["n"].(int64); ok {
				n = v
			}
		}
		writeJSON(w, map[string]int64{"count": n})
	})

	// Pluck single column values as flat array
	handlePost(mux, "/api/data/pluck", func(w http.ResponseWriter, r *http.Request, req struct {
		Table  string `json:"table"`
		Column string `json:"column"`
		Where  string `json:"where"`
		Args   []any  `json:"args"`
		Order  string `json:"order"`
		Limit  int    `json:"limit"`
	}) {
		if req.Table == "" || req.Column == "" {
			http.Error(w, "table and column required", http.StatusBadRequest)
			return
		}
		rows, err := db.SelectPaged(storage.SelectOpts{
			Table:   req.Table,
			Columns: []string{req.Column},
			Where:   req.Where,
			Args:    req.Args,
			Order:   req.Order,
			Limit:   req.Limit,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		vals := make([]any, len(rows))
		for i, row := range rows {
			vals[i] = row[req.Column]
		}
		writeJSON(w, vals)
	})

	// Distinct values for a column
	handlePost(mux, "/api/data/distinct", func(w http.ResponseWriter, r *http.Request, req struct {
		Table  string `json:"table"`
		Column string `json:"column"`
		Where  string `json:"where"`
		Args   []any  `json:"args"`
	}) {
		if req.Table == "" || req.Column == "" {
			http.Error(w, "table and column required", http.StatusBadRequest)
			return
		}
		vals, err := db.Distinct(req.Table, req.Column, req.Where, req.Args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if vals == nil {
			vals = []any{}
		}
		writeJSON(w, vals)
	})

	// Aggregate query (COUNT, SUM, MAX, MIN, AVG) with optional GROUP BY
	handlePost(mux, "/api/data/aggregate", func(w http.ResponseWriter, r *http.Request, req struct {
		Table   string `json:"table"`
		Expr    string `json:"expr"`
		Where   string `json:"where"`
		Args    []any  `json:"args"`
		GroupBy string `json:"group_by"`
	}) {
		if req.Table == "" || req.Expr == "" {
			http.Error(w, "table and expr required", http.StatusBadRequest)
			return
		}
		var rows []map[string]any
		var err error
		if req.GroupBy != "" {
			rows, err = db.AggregateGroupBy(req.Table, req.Expr, req.GroupBy, req.Where, req.Args...)
		} else {
			rows, err = db.Aggregate(req.Table, req.Expr, req.Where, req.Args...)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if rows == nil {
			rows = []map[string]any{}
		}
		writeJSON(w, rows)
	})

	// Update rows by WHERE clause
	handlePost(mux, "/api/data/update-where", func(w http.ResponseWriter, r *http.Request, req struct {
		Table string         `json:"table"`
		Data  map[string]any `json:"data"`
		Where string         `json:"where"`
		Args  []any          `json:"args"`
	}) {
		if req.Table == "" || req.Where == "" {
			http.Error(w, "table and where clause required", http.StatusBadRequest)
			return
		}
		n, err := db.UpdateWhere(req.Table, req.Data, req.Where, req.Args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]int64{"affected": n})
	})

	// Delete rows by WHERE clause
	handlePost(mux, "/api/data/delete-where", func(w http.ResponseWriter, r *http.Request, req struct {
		Table string `json:"table"`
		Where string `json:"where"`
		Args  []any  `json:"args"`
	}) {
		if req.Table == "" || req.Where == "" {
			http.Error(w, "table and where clause required", http.StatusBadRequest)
			return
		}
		n, err := db.DeleteWhere(req.Table, req.Where, req.Args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]int64{"affected": n})
	})

	// Upsert — insert or update by key column
	handlePost(mux, "/api/data/upsert", func(w http.ResponseWriter, r *http.Request, req struct {
		Table  string         `json:"table"`
		KeyCol string         `json:"key_col"`
		Data   map[string]any `json:"data"`
	}) {
		if req.Table == "" || req.KeyCol == "" {
			http.Error(w, "table and key_col required", http.StatusBadRequest)
			return
		}
		email := ""
		if selfEmail != nil {
			email = selfEmail()
		}
		id, err := db.Upsert(req.Table, req.KeyCol, selfID, email, req.Data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": "ok", "id": id})
	})

	// Add a column to a table
	handlePost(mux, "/api/data/tables/add-column", func(w http.ResponseWriter, r *http.Request, req struct {
		Table  string            `json:"table"`
		Column storage.ColumnDef `json:"column"`
	}) {
		if req.Table == "" || req.Column.Name == "" {
			http.Error(w, "table and column name required", http.StatusBadRequest)
			return
		}
		if err := db.AddColumn(req.Table, req.Column); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		onSchemaChange()
		writeJSON(w, map[string]string{"status": "added"})
	})

	// Drop a column from a table
	handlePost(mux, "/api/data/tables/drop-column", func(w http.ResponseWriter, r *http.Request, req struct {
		Table  string `json:"table"`
		Column string `json:"column"`
	}) {
		if req.Table == "" || req.Column == "" {
			http.Error(w, "table and column name required", http.StatusBadRequest)
			return
		}
		if err := db.DropColumn(req.Table, req.Column); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		onSchemaChange()
		writeJSON(w, map[string]string{"status": "dropped"})
	})

	// Set insert policy for a table
	handlePost(mux, "/api/data/tables/set-policy", func(w http.ResponseWriter, r *http.Request, req struct {
		Table  string `json:"table"`
		Policy string `json:"policy"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		switch req.Policy {
		case "owner", "email", "open", "group", "local":
			// valid
		default:
			http.Error(w, "policy must be owner, email, open, group, or local", http.StatusBadRequest)
			return
		}
		if db.IsORM(req.Table) {
			access := db.GetAccess(req.Table)
			access.Insert = req.Policy
			db.UpdateSchemaAccess(req.Table, &access)
		} else {
			if err := db.SetTableInsertPolicy(req.Table, req.Policy); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		writeJSON(w, map[string]string{
			"status": "updated",
			"policy": req.Policy,
		})
	})

	// Rename a table (also updates ORM schema if present)
	handlePost(mux, "/api/data/tables/rename", func(w http.ResponseWriter, r *http.Request, req struct {
		OldName string `json:"old_name"`
		NewName string `json:"new_name"`
	}) {
		if req.OldName == "" || req.NewName == "" {
			http.Error(w, "old and new name required", http.StatusBadRequest)
			return
		}
		if err := db.RenameTable(req.OldName, req.NewName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		db.RenameSchemaORM(req.OldName, req.NewName)
		onSchemaChange()
		writeJSON(w, map[string]string{
			"status":   "renamed",
			"new_name": req.NewName,
		})
	})

	// Export schema for any table (ORM returns stored schema, classic reads PRAGMA)
	handlePost(mux, "/api/data/tables/export-schema", func(w http.ResponseWriter, r *http.Request, req struct {
		Table string `json:"table"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		tbl, err := db.ExportSchema(r.Context(), req.Table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, tbl)
	})
}
