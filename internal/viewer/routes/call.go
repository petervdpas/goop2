package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/petervdpas/goop2/internal/call"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 65536,
	// Allow connections from the Wails webview (localhost, file://, etc.)
	CheckOrigin: func(r *http.Request) bool { return true },
}

// RegisterCall registers the native call API endpoints.
// callMgr may be nil — in that case only GET /api/call/mode is registered
// and it always returns {"mode":"browser"}, so the frontend always has a safe
// endpoint to query regardless of whether the feature is enabled.
func RegisterCall(mux *http.ServeMux, callMgr *call.Manager) {
	// GET /api/call/mode — always registered; safe to call in any mode.
	handleGet(mux, "/api/call/mode", func(w http.ResponseWriter, r *http.Request) {
		mode := "browser"
		if callMgr != nil {
			mode = "native"
		}
		writeJSON(w, map[string]string{"mode": mode})
	})

	if callMgr == nil {
		return
	}

	// GET /api/call/debug — live session status for testing without a UI.
	// Returns all active sessions with their PC state and RTP stats.
	handleGet(mux, "/api/call/debug", func(w http.ResponseWriter, r *http.Request) {
		sessions := callMgr.AllSessions()
		statuses := make([]call.SessionStatus, 0, len(sessions))
		for _, s := range sessions {
			statuses = append(statuses, s.Status())
		}
		writeJSON(w, map[string]any{
			"session_count": len(statuses),
			"sessions":      statuses,
		})
	})

	// POST /api/call/start
	handlePost(mux, "/api/call/start", func(w http.ResponseWriter, r *http.Request, req struct {
		ChannelID  string `json:"channel_id"`
		RemotePeer string `json:"remote_peer"`
	}) {
		if req.ChannelID == "" || req.RemotePeer == "" {
			http.Error(w, "missing channel_id or remote_peer", http.StatusBadRequest)
			return
		}
		if _, err := callMgr.StartCall(r.Context(), req.ChannelID, req.RemotePeer); err != nil {
			http.Error(w, fmt.Sprintf("start call failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "started", "channel_id": req.ChannelID})
	})

	// POST /api/call/accept
	handlePost(mux, "/api/call/accept", func(w http.ResponseWriter, r *http.Request, req struct {
		ChannelID  string `json:"channel_id"`
		RemotePeer string `json:"remote_peer"`
	}) {
		if req.ChannelID == "" || req.RemotePeer == "" {
			http.Error(w, "missing channel_id or remote_peer", http.StatusBadRequest)
			return
		}
		if _, err := callMgr.AcceptCall(r.Context(), req.ChannelID, req.RemotePeer); err != nil {
			http.Error(w, fmt.Sprintf("accept call failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "accepted", "channel_id": req.ChannelID})
	})

	// POST /api/call/hangup
	handlePost(mux, "/api/call/hangup", func(w http.ResponseWriter, r *http.Request, req struct {
		ChannelID string `json:"channel_id"`
	}) {
		if req.ChannelID == "" {
			http.Error(w, "missing channel_id", http.StatusBadRequest)
			return
		}
		sess, ok := callMgr.GetSession(req.ChannelID)
		if !ok {
			writeJSON(w, map[string]string{"status": "not_found"})
			return
		}
		sess.Hangup()
		writeJSON(w, map[string]string{"status": "hung_up"})
	})

	// POST /api/call/toggle-audio
	handlePost(mux, "/api/call/toggle-audio", func(w http.ResponseWriter, r *http.Request, req struct {
		ChannelID string `json:"channel_id"`
	}) {
		sess, ok := callMgr.GetSession(req.ChannelID)
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]bool{"muted": sess.ToggleAudio()})
	})

	// POST /api/call/toggle-video
	handlePost(mux, "/api/call/toggle-video", func(w http.ResponseWriter, r *http.Request, req struct {
		ChannelID string `json:"channel_id"`
	}) {
		sess, ok := callMgr.GetSession(req.ChannelID)
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]bool{"disabled": sess.ToggleVideo()})
	})

	// GET /api/call/events — SSE stream: incoming call notifications.
	// Each connection gets its own subscription channel; unsubscribed on disconnect
	// so the manager never accumulates stale handlers.
	handleGet(mux, "/api/call/events", func(w http.ResponseWriter, r *http.Request) {
		sseHeaders(w)
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		inCh := callMgr.SubscribeIncoming()
		defer callMgr.UnsubscribeIncoming(inCh)

		fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
		flusher.Flush()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case ic, ok := <-inCh:
				if !ok {
					return
				}
				data, err := json.Marshal(map[string]string{
					"type":        "incoming-call",
					"channel_id":  ic.ChannelID,
					"remote_peer": ic.RemotePeer,
				})
				if err != nil {
					continue
				}
				fmt.Fprintf(w, "event: call\ndata: %s\n\n", data)
				flusher.Flush()
			}
		}
	})

	// GET /api/call/session/{channel}/events — SSE: per-session events (hangup, state).
	// The browser subscribes after start/accept; fires once when the call ends.
	mux.HandleFunc("/api/call/session/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Path: /api/call/session/{channel}/events
		tail := strings.TrimPrefix(r.URL.Path, "/api/call/session/")
		parts := strings.SplitN(tail, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] != "events" {
			http.Error(w, "invalid path — expected /api/call/session/{channel}/events", http.StatusBadRequest)
			return
		}
		channelID := parts[0]

		sess, ok := callMgr.GetSession(channelID)
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		sseHeaders(w)
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
		flusher.Flush()

		select {
		case <-r.Context().Done():
			// Client disconnected before hangup — that's fine.
		case <-sess.HangupCh():
			data, _ := json.Marshal(map[string]string{
				"type":       "hangup",
				"channel_id": channelID,
			})
			fmt.Fprintf(w, "event: hangup\ndata: %s\n\n", data)
			flusher.Flush()
		}
	})

	// GET /api/call/media/{channel} — WebSocket: live WebM stream for Phase 4 browser display.
	// The browser's MSE API receives binary WebM messages and feeds them to a <video> element.
	// First message is the init segment; subsequent messages are clusters.
	mux.HandleFunc("/api/call/media/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		channelID := strings.TrimPrefix(r.URL.Path, "/api/call/media/")
		channelID = strings.TrimSuffix(channelID, "/")
		if channelID == "" {
			http.Error(w, "missing channel id", http.StatusBadRequest)
			return
		}

		sess, ok := callMgr.GetSession(channelID)
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("CALL [%s]: WebSocket upgrade error: %v", channelID, err)
			return
		}
		defer conn.Close()
		log.Printf("CALL [%s]: media WebSocket connected", channelID)

		dataCh, cancel := sess.SubscribeMedia()
		defer cancel()

		// Drain incoming messages (ping/pong, close frames) without blocking.
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()

		for {
			select {
			case <-r.Context().Done():
				log.Printf("CALL [%s]: media WebSocket disconnected", channelID)
				return
			case <-sess.HangupCh():
				return
			case data, ok := <-dataCh:
				if !ok {
					return
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
					return
				}
			}
		}
	})

	// Loopback signaling routes — browser ↔ Go localhost WebRTC (Phase 4).
	// POST /api/call/loopback/{channel}/offer   → browser SDP offer; Go returns SDP answer
	// POST /api/call/loopback/{channel}/ice      → browser ICE candidates → Go LocalPC
	// GET  /api/call/loopback/{channel}/ice      → SSE: Go LocalPC ICE candidates → browser
	mux.HandleFunc("/api/call/loopback/", func(w http.ResponseWriter, r *http.Request) {
		tail := strings.TrimPrefix(r.URL.Path, "/api/call/loopback/")
		parts := strings.SplitN(tail, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			http.Error(w, "invalid path — expected /api/call/loopback/{channel}/{action}", http.StatusBadRequest)
			return
		}
		channelID, action := parts[0], parts[1]

		sess, ok := callMgr.GetSession(channelID)
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		switch action {
		case "offer":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				SDP string `json:"sdp"`
			}
			if decodeJSON(w, r, &body) != nil {
				return
			}
			_ = sess
			// TODO Phase 4: pass SDP offer to LocalPC, return real SDP answer.
			writeJSON(w, map[string]string{"sdp": "", "status": "stub — Phase 4 pending"})

		case "ice":
			switch r.Method {
			case http.MethodPost:
				var body struct {
					Candidate string `json:"candidate"`
					Mid       string `json:"sdpMid"`
					Index     int    `json:"sdpMLineIndex"`
				}
				if decodeJSON(w, r, &body) != nil {
					return
				}
				_ = sess
				// TODO Phase 4: add to LocalPC remote candidates.
				writeJSON(w, map[string]string{"status": "ok"})

			case http.MethodGet:
				sseHeaders(w)
				flusher, ok := w.(http.Flusher)
				if !ok {
					http.Error(w, "streaming not supported", http.StatusInternalServerError)
					return
				}
				fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
				flusher.Flush()
				// TODO Phase 4: stream real ICE candidates from LocalPC.
				<-r.Context().Done()

			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}

		default:
			http.Error(w, "unknown loopback action", http.StatusNotFound)
		}
	})
}
