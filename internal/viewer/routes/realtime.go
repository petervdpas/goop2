package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/petervdpas/goop2/internal/realtime"
)

// RegisterRealtime adds realtime channel HTTP API endpoints.
func RegisterRealtime(mux *http.ServeMux, rtMgr *realtime.Manager, selfID string) {
	// POST /api/realtime/connect — create channel + invite peer
	handlePost(mux, "/api/realtime/connect", func(w http.ResponseWriter, r *http.Request, req struct {
		PeerID string `json:"peer_id"`
	}) {
		if req.PeerID == "" {
			http.Error(w, "missing peer_id", http.StatusBadRequest)
			return
		}
		ch, err := rtMgr.CreateChannel(r.Context(), req.PeerID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, ch)
	})

	// POST /api/realtime/accept — accept incoming channel
	handlePost(mux, "/api/realtime/accept", func(w http.ResponseWriter, r *http.Request, req struct {
		ChannelID  string `json:"channel_id"`
		HostPeerID string `json:"host_peer_id"`
	}) {
		if req.ChannelID == "" || req.HostPeerID == "" {
			http.Error(w, "missing channel_id or host_peer_id", http.StatusBadRequest)
			return
		}
		ch, err := rtMgr.AcceptChannel(r.Context(), req.ChannelID, req.HostPeerID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, ch)
	})

	// POST /api/realtime/send — send message on channel
	handlePost(mux, "/api/realtime/send", func(w http.ResponseWriter, r *http.Request, req struct {
		ChannelID string `json:"channel_id"`
		Payload   any    `json:"payload"`
	}) {
		if req.ChannelID == "" {
			http.Error(w, "missing channel_id", http.StatusBadRequest)
			return
		}
		if err := rtMgr.Send(req.ChannelID, req.Payload); err != nil {
			http.Error(w, fmt.Sprintf("send failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "sent"})
	})

	// POST /api/realtime/close — close channel
	handlePost(mux, "/api/realtime/close", func(w http.ResponseWriter, r *http.Request, req struct {
		ChannelID string `json:"channel_id"`
	}) {
		if req.ChannelID == "" {
			http.Error(w, "missing channel_id", http.StatusBadRequest)
			return
		}
		if err := rtMgr.CloseChannel(req.ChannelID); err != nil {
			http.Error(w, fmt.Sprintf("close failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "closed"})
	})

	// GET /api/realtime/channels — list active channels
	handleGet(mux, "/api/realtime/channels", func(w http.ResponseWriter, r *http.Request) {
		channels := rtMgr.ListChannels()
		if channels == nil {
			channels = []*realtime.Channel{}
		}
		writeJSON(w, channels)
	})

	// GET /api/realtime/events?channel=X — SSE stream for a channel (or all)
	handleGet(mux, "/api/realtime/events", func(w http.ResponseWriter, r *http.Request) {
		sseHeaders(w)

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		channelFilter := r.URL.Query().Get("channel")

		evtCh, cancel := rtMgr.Subscribe()
		defer cancel()

		fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
		flusher.Flush()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case env, ok := <-evtCh:
				if !ok {
					return
				}
				// Filter by channel if specified
				if channelFilter != "" && env.Channel != channelFilter {
					continue
				}
				data, err := json.Marshal(env)
				if err != nil {
					log.Printf("REALTIME: marshal error: %v", err)
					continue
				}
				fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
				flusher.Flush()
			}
		}
	})
}
