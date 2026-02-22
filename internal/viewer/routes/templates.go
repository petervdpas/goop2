
package routes

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"log"

	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/sitetemplates"
	"github.com/petervdpas/goop2/internal/storage"
	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/ui/viewmodels"
)

func registerTemplateRoutes(mux *http.ServeMux, d Deps, csrf string) {
	// GET /templates — show template gallery
	mux.HandleFunc("/templates", func(w http.ResponseWriter, r *http.Request) {
		if !requireLocal(w, r) {
			return
		}

		templates, _ := sitetemplates.List()

		// Fetch store templates from rendezvous servers (best-effort, 5s timeout).
		// The rendezvous server gates access — if registration is required and
		// the peer is not verified, it returns 403 with a human-readable message.
		var storeTemplates []rendezvous.StoreMeta
		var storePrices map[string]int
		var storeError string
		var peerID string
		if d.Node != nil {
			peerID = d.Node.ID()
		}
		var ownedTemplates map[string]bool
		if len(d.RVClients) > 0 {
			seen := map[string]bool{}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()

			for _, c := range d.RVClients {
				list, err := c.ListTemplates(ctx, peerID)
				if err != nil {
					if storeError == "" {
						storeError = err.Error()
					}
					continue
				}
				for _, m := range list {
					if !seen[m.Dir] {
						seen[m.Dir] = true
						storeTemplates = append(storeTemplates, m)
					}
				}
				// Fetch prices (best-effort, first successful response wins)
				if storePrices == nil {
					prices, _ := c.FetchPrices(ctx)
					if prices != nil {
						storePrices = prices
					}
				}
				// Fetch owned templates (best-effort)
				if ownedTemplates == nil {
					owned, _ := c.FetchOwnedTemplates(ctx, peerID)
					if owned != nil {
						ownedTemplates = owned
					}
				}
			}

			// Always show price badges on store templates.
			// If FetchPrices returned nil, default to empty map (all Free).
			if storePrices == nil && len(storeTemplates) > 0 {
				storePrices = map[string]int{}
			}
		}

		var activeTemplate string
		if cfg, err := config.LoadPartial(d.CfgPath); err == nil {
			activeTemplate = cfg.Viewer.ActiveTemplate
		}

		vm := viewmodels.TemplatesVM{
			BaseVM:              baseVM("Templates", "create", "page.templates", d),
			CSRF:                csrf,
			Templates:           templates,
			StoreTemplates:      storeTemplates,
			StoreTemplatePrices: storePrices,
			OwnedTemplates:      ownedTemplates,
			HasCredits:          storePrices != nil,
			StoreError:          storeError,
			ActiveTemplate:      activeTemplate,
		}
		render.Render(w, vm)
	})

	// POST /api/templates/apply — apply a built-in template (resets site + db)
	handlePost(mux, "/api/templates/apply", func(w http.ResponseWriter, r *http.Request, req struct {
		Template string `json:"template"`
		CSRF     string `json:"csrf"`
	}) {
		if !requireLocal(w, r) {
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

		// Get template files and metadata from embedded templates
		files, err := sitetemplates.SiteFiles(req.Template)
		if err != nil {
			http.Error(w, "template not found: "+err.Error(), http.StatusBadRequest)
			return
		}

		schema, _ := sitetemplates.Schema(req.Template)
		meta, _ := sitetemplates.GetMeta(req.Template)

		var tablePolicies map[string]string
		if len(meta.Tables) > 0 {
			tablePolicies = make(map[string]string)
			for name, tp := range meta.Tables {
				if tp.InsertPolicy != "" {
					tablePolicies[name] = tp.InsertPolicy
				}
			}
		}

		if err := applyTemplateFiles(d, files, schema, tablePolicies, meta.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Save active template to config
		if cfg, err := config.Load(d.CfgPath); err == nil {
			cfg.Viewer.ActiveTemplate = req.Template
			config.Save(d.CfgPath, cfg)
		}

		writeJSON(w, map[string]string{
			"status":   "applied",
			"template": req.Template,
		})
	})

	// POST /api/templates/apply-store — apply a store template (resets site + db)
	handlePost(mux, "/api/templates/apply-store", func(w http.ResponseWriter, r *http.Request, req struct {
		Template string `json:"template"`
		CSRF     string `json:"csrf"`
	}) {
		if !requireLocal(w, r) {
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

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		var peerID string
		if d.Node != nil {
			peerID = d.Node.ID()
		}

		// Spend credits (deduct + grant ownership) before downloading.
		// If the template is free or already owned, this is a no-op.
		var spendResult *rendezvous.SpendResult
		for _, c := range d.RVClients {
			sr, err := c.SpendCredits(ctx, req.Template, peerID)
			if err != nil {
				log.Printf("credits: spend failed for %q peer=%s: %v", req.Template, peerID, err)
				http.Error(w, err.Error(), http.StatusPaymentRequired)
				return
			}
			if sr != nil {
				log.Printf("credits: spent for %q — balance=%d owned=%v", req.Template, sr.Balance, sr.Owned)
			} else {
				log.Printf("credits: no credit service — skipping spend for %q", req.Template)
			}
			spendResult = sr
			break // only need to call once
		}

		// Download bundle from first rendezvous that has it
		var body io.ReadCloser
		var dlErr error
		for _, c := range d.RVClients {
			body, dlErr = c.DownloadTemplateBundle(ctx, req.Template, peerID)
			if dlErr == nil {
				break
			}
		}
		if dlErr != nil {
			http.Error(w, "failed to download template: "+dlErr.Error(), http.StatusBadGateway)
			return
		}
		defer body.Close()

		// Extract tar.gz into memory
		allFiles, err := extractTarGz(body)
		if err != nil {
			http.Error(w, "failed to extract template: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Separate site files, schema, and manifest
		var schema string
		var manifest rendezvous.StoreMeta
		siteFiles := make(map[string][]byte)

		for rel, data := range allFiles {
			switch rel {
			case "schema.sql":
				schema = string(data)
			case "manifest.json":
				json.Unmarshal(data, &manifest)
			default:
				siteFiles[rel] = data
			}
		}

		var tablePolicies map[string]string
		if len(manifest.Tables) > 0 {
			tablePolicies = make(map[string]string)
			for name, tp := range manifest.Tables {
				if tp.InsertPolicy != "" {
					tablePolicies[name] = tp.InsertPolicy
				}
			}
		}

		if err := applyTemplateFiles(d, siteFiles, schema, tablePolicies, manifest.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Save active template to config
		if cfg, err := config.Load(d.CfgPath); err == nil {
			cfg.Viewer.ActiveTemplate = req.Template
			config.Save(d.CfgPath, cfg)
		}

		resp := map[string]interface{}{
			"status":   "applied",
			"template": req.Template,
		}
		if spendResult != nil {
			resp["balance"] = spendResult.Balance
		}
		writeJSON(w, resp)
	})
}

// applyTemplateFiles runs the apply flow:
// 1. Drop all user tables
// 2. Clear site files (preserve lua/)
// 3. Write template site files
// 4. Execute schema.sql
// 5. Apply table insert policies
// 6. Ensure Lua engine rescans if Lua files are present
// 7. Auto-create a "template" group if any table uses "group" insert policy
func applyTemplateFiles(d Deps, files map[string][]byte, schema string, tablePolicies map[string]string, templateName string) error {
	// 1. Drop all user database tables
	if d.DB != nil {
		if err := dropAllTables(d.DB); err != nil {
			return fmt.Errorf("failed to clear database: %w", err)
		}
	}

	// 2. Clear site files (preserve lua/)
	if d.Content != nil {
		root := d.Content.RootAbs()
		if err := clearSitePreserveLua(root); err != nil {
			return fmt.Errorf("failed to clear site: %w", err)
		}
		if err := d.Content.EnsureRoot(); err != nil {
			return fmt.Errorf("failed to recreate site dir: %w", err)
		}
	}

	// 3. Write template site files
	if d.Content != nil {
		root := d.Content.RootAbs()
		for rel, data := range files {
			abs := filepath.Join(root, rel)
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return fmt.Errorf("failed to create dir: %w", err)
			}
			if err := os.WriteFile(abs, data, 0o644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}
		}
	}

	// 4. Run template schema SQL
	if d.DB != nil && schema != "" {
		if _, err := d.DB.Exec(schema); err != nil {
			return fmt.Errorf("failed to create tables: %w", err)
		}
		for _, name := range parseTableNames(schema) {
			d.DB.Exec("INSERT OR REPLACE INTO _tables (name, schema) VALUES (?, ?)", name, schema)
		}
	}

	// 5. Apply per-table insert policies
	if d.DB != nil && len(tablePolicies) > 0 {
		for tableName, policy := range tablePolicies {
			d.DB.SetTableInsertPolicy(tableName, policy)
		}
	}

	// 6. If the template includes Lua data functions, ensure the Lua engine
	//    is running and immediately rescan so scripts are available without
	//    waiting for the async fsnotify watcher.
	if d.EnsureLua != nil {
		for rel := range files {
			if strings.HasPrefix(rel, "lua/functions/") && strings.HasSuffix(rel, ".lua") {
				d.EnsureLua()
				break
			}
		}
	}

	// 7. Manage template co-author group lifecycle.
	if d.GroupManager != nil {
		// Always close any existing template groups — the new template may not use one.
		if existing, err := d.GroupManager.ListHostedGroups(); err == nil {
			for _, g := range existing {
				if g.AppType == "template" {
					_ = d.GroupManager.CloseGroup(g.ID)
				}
			}
		}

		// If the new template uses "group" policy, create a fresh co-author group.
		hasGroupPolicy := false
		for _, policy := range tablePolicies {
			if policy == "group" {
				hasGroupPolicy = true
				break
			}
		}
		if hasGroupPolicy {
			groupName := templateName + " Co-authors"
			if templateName == "" {
				groupName = "Co-authors"
			}
			groupID := generateGroupID()
			if err := d.GroupManager.CreateGroup(groupID, groupName, "template", 0, false); err != nil {
				log.Printf("template: failed to create co-author group: %v", err)
			} else {
				log.Printf("template: created co-author group %q (%s)", groupName, groupID)
				// Host auto-joins their own template group so they appear as a member.
				if err := d.GroupManager.JoinOwnGroup(groupID); err != nil {
					log.Printf("template: failed to auto-join co-author group: %v", err)
				}
			}
		}
	}

	return nil
}

// extractTarGz reads a tar.gz stream into a map of relative path → content.
// Strips the top-level directory prefix, rejects paths with "..",
// and enforces a 10MB per-file limit.
func extractTarGz(r io.Reader) (map[string][]byte, error) {
	const maxFileSize = 10 << 20 // 10MB

	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	files := make(map[string][]byte)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		// Strip top-level directory prefix (e.g. "blog/index.html" → "index.html")
		name := filepath.ToSlash(hdr.Name)
		if i := strings.IndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		if name == "" {
			continue
		}

		// Reject path traversal
		if strings.Contains(name, "..") {
			continue
		}

		if hdr.Size > maxFileSize {
			return nil, fmt.Errorf("file %q exceeds 10MB limit", name)
		}

		data, err := io.ReadAll(io.LimitReader(tr, maxFileSize+1))
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", name, err)
		}
		if int64(len(data)) > maxFileSize {
			return nil, fmt.Errorf("file %q exceeds 10MB limit", name)
		}

		files[name] = data
	}

	return files, nil
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

// clearSitePreserveLua removes all site files/directories except lua/.
// Chat scripts in lua/ survive template changes; templates write data
// functions to lua/functions/ which get recreated from the template.
func clearSitePreserveLua(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.Name() == "lua" {
			// Preserve lua/ root (chat scripts), but clear files inside
			// lua/functions/ so template data functions get a clean install.
			// We remove individual files rather than the directory itself to
			// preserve the fsnotify watch on the functions/ inode.
			fnDir := filepath.Join(root, "lua", "functions")
			fnEntries, err := os.ReadDir(fnDir)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			for _, fe := range fnEntries {
				if err := os.RemoveAll(filepath.Join(fnDir, fe.Name())); err != nil {
					return err
				}
			}
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
			return err
		}
	}
	return nil
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
