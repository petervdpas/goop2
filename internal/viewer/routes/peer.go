// internal/viewer/routes/peer.go

package routes

import (
	"context"
	"net/http"
	"strings"

	"goop/internal/ui/render"
	"goop/internal/ui/viewmodels"
	"goop/internal/util"
)

func registerPeerRoutes(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/peer/", func(w http.ResponseWriter, r *http.Request) {
		peerID := strings.TrimPrefix(r.URL.Path, "/peer/")
		if peerID == "" {
			http.NotFound(w, r)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), util.DefaultFetchTimeout)
		defer cancel()

		txt, err := d.Node.FetchContent(ctx, peerID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		vm := viewmodels.PeerContentVM{
			BaseVM:  baseVM("Peer", "peers", "page.peer", d),
			PeerID:  peerID,
			Content: txt,
		}
		render.Render(w, vm)
	})
}
