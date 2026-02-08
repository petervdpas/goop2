
package routes

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/luaprefabs"
	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/ui/viewmodels"
	"github.com/petervdpas/goop2/internal/util"
)

var validScriptName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// resolveTargetDir returns the script or functions directory based on isFunc.
func resolveTargetDir(d Deps, isFunc bool) (string, error) {
	cfg, err := config.Load(d.CfgPath)
	if err != nil {
		return "", err
	}
	dir := util.ResolvePath(d.PeerDir, cfg.Lua.ScriptDir)
	if isFunc {
		dir = filepath.Join(dir, "functions")
	}
	return dir, nil
}

func registerLuaRoutes(mux *http.ServeMux, d Deps, csrf string) {

	// GET /lua — show script list + prefab gallery + editor
	mux.HandleFunc("/lua", func(w http.ResponseWriter, r *http.Request) {
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		cfg, err := config.Load(d.CfgPath)
		if err != nil {
			http.Error(w, "failed to load config: "+err.Error(), http.StatusInternalServerError)
			return
		}

		scriptDir := util.ResolvePath(d.PeerDir, cfg.Lua.ScriptDir)
		functionsDir := filepath.Join(scriptDir, "functions")
		scripts := listScripts(scriptDir)
		functions := listScripts(functionsDir)

		// Build set of installed script names for quick lookup (chat scripts only)
		installed := make(map[string]bool, len(scripts))
		for _, s := range scripts {
			installed[s.Name] = true
		}

		// Compute per-script prefab install status
		rawPrefabs, _ := luaprefabs.List()
		prefabs := make([]viewmodels.PrefabStatus, len(rawPrefabs))
		for i, p := range rawPrefabs {
			ss := make([]viewmodels.PrefabScriptStatus, len(p.ScriptNames))
			allInstalled := true
			for j, sn := range p.ScriptNames {
				have := installed[sn]
				ss[j] = viewmodels.PrefabScriptStatus{Name: sn, Installed: have}
				if !have {
					allInstalled = false
				}
			}
			prefabs[i] = viewmodels.PrefabStatus{
				Name:         p.Name,
				Description:  p.Description,
				Icon:         p.Icon,
				Dir:          p.Dir,
				Scripts:      ss,
				AllInstalled: allInstalled,
			}
		}

		vm := viewmodels.LuaVM{
			BaseVM:     baseVM("Lua Scripts", "create", "page.lua", d),
			CSRF:       csrf,
			Scripts:    scripts,
			Functions:  functions,
			Prefabs:    prefabs,
			ScriptDir:  cfg.Lua.ScriptDir,
			LuaEnabled: cfg.Lua.Enabled,
		}

		// Check if editing a specific script
		editName := r.URL.Query().Get("edit")
		isFunc := r.URL.Query().Get("func") == "1"
		if editName != "" {
			dir := scriptDir
			if isFunc {
				dir = functionsDir
			}
			content, err := os.ReadFile(filepath.Join(dir, editName+".lua"))
			if err == nil {
				vm.EditName = editName
				vm.EditIsFunc = isFunc
				vm.Content = string(content)
			}
		}

		vm.Saved = r.URL.Query().Get("saved") == "1"

		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			vm.Error = errMsg
		}

		render.Render(w, vm)
	})

	// POST /lua/save — save a script
	mux.HandleFunc("/lua/save", func(w http.ResponseWriter, r *http.Request) {
		if err := validatePOSTRequest(w, r, csrf); err != nil {
			return
		}

		name := getTrimmedPostFormValue(r.PostForm, "name")
		content := r.PostForm.Get("content")
		isFunc := r.PostForm.Get("is_func") == "1"

		if !validScriptName.MatchString(name) {
			http.Redirect(w, r, "/lua?error=Invalid+script+name", http.StatusFound)
			return
		}

		dir, err := resolveTargetDir(d, isFunc)
		if err != nil {
			http.Redirect(w, r, "/lua?error="+err.Error(), http.StatusFound)
			return
		}

		if err := os.MkdirAll(dir, 0o755); err != nil {
			http.Redirect(w, r, "/lua?error="+err.Error(), http.StatusFound)
			return
		}

		funcParam := ""
		if isFunc {
			funcParam = "&func=1"
		}

		path := filepath.Join(dir, name+".lua")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			http.Redirect(w, r, "/lua?edit="+name+funcParam+"&error="+err.Error(), http.StatusFound)
			return
		}

		http.Redirect(w, r, "/lua?edit="+name+"&saved=1"+funcParam, http.StatusFound)
	})

	// POST /lua/new — create a new empty script
	mux.HandleFunc("/lua/new", func(w http.ResponseWriter, r *http.Request) {
		if err := validatePOSTRequest(w, r, csrf); err != nil {
			return
		}

		name := getTrimmedPostFormValue(r.PostForm, "name")
		isFunc := r.PostForm.Get("is_func") == "1"

		if !validScriptName.MatchString(name) {
			http.Redirect(w, r, "/lua?error=Invalid+script+name.+Use+letters,+numbers,+hyphens,+underscores.", http.StatusFound)
			return
		}

		dir, err := resolveTargetDir(d, isFunc)
		if err != nil {
			http.Redirect(w, r, "/lua?error="+err.Error(), http.StatusFound)
			return
		}

		if err := os.MkdirAll(dir, 0o755); err != nil {
			http.Redirect(w, r, "/lua?error="+err.Error(), http.StatusFound)
			return
		}

		funcParam := ""
		if isFunc {
			funcParam = "&func=1"
		}

		path := filepath.Join(dir, name+".lua")
		if _, err := os.Stat(path); err == nil {
			http.Redirect(w, r, "/lua?edit="+name+funcParam, http.StatusFound)
			return
		}

		var stub string
		if isFunc {
			stub = "--- " + name + "\nfunction call(request)\n    return { message = \"hello from " + name + "\" }\nend\n"
		} else {
			stub = "-- " + name + ".lua\nfunction handle(args)\n    return \"hello from !" + name + "\"\nend\n"
		}
		if err := os.WriteFile(path, []byte(stub), 0o644); err != nil {
			http.Redirect(w, r, "/lua?error="+err.Error(), http.StatusFound)
			return
		}

		http.Redirect(w, r, "/lua?edit="+name+funcParam, http.StatusFound)
	})

	// POST /lua/delete — delete a script
	mux.HandleFunc("/lua/delete", func(w http.ResponseWriter, r *http.Request) {
		if err := validatePOSTRequest(w, r, csrf); err != nil {
			return
		}

		name := getTrimmedPostFormValue(r.PostForm, "name")
		isFunc := r.PostForm.Get("is_func") == "1"

		if !validScriptName.MatchString(name) {
			http.Redirect(w, r, "/lua?error=Invalid+script+name", http.StatusFound)
			return
		}

		dir, err := resolveTargetDir(d, isFunc)
		if err != nil {
			http.Redirect(w, r, "/lua?error="+err.Error(), http.StatusFound)
			return
		}

		path := filepath.Join(dir, name+".lua")
		_ = os.Remove(path)

		http.Redirect(w, r, "/lua", http.StatusFound)
	})

	// POST /api/lua/prefabs/apply — install prefab scripts
	mux.HandleFunc("/api/lua/prefabs/apply", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var req struct {
			Prefab string `json:"prefab"`
			Script string `json:"script"` // optional: install single script (name without .lua)
			CSRF   string `json:"csrf"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.CSRF != csrf {
			http.Error(w, "bad csrf", http.StatusForbidden)
			return
		}
		if req.Prefab == "" {
			http.Error(w, "prefab name required", http.StatusBadRequest)
			return
		}

		scripts, err := luaprefabs.Scripts(req.Prefab)
		if err != nil {
			http.Error(w, "prefab not found: "+err.Error(), http.StatusBadRequest)
			return
		}

		// If a specific script is requested, filter to just that one
		if req.Script != "" {
			target := req.Script + ".lua"
			data, ok := scripts[target]
			if !ok {
				http.Error(w, "script not found in prefab", http.StatusBadRequest)
				return
			}
			scripts = map[string][]byte{target: data}
		}

		// Prefabs install to the chat scripts directory
		scriptDir, err := resolveTargetDir(d, false)
		if err != nil {
			http.Error(w, "failed to resolve script dir: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := os.MkdirAll(scriptDir, 0o755); err != nil {
			http.Error(w, "failed to create script dir: "+err.Error(), http.StatusInternalServerError)
			return
		}

		for name, data := range scripts {
			abs := filepath.Join(scriptDir, name)
			if err := os.WriteFile(abs, data, 0o644); err != nil {
				http.Error(w, "failed to write "+name+": "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "installed",
			"prefab": req.Prefab,
		})
	})
}

func listScripts(dir string) []viewmodels.LuaScript {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var scripts []viewmodels.LuaScript
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lua") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".lua")
		scripts = append(scripts, viewmodels.LuaScript{
			Name: name,
			Size: info.Size(),
		})
	}
	return scripts
}
