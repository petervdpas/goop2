// internal/viewer/routes/data.go
package routes

import (
	"encoding/json"
	"net/http"

	"goop/internal/storage"
)

// RegisterData adds data/storage API endpoints
func RegisterData(mux *http.ServeMux, db *storage.DB, selfID string, selfEmail func() string) {
	// List all tables
	mux.HandleFunc("/api/data/tables", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		tables, err := db.ListTables()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tables)
	})

	// Create a new table
	mux.HandleFunc("/api/data/tables/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Name    string             `json:"name"`
			Columns []storage.ColumnDef `json:"columns"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "created",
			"table":  req.Name,
		})
	})

	// Insert data into a table
	mux.HandleFunc("/api/data/insert", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Table string                 `json:"table"`
			Data  map[string]interface{} `json:"data"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "inserted",
			"id":     id,
		})
	})

	// Query data from a table
	mux.HandleFunc("/api/data/query", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Table   string        `json:"table"`
			Columns []string      `json:"columns"`
			Where   string        `json:"where"`
			Args    []interface{} `json:"args"`
			Limit   int           `json:"limit"`
			Offset  int           `json:"offset"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rows)
	})

	// Describe table schema
	mux.HandleFunc("/api/data/tables/describe", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Table string `json:"table"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}

		cols, err := db.DescribeTable(req.Table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cols)
	})

	// Delete a table
	mux.HandleFunc("/api/data/tables/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Table string `json:"table"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}

		if err := db.DeleteTable(req.Table); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "deleted",
			"table":  req.Table,
		})
	})

	// Update a row
	mux.HandleFunc("/api/data/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Table string                 `json:"table"`
			ID    int64                  `json:"id"`
			Data  map[string]interface{} `json:"data"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "updated",
		})
	})

	// Delete a row
	mux.HandleFunc("/api/data/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Table string `json:"table"`
			ID    int64  `json:"id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "deleted",
		})
	})

	// Add a column to a table
	mux.HandleFunc("/api/data/tables/add-column", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Table  string            `json:"table"`
			Column storage.ColumnDef `json:"column"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Table == "" || req.Column.Name == "" {
			http.Error(w, "table and column name required", http.StatusBadRequest)
			return
		}

		if err := db.AddColumn(req.Table, req.Column); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})
	})

	// Drop a column from a table
	mux.HandleFunc("/api/data/tables/drop-column", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Table  string `json:"table"`
			Column string `json:"column"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Table == "" || req.Column == "" {
			http.Error(w, "table and column name required", http.StatusBadRequest)
			return
		}

		if err := db.DropColumn(req.Table, req.Column); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "dropped"})
	})

	// Rename a table
	mux.HandleFunc("/api/data/tables/rename", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			OldName string `json:"old_name"`
			NewName string `json:"new_name"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.OldName == "" || req.NewName == "" {
			http.Error(w, "old and new name required", http.StatusBadRequest)
			return
		}

		if err := db.RenameTable(req.OldName, req.NewName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "renamed",
			"new_name": req.NewName,
		})
	})
}
