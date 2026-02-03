// internal/viewer/routes/export.go

package routes

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"goop/internal/config"
)

// ExportManifest is written as manifest.json inside the export zip.
type ExportManifest struct {
	Version    int                          `json:"version"`
	Label      string                       `json:"label"`
	ExportedAt string                       `json:"exported_at"`
	LuaEnabled bool                         `json:"lua_enabled"`
	Tables     map[string]TablePolicyExport `json:"tables,omitempty"`
}

// TablePolicyExport holds per-table policy in the export manifest.
type TablePolicyExport struct {
	InsertPolicy string `json:"insert_policy"`
}

func registerExportRoutes(mux *http.ServeMux, d Deps, csrf string) {
	// GET /api/site/export — download site as zip
	mux.HandleFunc("/api/site/export", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// Load config to get lua settings
		var cfg config.Config
		if d.CfgPath != "" {
			var err error
			cfg, err = config.Load(d.CfgPath)
			if err != nil {
				http.Error(w, "failed to load config: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		label := "site"
		if d.SelfLabel != nil {
			if l := d.SelfLabel(); l != "" {
				label = l
			}
		}

		now := time.Now().UTC()

		// Build manifest
		manifest := ExportManifest{
			Version:    1,
			Label:      label,
			ExportedAt: now.Format(time.RFC3339),
			LuaEnabled: cfg.Lua.Enabled,
		}

		if d.DB != nil {
			tables, err := d.DB.ListTables()
			if err == nil && len(tables) > 0 {
				manifest.Tables = make(map[string]TablePolicyExport)
				for _, t := range tables {
					manifest.Tables[t.Name] = TablePolicyExport{InsertPolicy: t.InsertPolicy}
				}
			}
		}

		manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			http.Error(w, "failed to build manifest: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Generate schema.sql
		var schemaSQL string
		if d.DB != nil {
			schemaSQL, err = d.DB.DumpSQL()
			if err != nil {
				http.Error(w, "failed to dump database: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		// Collect site files
		siteFiles := make(map[string][]byte)
		if d.Content != nil {
			ctx := r.Context()
			items, err := d.Content.ListTree(ctx, "")
			if err == nil {
				for _, item := range items {
					if item.IsDir {
						continue
					}
					data, _, readErr := d.Content.Read(ctx, item.Path)
					if readErr == nil {
						siteFiles[item.Path] = data
					}
				}
			}
		}

		// Collect lua scripts
		luaFiles := make(map[string][]byte)
		if cfg.Lua.ScriptDir != "" && d.PeerDir != "" {
			scriptDir := cfg.Lua.ScriptDir
			if !filepath.IsAbs(scriptDir) {
				scriptDir = filepath.Join(d.PeerDir, scriptDir)
			}
			luaFiles, _ = collectLuaScripts(scriptDir)
		}

		// Write zip to buffer
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)

		// manifest.json
		if fw, err := zw.Create("manifest.json"); err == nil {
			fw.Write(manifestJSON)
		}

		// schema.sql
		if fw, err := zw.Create("schema.sql"); err == nil {
			fw.Write([]byte(schemaSQL))
		}

		// site/ files
		for rel, data := range siteFiles {
			if fw, err := zw.Create("site/" + rel); err == nil {
				fw.Write(data)
			}
		}

		// lua/ files
		for rel, data := range luaFiles {
			if fw, err := zw.Create("lua/" + rel); err == nil {
				fw.Write(data)
			}
		}

		if err := zw.Close(); err != nil {
			http.Error(w, "failed to create zip: "+err.Error(), http.StatusInternalServerError)
			return
		}

		filename := fmt.Sprintf("goop-export-%s-%s.zip", label, now.Format("2006-01-02"))
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Write(buf.Bytes())
	})

	// POST /api/site/import — upload and apply zip
	mux.HandleFunc("/api/site/import", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if err := r.ParseMultipartForm(50 << 20); err != nil {
			http.Error(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
			return
		}

		// CSRF check
		if r.FormValue("csrf") != csrf {
			http.Error(w, "bad csrf", http.StatusForbidden)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		data, err := io.ReadAll(io.LimitReader(file, 50<<20+1))
		if err != nil {
			http.Error(w, "failed to read file", http.StatusInternalServerError)
			return
		}
		if len(data) > 50<<20 {
			http.Error(w, "file exceeds 50MB limit", http.StatusBadRequest)
			return
		}

		// Extract zip
		allFiles, err := extractZip(data)
		if err != nil {
			http.Error(w, "failed to extract zip: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Separate manifest, schema, site files, lua files
		var manifest ExportManifest
		var schema string
		siteFiles := make(map[string][]byte)
		luaFiles := make(map[string][]byte)

		for rel, content := range allFiles {
			switch {
			case rel == "manifest.json":
				json.Unmarshal(content, &manifest)
			case rel == "schema.sql":
				schema = string(content)
			case strings.HasPrefix(rel, "site/"):
				siteFiles[strings.TrimPrefix(rel, "site/")] = content
			case strings.HasPrefix(rel, "lua/"):
				luaFiles[strings.TrimPrefix(rel, "lua/")] = content
			}
		}

		// Build table policies from manifest
		var tablePolicies map[string]string
		if len(manifest.Tables) > 0 {
			tablePolicies = make(map[string]string)
			for name, tp := range manifest.Tables {
				if tp.InsertPolicy != "" {
					tablePolicies[name] = tp.InsertPolicy
				}
			}
		}

		// Reuse the existing template apply flow for site files + schema + policies
		if err := applyTemplateFiles(d, siteFiles, schema, tablePolicies); err != nil {
			http.Error(w, "failed to apply import: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Handle lua scripts if any present in archive
		if len(luaFiles) > 0 {
			var cfg config.Config
			if d.CfgPath != "" {
				cfg, _ = config.Load(d.CfgPath)
			}
			if cfg.Lua.ScriptDir != "" && d.PeerDir != "" {
				scriptDir := cfg.Lua.ScriptDir
				if !filepath.IsAbs(scriptDir) {
					scriptDir = filepath.Join(d.PeerDir, scriptDir)
				}
				if err := writeLuaScripts(scriptDir, luaFiles); err != nil {
					http.Error(w, "failed to write lua scripts: "+err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "imported"})
	})
}

// extractZip reads a zip from a byte slice into a map of relative path → content.
// Rejects paths with "..", enforces a 10MB per-file limit, and strips an optional
// wrapper directory (if all entries share a common top-level prefix).
func extractZip(data []byte) (map[string][]byte, error) {
	const maxFileSize = 10 << 20 // 10MB

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("zip: %w", err)
	}

	// Detect optional wrapper dir: if every file starts with "prefix/",
	// strip that prefix.
	prefix := ""
	for i, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) < 2 {
			prefix = ""
			break
		}
		if i == 0 || prefix == "" {
			prefix = parts[0] + "/"
		} else if !strings.HasPrefix(f.Name, prefix) {
			prefix = ""
			break
		}
	}

	files := make(map[string][]byte)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}

		name := f.Name
		if prefix != "" {
			name = strings.TrimPrefix(name, prefix)
		}
		if name == "" {
			continue
		}

		// Reject path traversal
		if strings.Contains(name, "..") {
			continue
		}

		if f.UncompressedSize64 > maxFileSize {
			return nil, fmt.Errorf("file %q exceeds 10MB limit", name)
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %q: %w", name, err)
		}
		content, err := io.ReadAll(io.LimitReader(rc, maxFileSize+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", name, err)
		}
		if int64(len(content)) > maxFileSize {
			return nil, fmt.Errorf("file %q exceeds 10MB limit", name)
		}

		files[name] = content
	}

	return files, nil
}

// collectLuaScripts walks the lua script directory, collects all files,
// and skips the .state/ subdirectory.
func collectLuaScripts(scriptDir string) (map[string][]byte, error) {
	files := make(map[string][]byte)

	info, err := os.Stat(scriptDir)
	if err != nil || !info.IsDir() {
		return files, nil
	}

	err = filepath.WalkDir(scriptDir, func(path string, de os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(scriptDir, path)
		rel = filepath.ToSlash(rel)

		// Skip .state/ directory
		if de.IsDir() && (de.Name() == ".state" || strings.HasPrefix(rel, ".state")) {
			return filepath.SkipDir
		}

		if de.IsDir() {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = data
		return nil
	})

	return files, err
}

// writeLuaScripts clears existing lua scripts (not .state/) and writes
// new files from the archive.
func writeLuaScripts(scriptDir string, files map[string][]byte) error {
	// Ensure the script directory exists
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return err
	}

	// Clear existing scripts, preserving .state/
	entries, err := os.ReadDir(scriptDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range entries {
		if e.Name() == ".state" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(scriptDir, e.Name())); err != nil {
			return err
		}
	}

	// Write new files
	for rel, data := range files {
		// Safety: reject path traversal
		if strings.Contains(rel, "..") {
			continue
		}
		abs := filepath.Join(scriptDir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, data, 0o644); err != nil {
			return err
		}
	}

	return nil
}
