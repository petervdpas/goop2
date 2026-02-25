package routes

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/petervdpas/goop2/internal/call"
	"github.com/petervdpas/goop2/internal/mq"
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
// modeFirstSeen is flipped to true after the first /api/call/mode request
// that returns "native". JS uses the "first" field to log exactly once per
// server process (survives page navigations in Wails).
var modeFirstSeen bool

func RegisterCall(mux *http.ServeMux, callMgr *call.Manager, mqMgr *mq.Manager) {
	// GET /api/call/mode — always registered; safe to call in any mode.
	handleGet(mux, "/api/call/mode", func(w http.ResponseWriter, r *http.Request) {
		mode := "browser"
		first := false
		if callMgr != nil {
			mode = "native"
			if !modeFirstSeen {
				modeFirstSeen = true
				first = true
				// Log via log.Printf so it flows through the SSE stream
				// and appears in the Video tab on the Logs page.
				log.Printf("[info] [call-native] mode=native — Go/Pion call stack active")
			}
		}
		writeJSON(w, map[string]any{"mode": mode, "first": first, "platform": runtime.GOOS})
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
		sess, err := callMgr.StartCall(r.Context(), req.ChannelID, req.RemotePeer)
		if err != nil {
			http.Error(w, fmt.Sprintf("start call failed: %v", err), http.StatusInternalServerError)
			return
		}
		watchHangup(sess, req.ChannelID, mqMgr)
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
		sess, err := callMgr.AcceptCall(r.Context(), req.ChannelID, req.RemotePeer)
		if err != nil {
			http.Error(w, fmt.Sprintf("accept call failed: %v", err), http.StatusInternalServerError)
			return
		}
		watchHangup(sess, req.ChannelID, mqMgr)
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
			// Only POST is accepted — browser sends its ICE candidates here for
			// Go's LocalPC (Phase 4).
			// The reverse direction (Go → browser ICE) uses MQ: mqMgr.PublishLoopbackICE()
			// publishes to topic "call:loopback:{channelID}"; the browser subscribes
			// via Goop.mq.onLoopbackICE(channelId, fn) in call-native.js.
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
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

		default:
			http.Error(w, "unknown loopback action", http.StatusNotFound)
		}
	})
}

// watchHangup spawns a goroutine that waits for the session to end and then
// publishes a call-hangup event to the browser via MQ PublishLocal so the
// call overlay closes regardless of how the hangup was triggered (remote peer,
// PC failure, etc.). mqMgr may be nil when native calls are disabled.
func watchHangup(sess *call.Session, channelID string, mqMgr *mq.Manager) {
	if mqMgr == nil {
		return
	}
	go func() {
		<-sess.HangupCh()
		mqMgr.PublishCallHangup(channelID)
	}()
}
