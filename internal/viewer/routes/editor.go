package routes

import (
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/petervdpas/goop2/internal/content"
	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/ui/viewmodels"
)

func registerEditorRoutes(mux *http.ServeMux, d Deps, csrf string) {
	// /create → redirect to /templates
	mux.HandleFunc("/create", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/templates", http.StatusFound)
	})

	// GET /edit?path=...
	mux.HandleFunc("/edit", func(w http.ResponseWriter, r *http.Request) {
		if !requireContentStore(w, d.Content) {
			return
		}
		if !requireLocal(w, r) {
			return
		}

		rel := normalizeRel(r.URL.Query().Get("path")) // FILE path
		dir := dirOf(rel)

		b, etag, err := d.Content.Read(r.Context(), rel)
		if err == content.ErrNotFound {
			b = []byte("")
			etag = "none"
		} else if err != nil {
			vm := viewmodels.EditorVM{
				BaseVM:   baseVM("Editor", "editor", "page.editor", d),
				CSRF:     csrf,
				Path:     rel,
				Dir:      dir,
				SiteRoot: d.Content.RootAbs(),
				Error:    err.Error(),
			}
			render.Render(w, vm)
			return
		}

		tree, _ := d.Content.ListTree(r.Context(), "")
		list, _ := d.Content.List(r.Context(), dir)

		files := make([]viewmodels.EditorFileRow, 0, len(list))
		for _, it := range list {
			files = append(files, viewmodels.EditorFileRow{
				Path:  it.Path,
				IsDir: it.IsDir,
				Size:  it.Size,
				Mod:   it.Mod,
				ETag:  it.ETag,
			})
		}

		treeRows := make([]viewmodels.EditorTreeRow, 0, len(tree))
		for _, it := range tree {
			treeRows = append(treeRows, viewmodels.EditorTreeRow{
				Path:  it.Path,
				IsDir: it.IsDir,
				Depth: it.Depth,
			})
		}

		vm := viewmodels.EditorVM{
			BaseVM:   baseVM("Create", "create", "page.editor", d),
			CSRF:     csrf,
			Path:     rel,
			Dir:      dir,
			Files:    files,
			Tree:     treeRows,
			Content:  string(b),
			ETag:     etag,
			SiteRoot: d.Content.RootAbs(),
			IsImage:  isImageExt(rel),
			Saved:    (r.URL.Query().Get("saved") == "1"),
		}
		render.Render(w, vm)
	})

	// POST /edit/save
	mux.HandleFunc("/edit/save", func(w http.ResponseWriter, r *http.Request) {
		if !requireContentStore(w, d.Content) {
			return
		}
		if err := validatePOSTRequest(w, r, csrf); err != nil {
			return
		}

		rel := normalizeRel(getTrimmedPostFormValue(r.PostForm, "path")) // FILE path
		ifMatch := getTrimmedPostFormValue(r.PostForm, "if_match")
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
		if !requireContentStore(w, d.Content) {
			return
		}
		if err := validatePOSTRequest(w, r, csrf); err != nil {
			return
		}

		rawDir := getTrimmedPostFormValue(r.PostForm, "dir")
		dir, err := d.Content.NormalizeDir(r.Context(), rawDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		name := getTrimmedPostFormValue(r.PostForm, "name")

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
		if !requireContentStore(w, d.Content) {
			return
		}
		if err := validatePOSTRequest(w, r, csrf); err != nil {
			return
		}

		rawDir := getTrimmedPostFormValue(r.PostForm, "dir")
		dir, err := d.Content.NormalizeDir(r.Context(), rawDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		name := getTrimmedPostFormValue(r.PostForm, "name")
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

	// POST /edit/delete
	mux.HandleFunc("/edit/delete", func(w http.ResponseWriter, r *http.Request) {
		if !requireContentStore(w, d.Content) {
			return
		}
		if err := validatePOSTRequest(w, r, csrf); err != nil {
			return
		}

		p := getTrimmedPostFormValue(r.PostForm, "path")
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

	// POST /edit/rename
	mux.HandleFunc("/edit/rename", func(w http.ResponseWriter, r *http.Request) {
		if !requireContentStore(w, d.Content) {
			return
		}
		if err := validatePOSTRequest(w, r, csrf); err != nil {
			return
		}

		from := normalizeRel(getTrimmedPostFormValue(r.PostForm, "from"))
		to := normalizeRel(getTrimmedPostFormValue(r.PostForm, "to"))

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
