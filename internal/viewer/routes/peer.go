// internal/viewer/routes/peer.go

package routes

import (
	"context"
	"net/http"
	"strings"
	"time"

	"goop/internal/viewer/render"
)

func registerPeerRoutes(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/peer/", func(w http.ResponseWriter, r *http.Request) {
		peerID := strings.TrimPrefix(r.URL.Path, "/peer/")
		if peerID == "" {
			http.NotFound(w, r)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		txt, err := d.Node.FetchContent(ctx, peerID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		vm := render.PeerContentVM{
			BaseVM:  baseVM("Peer", "peers", "page.peer", d),
			PeerID:  peerID,
			Content: txt,
		}
		render.Render(w, vm)
	})
}
