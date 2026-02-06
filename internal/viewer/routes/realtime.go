package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"goop/internal/realtime"
)

// RegisterRealtime adds realtime channel HTTP API endpoints.
func RegisterRealtime(mux *http.ServeMux, rtMgr *realtime.Manager, selfID string) {
	// POST /api/realtime/connect — create channel + invite peer
	mux.HandleFunc("/api/realtime/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			PeerID string `json:"peer_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PeerID == "" {
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
	mux.HandleFunc("/api/realtime/accept", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			ChannelID  string `json:"channel_id"`
			HostPeerID string `json:"host_peer_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ChannelID == "" || req.HostPeerID == "" {
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
	mux.HandleFunc("/api/realtime/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			ChannelID string `json:"channel_id"`
			Payload   any    `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ChannelID == "" {
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
	mux.HandleFunc("/api/realtime/close", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ChannelID == "" {
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
	mux.HandleFunc("/api/realtime/channels", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		channels := rtMgr.ListChannels()
		if channels == nil {
			channels = []*realtime.Channel{}
		}
		writeJSON(w, channels)
	})

	// GET /api/realtime/events?channel=X — SSE stream for a channel (or all)
	mux.HandleFunc("/api/realtime/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

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
