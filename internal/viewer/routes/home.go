
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
		if d.CfgPath != "" {
			if cfg, err := config.Load(d.CfgPath); err == nil {
				selfVideoDisabled = cfg.Viewer.VideoDisabled
				hideUnverified = cfg.Viewer.HideUnverified
			}
		}
		vm := viewmodels.PeersVM{
			BaseVM:            baseVM("Goop", "peers", "page.peers", d),
			Peers:             viewmodels.BuildPeerRows(d.Peers.Snapshot()),
			SelfVideoDisabled: selfVideoDisabled,
			HideUnverified:    hideUnverified,
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
				data, _ := json.Marshal(evt)
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
				flusher.Flush()
			}
		}
	})


	// Probe all peers synchronously and return the updated list.
	mux.HandleFunc("/api/peers/probe", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		d.Node.ProbeAllPeers(r.Context())
		writeJSON(w, viewmodels.BuildPeerRows(d.Peers.Snapshot()))
	})

	// JSON endpoint for peers list
	mux.HandleFunc("/api/peers", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, viewmodels.BuildPeerRows(d.Peers.Snapshot()))
	})

	// JSON endpoint for self identity
	mux.HandleFunc("/api/self", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
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
