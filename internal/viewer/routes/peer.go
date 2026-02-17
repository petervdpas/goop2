
package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/ui/viewmodels"
	"github.com/petervdpas/goop2/internal/util"
)

func registerPeerRoutes(mux *http.ServeMux, d Deps) {
	// Page route - renders immediately with cached local data
	mux.HandleFunc("/peer/", func(w http.ResponseWriter, r *http.Request) {
		peerID := strings.TrimPrefix(r.URL.Path, "/peer/")
		if peerID == "" {
			http.NotFound(w, r)
			return
		}

		peerEmail := ""
		avatarHash := ""
		videoDisabled := false
		reachable := true
		if d.Peers != nil {
			if sp, ok := d.Peers.Get(peerID); ok {
				peerEmail = sp.Email
				avatarHash = sp.AvatarHash
				videoDisabled = sp.VideoDisabled
				reachable = sp.Reachable
			}
		}

		selfVideoDisabled := false
		luaEnabled := false
		if d.CfgPath != "" {
			if cfg, err := config.Load(d.CfgPath); err == nil {
				selfVideoDisabled = cfg.Viewer.VideoDisabled
				luaEnabled = cfg.Lua.Enabled
			}
		}

		vm := viewmodels.PeerContentVM{
			BaseVM:            baseVM("Peer", "peers", "page.peer", d),
			PeerID:            peerID,
			Content:           "", // loaded async via API
			PeerEmail:         peerEmail,
			AvatarHash:        avatarHash,
			VideoDisabled:     videoDisabled,
			SelfVideoDisabled: selfVideoDisabled,
			LuaEnabled:        luaEnabled,
			Reachable:         reachable,
		}
		render.Render(w, vm)
	})

	// API route - fetches remote peer content
	mux.HandleFunc("/api/peer/content", func(w http.ResponseWriter, r *http.Request) {
		peerID := r.URL.Query().Get("id")
		if peerID == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), util.DefaultFetchTimeout)
		defer cancel()

		content, err := d.Node.FetchContent(ctx, peerID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"content": content})
	})
}
