// internal/viewer/routes/data.go
package routes

import (
	"encoding/json"
	"net/http"

	"goop/internal/storage"
)

// RegisterData adds data/storage API endpoints
func RegisterData(mux *http.ServeMux, db *storage.DB, selfID string) {
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
			Name       string              `json:"name"`
			Columns    []storage.ColumnDef `json:"columns"`
			Visibility string              `json:"visibility"`
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

		if req.Visibility == "" {
			req.Visibility = "private"
		}

		if err := db.CreateTable(req.Name, req.Columns, req.Visibility); err != nil {
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

		id, err := db.Insert(req.Table, selfID, req.Data)
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
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Table == "" {
			http.Error(w, "table name required", http.StatusBadRequest)
			return
		}

		rows, err := db.Select(req.Table, req.Columns, req.Where, req.Args...)
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
}
