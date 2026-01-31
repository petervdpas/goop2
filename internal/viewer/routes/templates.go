// internal/viewer/routes/templates.go

package routes

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"goop/internal/sitetemplates"
	"goop/internal/storage"
	"goop/internal/ui/render"
	"goop/internal/ui/viewmodels"
)

func registerTemplateRoutes(mux *http.ServeMux, d Deps, csrf string) {
	// GET /templates — show template gallery
	mux.HandleFunc("/templates", func(w http.ResponseWriter, r *http.Request) {
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		templates, _ := sitetemplates.List()

		vm := viewmodels.TemplatesVM{
			BaseVM:    baseVM("Templates", "create", "page.templates", d),
			CSRF:      csrf,
			Templates: templates,
		}
		render.Render(w, vm)
	})

	// POST /api/templates/apply — apply a template (resets site + db)
	mux.HandleFunc("/api/templates/apply", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var req struct {
			Template string `json:"template"`
			CSRF     string `json:"csrf"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.CSRF != csrf {
			http.Error(w, "bad csrf", http.StatusForbidden)
			return
		}
		if req.Template == "" {
			http.Error(w, "template name required", http.StatusBadRequest)
			return
		}

		// 1. Drop all user database tables
		if d.DB != nil {
			if err := dropAllTables(d.DB); err != nil {
				http.Error(w, "failed to clear database: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		// 2. Clear all site files
		if d.Content != nil {
			root := d.Content.RootAbs()
			if err := os.RemoveAll(root); err != nil {
				http.Error(w, "failed to clear site: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if err := d.Content.EnsureRoot(); err != nil {
				http.Error(w, "failed to recreate site dir: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		// 3. Write template site files
		files, err := sitetemplates.SiteFiles(req.Template)
		if err != nil {
			http.Error(w, "template not found: "+err.Error(), http.StatusBadRequest)
			return
		}

		if d.Content != nil {
			root := d.Content.RootAbs()
			for rel, data := range files {
				abs := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
					http.Error(w, "failed to create dir: "+err.Error(), http.StatusInternalServerError)
					return
				}
				if err := os.WriteFile(abs, data, 0o644); err != nil {
					http.Error(w, "failed to write file: "+err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}

		// 4. Run template schema SQL
		if d.DB != nil {
			schema, err := sitetemplates.Schema(req.Template)
			if err == nil && schema != "" {
				if _, err := d.DB.Exec(schema); err != nil {
					http.Error(w, "failed to create tables: "+err.Error(), http.StatusInternalServerError)
					return
				}
				// Register each table found in the schema
				for _, name := range parseTableNames(schema) {
					d.DB.Exec("INSERT OR REPLACE INTO _tables (name, schema) VALUES (?, ?)", name, schema)
				}
			}

			// 5. Apply per-table insert policies from manifest
			meta, err := sitetemplates.GetMeta(req.Template)
			if err == nil && len(meta.Tables) > 0 {
				for tableName, tp := range meta.Tables {
					if tp.InsertPolicy != "" {
						d.DB.SetTableInsertPolicy(tableName, tp.InsertPolicy)
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "applied",
			"template": req.Template,
		})
	})
}

var reCreateTable = regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(\w+)`)

func parseTableNames(schema string) []string {
	var names []string
	for _, m := range reCreateTable.FindAllStringSubmatch(schema, -1) {
		name := strings.ToLower(m[1])
		if !strings.HasPrefix(name, "_") {
			names = append(names, name)
		}
	}
	return names
}

func dropAllTables(db *storage.DB) error {
	tables, err := db.ListTables()
	if err != nil {
		return err
	}
	for _, t := range tables {
		if err := db.DeleteTable(t.Name); err != nil {
			return err
		}
	}
	return nil
}
