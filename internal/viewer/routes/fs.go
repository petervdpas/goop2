package routes

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func RegisterFS(mux *http.ServeMux) {
	handleGet(mux, "/api/fs/browse", func(w http.ResponseWriter, r *http.Request) {
		dir := r.URL.Query().Get("dir")
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				dir = "/"
			} else {
				dir = home
			}
		}

		dir = filepath.Clean(dir)

		entries, err := os.ReadDir(dir)
		if err != nil {
			http.Error(w, "cannot read directory", http.StatusBadRequest)
			return
		}

		type entry struct {
			Name  string `json:"name"`
			IsDir bool   `json:"is_dir"`
			Size  int64  `json:"size,omitempty"`
		}

		var dirs, files []entry
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			ent := entry{Name: e.Name(), IsDir: e.IsDir()}
			if !e.IsDir() {
				ent.Size = info.Size()
			}
			if e.IsDir() {
				dirs = append(dirs, ent)
			} else {
				files = append(files, ent)
			}
		}

		sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
		sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

		writeJSON(w, map[string]any{
			"dir":     dir,
			"parent":  filepath.Dir(dir),
			"entries": append(dirs, files...),
		})
	})
}
