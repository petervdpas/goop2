package routes

import (
	"net/http"
	"path/filepath"
	"sort"

	"github.com/petervdpas/goop2/internal/orm/mapper"
	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/storage"
)

func RegisterTransformations(mux *http.ServeMux, peerDir string, db *storage.DB) {
	txDir := filepath.Join(peerDir, "transformations")

	handleGet(mux, "/api/data/transformations", func(w http.ResponseWriter, r *http.Request) {
		items, err := mapper.LoadDir(txDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type entry struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			FieldCount  int    `json:"field_count"`
			SourceType  string `json:"source_type"`
			TargetType  string `json:"target_type"`
		}
		result := make([]entry, 0, len(items))
		for _, t := range items {
			result = append(result, entry{
				Name:        t.Name,
				Description: t.Description,
				FieldCount:  len(t.Fields),
				SourceType:  t.Source.Type,
				TargetType:  t.Target.Type,
			})
		}
		writeJSON(w, result)
	})

	handlePost(mux, "/api/data/transformations/get", func(w http.ResponseWriter, r *http.Request, req struct {
		Name string `json:"name"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		t, err := mapper.Load(txDir, req.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, t)
	})

	handlePost(mux, "/api/data/transformations/save", func(w http.ResponseWriter, r *http.Request, req mapper.Transformation) {
		if err := mapper.Save(txDir, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"status": "saved", "name": req.Name})
	})

	handlePost(mux, "/api/data/transformations/delete", func(w http.ResponseWriter, r *http.Request, req struct {
		Name string `json:"name"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if err := mapper.Delete(txDir, req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
	})

	handlePost(mux, "/api/data/transformations/preview", func(w http.ResponseWriter, r *http.Request, req struct {
		Name string       `json:"name"`
		Rows []schema.Row `json:"rows"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		t, err := mapper.Load(txDir, req.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		results, err := t.ApplyMany(req.Rows)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, results)
	})

	handlePost(mux, "/api/data/transformations/execute", func(w http.ResponseWriter, r *http.Request, req struct {
		Name  string `json:"name"`
		Where string `json:"where"`
		Args  []any  `json:"args"`
		Limit int    `json:"limit"`
	}) {
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if db == nil {
			http.Error(w, "database not available", http.StatusServiceUnavailable)
			return
		}

		t, err := mapper.Load(txDir, req.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		if t.Source.Type != "table" || t.Source.Name == "" {
			http.Error(w, "execute requires source type 'table' with a name", http.StatusBadRequest)
			return
		}
		if t.Target.Type != "table" || t.Target.Name == "" {
			http.Error(w, "execute requires target type 'table' with a name", http.StatusBadRequest)
			return
		}

		limit := req.Limit
		if limit <= 0 {
			limit = 10000
		}
		sourceRows, err := db.SelectPaged(storage.SelectOpts{
			Table: t.Source.Name,
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
		results, err := t.ApplyMany(schemaRows)
		if err != nil {
			http.Error(w, "transform: "+err.Error(), http.StatusBadRequest)
			return
		}

		inserted := 0
		for _, row := range results {
			if _, err := db.Insert(t.Target.Name, "", "", row); err != nil {
				http.Error(w, "insert: "+err.Error(), http.StatusInternalServerError)
				return
			}
			inserted++
		}

		writeJSON(w, map[string]any{
			"status":   "executed",
			"source":   t.Source.Name,
			"target":   t.Target.Name,
			"inserted": inserted,
		})
	})

	handleGet(mux, "/api/data/transformations/transforms", func(w http.ResponseWriter, r *http.Request) {
		names := mapper.TransformNames()
		sort.Strings(names)
		writeJSON(w, names)
	})
}
