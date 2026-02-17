
package routes

import (
	"io"
	"net/http"
	"strings"
)

func registerSiteAPIRoutes(mux *http.ServeMux, d Deps) {
	// List site files as a flat tree
	handleGet(mux, "/api/site/files", func(w http.ResponseWriter, r *http.Request) {
		if d.Content == nil {
			http.Error(w, "content store not configured", http.StatusInternalServerError)
			return
		}

		tree, err := d.Content.ListTree(r.Context(), "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, tree)
	})

	// Upload a file to the site content store (multipart)
	handlePostAction(mux, "/api/site/upload", func(w http.ResponseWriter, r *http.Request) {
		if d.Content == nil {
			http.Error(w, "content store not configured", http.StatusInternalServerError)
			return
		}

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "request too large or bad form: "+err.Error(), http.StatusBadRequest)
			return
		}

		destPath := strings.TrimSpace(r.FormValue("path"))
		if destPath == "" {
			http.Error(w, "path required", http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file required: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "read error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		etag, err := d.Content.Write(r.Context(), destPath, data, "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(w, map[string]string{
			"status": "uploaded",
			"path":   destPath,
			"etag":   etag,
		})
	})

	// Delete a file from the site content store
	handlePost(mux, "/api/site/delete", func(w http.ResponseWriter, r *http.Request, req struct {
		Path string `json:"path"`
	}) {
		if d.Content == nil {
			http.Error(w, "content store not configured", http.StatusInternalServerError)
			return
		}
		if req.Path == "" {
			http.Error(w, "path required", http.StatusBadRequest)
			return
		}

		if err := d.Content.Delete(r.Context(), req.Path); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(w, map[string]string{
			"status": "deleted",
		})
	})
}
