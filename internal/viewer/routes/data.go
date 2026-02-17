package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/storage"
)

// RegisterData adds data/storage API endpoints
func RegisterData(mux *http.ServeMux, db *storage.DB, selfID string, selfEmail func() string) {
	// List all tables
	handleGet(mux, "/api/data/tables", func(w http.ResponseWriter, r *http.Request) {
		tables, err := db.ListTables()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, tables)
	})

	// Create a new table
	handlePost(mux, "/api/data/tables/create", func(w http.ResponseWriter, r *http.Request, req struct {
		Name    string             `json:"name"`
		Columns []storage.ColumnDef `json:"columns"`
	}) {
		if req.Name == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		if len(req.Columns) == 0 {
			http.Error(w, "at least one column required", http.StatusBadRequest)
			return
		}
		if err := db.CreateTable(req.Name, req.Columns); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{
			"status": "created",
			"table":  req.Name,
		})
	})

	// Insert data into a table
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
		id, err := db.Insert(req.Table, selfID, email, req.Data)
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

	// Describe table schema
	handlePost(mux, "/api/data/tables/describe", func(w http.ResponseWriter, r *http.Request, req struct {
		Table string `json:"table"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		cols, err := db.DescribeTable(req.Table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, cols)
	})

	// Delete a table
	handlePost(mux, "/api/data/tables/delete", func(w http.ResponseWriter, r *http.Request, req struct {
		Table string `json:"table"`
	}) {
		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}
		if err := db.DeleteTable(req.Table); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
		case "owner", "email", "open":
			// valid
		default:
			http.Error(w, "policy must be owner, email, or open", http.StatusBadRequest)
			return
		}
		if err := db.SetTableInsertPolicy(req.Table, req.Policy); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{
			"status": "updated",
			"policy": req.Policy,
		})
	})

	// Rename a table
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
		writeJSON(w, map[string]string{
			"status":   "renamed",
			"new_name": req.NewName,
		})
	})
}
