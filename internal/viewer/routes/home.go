// internal/viewer/routes/home.go

package routes

import (
	"net/http"

	"goop/internal/viewer/render"
)

func registerHomeRoutes(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/peers", http.StatusFound)
	})

	mux.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {
		vm := render.PeersVM{
			BaseVM: baseVM("Goop", "peers", "page.peers", d),
			Peers:  render.BuildPeerRows(d.Peers.Snapshot()),
		}
		render.Render(w, vm)
	})
}
