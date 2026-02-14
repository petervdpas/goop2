package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/petervdpas/goop2/internal/listen"
)

// RegisterListen adds listening room HTTP API endpoints.
func RegisterListen(mux *http.ServeMux, lm *listen.Manager) {

	// POST /api/listen/create — host creates a room
	mux.HandleFunc("/api/listen/create", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if decodeJSON(w, r, &req) != nil {
			return
		}
		if req.Name == "" {
			req.Name = "Listening Room"
		}

		room, err := lm.CreateRoom(req.Name)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusConflict)
			return
		}
		writeJSON(w, room)
	})

	// POST /api/listen/close — host closes room
	mux.HandleFunc("/api/listen/close", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if err := lm.CloseRoom(); err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "closed"})
	})

	// POST /api/listen/load — host loads an MP3 file
	mux.HandleFunc("/api/listen/load", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if !requireLocal(w, r) {
			return
		}
		var req struct {
			FilePath string `json:"file_path"`
		}
		if decodeJSON(w, r, &req) != nil {
			return
		}
		if req.FilePath == "" {
			http.Error(w, "missing file_path", http.StatusBadRequest)
			return
		}

		track, err := lm.LoadTrack(req.FilePath)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, track)
	})

	// POST /api/listen/control — host play/pause/seek
	mux.HandleFunc("/api/listen/control", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req struct {
			Action   string  `json:"action"`
			Position float64 `json:"position"`
		}
		if decodeJSON(w, r, &req) != nil {
			return
		}

		var err error
		switch req.Action {
		case "play":
			err = lm.Play()
		case "pause":
			err = lm.Pause()
		case "seek":
			err = lm.Seek(req.Position)
		default:
			http.Error(w, "unknown action: "+req.Action, http.StatusBadRequest)
			return
		}

		if err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// POST /api/listen/join — listener joins a room
	mux.HandleFunc("/api/listen/join", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req struct {
			HostPeerID string `json:"host_peer_id"`
			RoomID     string `json:"room_id"`
		}
		if decodeJSON(w, r, &req) != nil {
			return
		}
		if req.HostPeerID == "" || req.RoomID == "" {
			http.Error(w, "missing host_peer_id or room_id", http.StatusBadRequest)
			return
		}

		if err := lm.JoinRoom(req.HostPeerID, req.RoomID); err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]string{"status": "joined"})
	})

	// POST /api/listen/leave — listener leaves
	mux.HandleFunc("/api/listen/leave", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if err := lm.LeaveRoom(); err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "left"})
	})

	// GET /api/listen/stream — audio stream (Content-Type: audio/mpeg)
	mux.HandleFunc("/api/listen/stream", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}

		reader, err := lm.AudioReader()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusServiceUnavailable)
			return
		}
		defer reader.Close()

		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Cache-Control", "no-cache, no-store")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)

		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			if err != nil {
				return
			}
		}
	})

	// GET /api/listen/state — current room state
	mux.HandleFunc("/api/listen/state", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		room := lm.GetRoom()
		if room == nil {
			writeJSON(w, map[string]any{"room": nil})
			return
		}
		writeJSON(w, map[string]any{"room": room})
	})

	// GET /api/listen/events — SSE for state updates
	mux.HandleFunc("/api/listen/events", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}

		sseHeaders(w)

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		evtCh, cancel := lm.SubscribeSSE()
		defer cancel()

		fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
		flusher.Flush()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case room, ok := <-evtCh:
				if !ok {
					return
				}
				data, err := json.Marshal(map[string]any{"room": room})
				if err != nil {
					log.Printf("LISTEN: marshal error: %v", err)
					continue
				}
				fmt.Fprintf(w, "event: state\ndata: %s\n\n", data)
				flusher.Flush()
			}
		}
	})
}
