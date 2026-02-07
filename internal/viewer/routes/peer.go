// internal/viewer/routes/peer.go

package routes

import (
	"context"
	"net/http"
	"strings"

	"goop/internal/config"
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

		peerEmail := ""
		avatarHash := ""
		videoDisabled := false
		if d.Peers != nil {
			if sp, ok := d.Peers.Get(peerID); ok {
				peerEmail = sp.Email
				avatarHash = sp.AvatarHash
				videoDisabled = sp.VideoDisabled
			}
		}

		selfVideoDisabled := false
		if d.CfgPath != "" {
			if cfg, err := config.Load(d.CfgPath); err == nil {
				selfVideoDisabled = cfg.Viewer.VideoDisabled
			}
		}

		vm := viewmodels.PeerContentVM{
			BaseVM:            baseVM("Peer", "peers", "page.peer", d),
			PeerID:            peerID,
			Content:           txt,
			PeerEmail:         peerEmail,
			AvatarHash:        avatarHash,
			VideoDisabled:     videoDisabled,
			SelfVideoDisabled: selfVideoDisabled,
		}
		render.Render(w, vm)
	})
}
