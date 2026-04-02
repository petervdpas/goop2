package routes

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/storage"
	"github.com/petervdpas/goop2/internal/ui/render"
)

func RegisterSchema(mux *http.ServeMux, peerDir string, db *storage.DB, onSchemaChange func()) {
	if onSchemaChange == nil {
		onSchemaChange = func() {}
	}
	schemasDir := filepath.Join(peerDir, "schemas")

	handleGet(mux, "/api/data/schemas", func(w http.ResponseWriter, r *http.Request) {
		entries, err := os.ReadDir(schemasDir)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, []any{})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type entry struct {
			Name    string         `json:"name"`
			Columns int            `json:"columns"`
			HasKey  bool           `json:"has_key"`
			Context bool           `json:"context"`
			Access  *schema.Access `json:"access,omitempty"`
		}
		var result []entry
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(schemasDir, e.Name()))
			if err != nil {
				continue
			}
			var tbl schema.Table
			if json.Unmarshal(data, &tbl) != nil {
				continue
			}
			hasKey := false
			for _, c := range tbl.Columns {
				if c.Key {
					hasKey = true
					break
				}
			}
			result = append(result, entry{
				Name:    tbl.Name,
				Columns: len(tbl.Columns),
				HasKey:  hasKey,
				Context: tbl.Context,
				Access:  tbl.Access,
			})
		}
		if result == nil {
			result = []entry{}
		}
		writeJSON(w, result)
	})

	handlePost(mux, "/api/data/schemas/get", func(w http.ResponseWriter, r *http.Request, req struct {
		Name string `json:"name"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		var tbl schema.Table
		path := filepath.Join(schemasDir, req.Name+".json")
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			if err := json.Unmarshal(data, &tbl); err != nil {
				http.Error(w, "invalid schema: "+err.Error(), http.StatusInternalServerError)
				return
			}
		} else if db != nil {
			stored, err := db.GetSchema(req.Name)
			if err != nil || stored == nil {
				http.Error(w, "schema not found", http.StatusNotFound)
				return
			}
			tbl = *stored
		} else {
			http.Error(w, "schema not found", http.StatusNotFound)
			return
		}
		writeJSON(w, tbl)
	})

	handlePost(mux, "/api/data/schemas/save", func(w http.ResponseWriter, r *http.Request, req schema.Table) {
		if req.Name == "" {
			http.Error(w, "schema name required", http.StatusBadRequest)
			return
		}
		if err := os.MkdirAll(schemasDir, 0755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data, err := json.MarshalIndent(req, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		path := filepath.Join(schemasDir, req.Name+".json")
		if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "saved", "name": req.Name})
	})

	handlePost(mux, "/api/data/schemas/delete", func(w http.ResponseWriter, r *http.Request, req struct {
		Name string `json:"name"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		path := filepath.Join(schemasDir, req.Name+".json")
		if err := os.Remove(path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
	})

	handlePost(mux, "/api/data/schemas/ddl", func(w http.ResponseWriter, r *http.Request, req schema.Table) {
		if req.Name == "" || len(req.Columns) == 0 {
			http.Error(w, "name and columns required", http.StatusBadRequest)
			return
		}
		ddl := req.DDL()
		writeJSON(w, map[string]string{
			"ddl":  ddl,
			"html": render.Highlight(ddl, "sql"),
		})
	})

	handlePost(mux, "/api/data/schemas/apply", func(w http.ResponseWriter, r *http.Request, req struct {
		Name string `json:"name"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if db == nil {
			http.Error(w, "database not available", http.StatusServiceUnavailable)
			return
		}
		path := filepath.Join(schemasDir, req.Name+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, "schema not found", http.StatusNotFound)
			return
		}
		var tbl schema.Table
		if err := json.Unmarshal(data, &tbl); err != nil {
			http.Error(w, "invalid schema: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := tbl.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := db.CreateTableORM(&tbl); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		onSchemaChange()
		writeJSON(w, map[string]string{"status": "created", "table": tbl.Name})
	})

	handlePost(mux, "/api/data/schemas/set-context", func(w http.ResponseWriter, r *http.Request, req struct {
		Name    string `json:"name"`
		Context bool   `json:"context"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		path := filepath.Join(schemasDir, req.Name+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, "schema not found", http.StatusNotFound)
			return
		}
		var tbl schema.Table
		if err := json.Unmarshal(data, &tbl); err != nil {
			http.Error(w, "invalid schema: "+err.Error(), http.StatusInternalServerError)
			return
		}
		tbl.Context = req.Context
		out, err := json.MarshalIndent(tbl, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(path, append(out, '\n'), 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if db != nil {
			db.UpdateSchemaContext(tbl.Name, req.Context)
		}
		onSchemaChange()
		writeJSON(w, map[string]any{"status": "updated", "context": req.Context})
	})

	handlePost(mux, "/api/data/schemas/set-access", func(w http.ResponseWriter, r *http.Request, req struct {
		Name   string        `json:"name"`
		Access schema.Access `json:"access"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if err := req.Access.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var tbl schema.Table
		path := filepath.Join(schemasDir, req.Name+".json")
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			if err := json.Unmarshal(data, &tbl); err != nil {
				http.Error(w, "invalid schema: "+err.Error(), http.StatusInternalServerError)
				return
			}
		} else if db != nil {
			stored, err := db.GetSchema(req.Name)
			if err != nil || stored == nil {
				http.Error(w, "schema not found", http.StatusNotFound)
				return
			}
			tbl = *stored
		} else {
			http.Error(w, "schema not found", http.StatusNotFound)
			return
		}
		tbl.Access = &req.Access
		out, err := json.MarshalIndent(tbl, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(path, append(out, '\n'), 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if db != nil {
			db.UpdateSchemaAccess(tbl.Name, &req.Access)
		}
		onSchemaChange()
		writeJSON(w, map[string]any{"status": "updated", "access": req.Access})
	})

	handlePost(mux, "/api/data/schemas/set-roles", func(w http.ResponseWriter, r *http.Request, req struct {
		Name  string                       `json:"name"`
		Roles map[string]schema.RoleAccess `json:"roles"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		// Try file first, fall back to ORM schema in DB
		var tbl schema.Table
		path := filepath.Join(schemasDir, req.Name+".json")
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			if err := json.Unmarshal(data, &tbl); err != nil {
				http.Error(w, "invalid schema: "+err.Error(), http.StatusInternalServerError)
				return
			}
		} else if db != nil {
			stored, err := db.GetSchema(req.Name)
			if err != nil || stored == nil {
				http.Error(w, "schema not found", http.StatusNotFound)
				return
			}
			tbl = *stored
		} else {
			http.Error(w, "schema not found", http.StatusNotFound)
			return
		}
		tbl.Roles = req.Roles
		out, err := json.MarshalIndent(tbl, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(path, append(out, '\n'), 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if db != nil {
			db.UpdateSchemaRoles(tbl.Name, req.Roles)
		}
		onSchemaChange()
		writeJSON(w, map[string]any{"status": "updated", "roles": req.Roles})
	})
}
