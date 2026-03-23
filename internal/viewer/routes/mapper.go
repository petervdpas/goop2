package routes

import (
	"net/http"
	"path/filepath"
	"sort"

	"github.com/petervdpas/goop2/internal/orm/mapper"
	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/storage"
)

func RegisterMapper(mux *http.ServeMux, peerDir string, db *storage.DB) {
	mappingsDir := filepath.Join(peerDir, "mappings")

	handleGet(mux, "/api/data/mappers", func(w http.ResponseWriter, r *http.Request) {
		mappings, err := mapper.LoadDir(mappingsDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type entry struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			FieldCount  int    `json:"field_count"`
		}
		result := make([]entry, 0, len(mappings))
		for _, m := range mappings {
			result = append(result, entry{
				Name:        m.Name,
				Description: m.Description,
				FieldCount:  len(m.Fields),
			})
		}
		writeJSON(w, result)
	})

	handlePost(mux, "/api/data/mappers/get", func(w http.ResponseWriter, r *http.Request, req struct {
		Name string `json:"name"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		m, err := mapper.Load(mappingsDir, req.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, m)
	})

	handlePost(mux, "/api/data/mappers/save", func(w http.ResponseWriter, r *http.Request, req mapper.Mapping) {
		if err := mapper.Save(mappingsDir, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"status": "saved", "name": req.Name})
	})

	handlePost(mux, "/api/data/mappers/delete", func(w http.ResponseWriter, r *http.Request, req struct {
		Name string `json:"name"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if err := mapper.Delete(mappingsDir, req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
	})

	handlePost(mux, "/api/data/mappers/preview", func(w http.ResponseWriter, r *http.Request, req struct {
		Name string       `json:"name"`
		Rows []schema.Row `json:"rows"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		m, err := mapper.Load(mappingsDir, req.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		results, err := m.ApplyMany(req.Rows)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, results)
	})

	handlePost(mux, "/api/data/mappers/execute", func(w http.ResponseWriter, r *http.Request, req struct {
		Name        string `json:"name"`
		SourceTable string `json:"source_table"`
		TargetTable string `json:"target_table"`
		Where       string `json:"where"`
		Args        []any  `json:"args"`
		Limit       int    `json:"limit"`
	}) {
		if req.Name == "" || req.SourceTable == "" || req.TargetTable == "" {
			http.Error(w, "name, source_table, and target_table required", http.StatusBadRequest)
			return
		}
		if db == nil {
			http.Error(w, "database not available", http.StatusServiceUnavailable)
			return
		}

		m, err := mapper.Load(mappingsDir, req.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		limit := req.Limit
		if limit <= 0 {
			limit = 10000
		}
		sourceRows, err := db.SelectPaged(storage.SelectOpts{
			Table: req.SourceTable,
			Where: req.Where,
			Args:  req.Args,
			Limit: limit,
		})
		if err != nil {
			http.Error(w, "source query: "+err.Error(), http.StatusInternalServerError)
			return
		}

		schemaRows := make([]schema.Row, len(sourceRows))
		for i, r := range sourceRows {
			schemaRows[i] = r
		}
		results, err := m.ApplyMany(schemaRows)
		if err != nil {
			http.Error(w, "mapping: "+err.Error(), http.StatusBadRequest)
			return
		}

		inserted := 0
		for _, row := range results {
			if _, err := db.Insert(req.TargetTable, "", "", row); err != nil {
				http.Error(w, "insert: "+err.Error(), http.StatusInternalServerError)
				return
			}
			inserted++
		}

		writeJSON(w, map[string]any{
			"status":   "executed",
			"inserted": inserted,
		})
	})

	handleGet(mux, "/api/data/mappers/transforms", func(w http.ResponseWriter, r *http.Request) {
		names := mapper.TransformNames()
		sort.Strings(names)
		writeJSON(w, names)
	})
}
