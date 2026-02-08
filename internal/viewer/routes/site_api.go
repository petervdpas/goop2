
package routes

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func registerSiteAPIRoutes(mux *http.ServeMux, d Deps) {
	// List site files as a flat tree
	mux.HandleFunc("/api/site/files", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.Content == nil {
			http.Error(w, "content store not configured", http.StatusInternalServerError)
			return
		}

		tree, err := d.Content.ListTree(r.Context(), "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tree)
	})

	// Upload a file to the site content store (multipart)
	mux.HandleFunc("/api/site/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "uploaded",
			"path":   destPath,
			"etag":   etag,
		})
	})

	// Delete a file from the site content store
	mux.HandleFunc("/api/site/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.Content == nil {
			http.Error(w, "content store not configured", http.StatusInternalServerError)
			return
		}

		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "deleted",
		})
	})
}
