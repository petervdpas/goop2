// internal/viewer/routes/site_api.go

package routes

import (
	"encoding/json"
	"net/http"
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
}
