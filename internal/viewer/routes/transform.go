package routes

import (
	"net/http"
	"os"
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

		var sourceRows []schema.Row

		if t.Source.Type == "table" {
			if t.Source.Name == "" {
				http.Error(w, "table source requires a name", http.StatusBadRequest)
				return
			}
			limit := req.Limit
			if limit <= 0 {
				limit = 10000
			}
			dbRows, err := db.SelectPaged(storage.SelectOpts{
				Table: t.Source.Name,
				Where: req.Where,
				Args:  req.Args,
				Limit: limit,
			})
			if err != nil {
				http.Error(w, "source query: "+err.Error(), http.StatusInternalServerError)
				return
			}
			sourceRows = make([]schema.Row, len(dbRows))
			for i, r := range dbRows {
				sourceRows[i] = r
			}
		} else {
			reader, err := mapper.NewSourceReader(t.Source)
			if err != nil {
				http.Error(w, "source: "+err.Error(), http.StatusBadRequest)
				return
			}
			sourceRows, err = reader.Read()
			if err != nil {
				http.Error(w, "source read: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		results, err := t.ApplyMany(sourceRows)
		if err != nil {
			http.Error(w, "transform: "+err.Error(), http.StatusBadRequest)
			return
		}

		if t.Target.Type == "table" {
			if t.Target.Name == "" {
				http.Error(w, "table target requires a name", http.StatusBadRequest)
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
				"target":   t.Target.Name,
				"inserted": inserted,
			})
		} else {
			writer, err := mapper.NewTargetWriter(t.Target)
			if err != nil {
				http.Error(w, "target: "+err.Error(), http.StatusBadRequest)
				return
			}
			written, err := writer.Write(results)
			if err != nil {
				http.Error(w, "target write: "+err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{
				"status":  "executed",
				"target":  t.Target.Path,
				"written": written,
			})
		}
	})

	handleGet(mux, "/api/data/transformations/transforms", func(w http.ResponseWriter, r *http.Request) {
		names := mapper.TransformNames()
		sort.Strings(names)
		writeJSON(w, names)
	})

	handlePost(mux, "/api/data/transformations/file-exists", func(w http.ResponseWriter, r *http.Request, req struct {
		Path string `json:"path"`
	}) {
		if req.Path == "" {
			writeJSON(w, map[string]bool{"exists": false})
			return
		}
		_, err := os.Stat(req.Path)
		writeJSON(w, map[string]bool{"exists": err == nil})
	})

	handlePost(mux, "/api/data/transformations/source-fields", func(w http.ResponseWriter, r *http.Request, req mapper.DataEndpoint) {
		if req.Type == "table" {
			if db == nil || req.Name == "" {
				http.Error(w, "table name required", http.StatusBadRequest)
				return
			}
			tbl, err := db.GetSchema(req.Name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if tbl != nil {
				writeJSON(w, tbl.Columns)
				return
			}
			cols, err := db.DescribeTable(req.Name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, cols)
			return
		}

		reader, err := mapper.NewSourceReader(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rows, err := reader.Read()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var fields []string
		if len(rows) > 0 {
			for k := range rows[0] {
				fields = append(fields, k)
			}
			sort.Strings(fields)
		}
		writeJSON(w, fields)
	})
}
