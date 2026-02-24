
package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/ui/viewmodels"
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
		selfVideoDisabled := false
		hideUnverified := false
		splash := "goop2-splash2.png"
		if d.CfgPath != "" {
			if cfg, err := config.Load(d.CfgPath); err == nil {
				selfVideoDisabled = cfg.Viewer.VideoDisabled
				hideUnverified = cfg.Viewer.HideUnverified
				if cfg.Viewer.Splash != "" {
					splash = cfg.Viewer.Splash
				}
			}
		}
		vm := viewmodels.PeersVM{
			BaseVM:            baseVM("Goop", "peers", "page.peers", d),
			Peers:             viewmodels.BuildPeerRows(d.Peers.Snapshot()),
			SelfVideoDisabled: selfVideoDisabled,
			HideUnverified:    hideUnverified,
			Splash:            splash,
		}
		render.Render(w, vm)
	})

	// Probe all peers synchronously and return the updated list.
	handlePostAction(mux, "/api/peers/probe", func(w http.ResponseWriter, r *http.Request) {
		d.Node.ProbeAllPeers(r.Context())
		writeJSON(w, viewmodels.BuildPeerRows(d.Peers.Snapshot()))
	})

	// Toggle favorite status for a peer
	handlePost(mux, "/api/peers/favorite", func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		peerID, _ := body["peer_id"].(string)
		fav, _ := body["favorite"].(bool)
		if peerID == "" {
			http.Error(w, "peer_id required", http.StatusBadRequest)
			return
		}
		if err := d.DB.SetFavorite(peerID, fav); err != nil {
			http.Error(w, "failed to update", http.StatusInternalServerError)
			return
		}
		d.Peers.SetFavorite(peerID, fav)
		w.WriteHeader(http.StatusOK)
	})

	// JSON endpoint for peers list
	handleGet(mux, "/api/peers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, viewmodels.BuildPeerRows(d.Peers.Snapshot()))
	})

	// JSON endpoint for self identity
	handleGet(mux, "/api/self", func(w http.ResponseWriter, r *http.Request) {
		avatarHash := ""
		if d.AvatarStore != nil {
			avatarHash = d.AvatarStore.Hash()
		}
		writeJSON(w, map[string]string{
			"id":          d.Node.ID(),
			"label":       safeCall(d.SelfLabel),
			"email":       safeCall(d.SelfEmail),
			"avatar_hash": avatarHash,
		})
	})
}
