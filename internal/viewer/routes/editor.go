// internal/viewer/routes/editor.go
package routes

import (
	"net/http"
	"net/url"
	"path"
	"strings"

	"goop/internal/content"
	"goop/internal/ui/render"
)

func registerEditorRoutes(mux *http.ServeMux, d Deps, csrf string) {
	// GET /edit?path=...
	mux.HandleFunc("/edit", func(w http.ResponseWriter, r *http.Request) {
		if d.Content == nil {
			http.Error(w, "content store not configured", http.StatusInternalServerError)
			return
		}
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		rel := normalizeRel(r.URL.Query().Get("path")) // FILE path
		dir := dirOf(rel)

		b, etag, err := d.Content.Read(r.Context(), rel)
		if err == content.ErrNotFound {
			b = []byte("")
			etag = "none"
		} else if err != nil {
			vm := render.EditorVM{
				BaseVM: baseVM("Editor", "editor", "page.editor", d),
				CSRF:   csrf,
				Path:   rel,
				Dir:    dir,
				Error:  err.Error(),
			}
			render.Render(w, vm)
			return
		}

		tree, _ := d.Content.ListTree(r.Context(), "")
		list, _ := d.Content.List(r.Context(), dir)

		files := make([]render.EditorFileRow, 0, len(list))
		for _, it := range list {
			files = append(files, render.EditorFileRow{
				Path:  it.Path,
				IsDir: it.IsDir,
				Size:  it.Size,
				Mod:   it.Mod,
				ETag:  it.ETag,
			})
		}

		treeRows := make([]render.EditorTreeRow, 0, len(tree))
		for _, it := range tree {
			treeRows = append(treeRows, render.EditorTreeRow{
				Path:  it.Path,
				IsDir: it.IsDir,
				Depth: it.Depth,
			})
		}

		vm := render.EditorVM{
			BaseVM:  baseVM("Editor", "editor", "page.editor", d),
			CSRF:    csrf,
			Path:    rel,
			Dir:     dir,
			Files:   files,
			Tree:    treeRows,
			Content: string(b),
			ETag:    etag,
			Saved:   (r.URL.Query().Get("saved") == "1"),
		}
		render.Render(w, vm)
	})

	// POST /edit/save
	mux.HandleFunc("/edit/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.Content == nil {
			http.Error(w, "content store not configured", http.StatusInternalServerError)
			return
		}
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("csrf") != csrf {
			http.Error(w, "bad csrf", http.StatusForbidden)
			return
		}

		rel := normalizeRel(r.PostForm.Get("path")) // FILE path
		ifMatch := strings.TrimSpace(r.PostForm.Get("if_match"))
		body := r.PostForm.Get("content")

		_, err := d.Content.Write(r.Context(), rel, []byte(body), ifMatch)
		if err == content.ErrConflict {
			http.Error(w, "conflict: reload and try again", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		http.Redirect(w, r, "/edit?path="+url.QueryEscape(rel)+"&saved=1", http.StatusFound)
	})

	// POST /edit/mkdir
	mux.HandleFunc("/edit/mkdir", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.Content == nil {
			http.Error(w, "content store not configured", http.StatusInternalServerError)
			return
		}
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("csrf") != csrf {
			http.Error(w, "bad csrf", http.StatusForbidden)
			return
		}

		rawDir := r.PostForm.Get("dir")
		dir, err := d.Content.NormalizeDir(r.Context(), rawDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(r.PostForm.Get("name"))

		createdDir, err := d.Content.MkdirUnder(r.Context(), dir, name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		jump := normalizeRel(path.Join(createdDir, "index.html"))
		http.Redirect(w, r, "/edit?path="+url.QueryEscape(jump), http.StatusFound)
	})

	// POST /edit/new
	mux.HandleFunc("/edit/new", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.Content == nil {
			http.Error(w, "content store not configured", http.StatusInternalServerError)
			return
		}
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("csrf") != csrf {
			http.Error(w, "bad csrf", http.StatusForbidden)
			return
		}

		rawDir := r.PostForm.Get("dir")
		dir, err := d.Content.NormalizeDir(r.Context(), rawDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(r.PostForm.Get("name"))
		name = strings.TrimPrefix(name, "/")
		name = strings.TrimSuffix(name, "/")
		if name == "" || name == "." || name == ".." {
			http.Error(w, "bad filename", http.StatusBadRequest)
			return
		}
		if strings.Contains(name, "/") || strings.Contains(name, `\`) {
			http.Error(w, "filename must not contain slashes", http.StatusBadRequest)
			return
		}

		target := normalizeRel(path.Join(dir, name))
		if target == "" {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}

		if _, err := d.Content.Write(r.Context(), target, []byte(""), "none"); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/edit?path="+url.QueryEscape(target), http.StatusFound)
	})

	// POST /edit/delete (unchanged)
	mux.HandleFunc("/edit/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.Content == nil {
			http.Error(w, "content store not configured", http.StatusInternalServerError)
			return
		}
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("csrf") != csrf {
			http.Error(w, "bad csrf", http.StatusForbidden)
			return
		}

		p := strings.TrimSpace(r.PostForm.Get("path"))
		p = strings.TrimPrefix(p, "/")
		p = strings.ReplaceAll(p, `\`, "/")
		p = path.Clean(p)
		if p == "." || p == "" {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}

		recursive := (r.PostForm.Get("recursive") == "1")

		if err := d.Content.DeletePath(r.Context(), p, recursive); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		parent := dirOf(p)
		open := "index.html"
		if parent != "" {
			open = normalizeRel(parent + "/index.html")
		}
		http.Redirect(w, r, "/edit?path="+url.QueryEscape(open), http.StatusFound)
	})

	// POST /edit/rename (unchanged)
	mux.HandleFunc("/edit/rename", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.Content == nil {
			http.Error(w, "content store not configured", http.StatusInternalServerError)
			return
		}
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("csrf") != csrf {
			http.Error(w, "bad csrf", http.StatusForbidden)
			return
		}

		from := normalizeRel(r.PostForm.Get("from"))
		to := normalizeRel(r.PostForm.Get("to"))

		if from == "" || to == "" {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}

		if err := d.Content.Rename(r.Context(), from, to); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/edit?path="+url.QueryEscape(to), http.StatusFound)
	})
}
