
package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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

	// SSE endpoint for live peer updates
	mux.HandleFunc("/api/peers/events", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		sseHeaders(w)

		// Subscribe to peer updates
		ch := d.Peers.Subscribe()
		defer d.Peers.Unsubscribe(ch)

		// Send initial snapshot
		snapshot := d.Peers.Snapshot()
		snapshotData, _ := json.Marshal(map[string]any{
			"type":  "snapshot",
			"peers": viewmodels.BuildPeerRows(snapshot),
		})
		fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", snapshotData)
		flusher.Flush()

		heartbeat := time.NewTicker(25 * time.Second)
		defer heartbeat.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeat.C:
				fmt.Fprintf(w, ": ping\n\n")
				flusher.Flush()
			case evt, ok := <-ch:
				if !ok {
					return
				}
				var data []byte
				if evt.Type == "update" && evt.PeerID != "" && evt.Peer != nil {
					data, _ = json.Marshal(map[string]any{
						"type":    "update",
						"peer_id": evt.PeerID,
						"peer":    viewmodels.BuildPeerRow(evt.PeerID, *evt.Peer),
					})
				} else {
					data, _ = json.Marshal(evt)
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
				flusher.Flush()
			}
		}
	})


	// Probe all peers synchronously and return the updated list.
	handlePostAction(mux, "/api/peers/probe", func(w http.ResponseWriter, r *http.Request) {
		d.Node.ProbeAllPeers(r.Context())
		writeJSON(w, viewmodels.BuildPeerRows(d.Peers.Snapshot()))
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
