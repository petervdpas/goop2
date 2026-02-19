package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/petervdpas/goop2/internal/listen"
)

// RegisterListen adds listening group HTTP API endpoints.
func RegisterListen(mux *http.ServeMux, lm *listen.Manager, peerName func(string) string) {

	// POST /api/listen/create — host creates a group
	handlePost(mux, "/api/listen/create", func(w http.ResponseWriter, r *http.Request, req struct {
		Name string `json:"name"`
	}) {
		if req.Name == "" {
			req.Name = "Listening Group"
		}
		group, err := lm.CreateGroup(req.Name)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusConflict)
			return
		}
		writeJSON(w, group)
	})

	// POST /api/listen/close — host closes group
	handlePostAction(mux, "/api/listen/close", func(w http.ResponseWriter, r *http.Request) {
		if err := lm.CloseGroup(); err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "closed"})
	})

	// POST /api/listen/load — host loads one or more MP3 files as a playlist.
	// Accepts either {file_path: "..."} or {file_paths: ["...", ...]}.
	handlePost(mux, "/api/listen/load", func(w http.ResponseWriter, r *http.Request, req struct {
		FilePath  string   `json:"file_path"`
		FilePaths []string `json:"file_paths"`
	}) {
		if !requireLocal(w, r) {
			return
		}
		paths := req.FilePaths
		if len(paths) == 0 && req.FilePath != "" {
			paths = []string{req.FilePath}
		}
		if len(paths) == 0 {
			http.Error(w, "missing file_path or file_paths", http.StatusBadRequest)
			return
		}
		track, err := lm.LoadQueue(paths)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, track)
	})

	// POST /api/listen/queue/add — append files to the playlist
	handlePost(mux, "/api/listen/queue/add", func(w http.ResponseWriter, r *http.Request, req struct {
		FilePaths []string `json:"file_paths"`
	}) {
		if !requireLocal(w, r) {
			return
		}
		if len(req.FilePaths) == 0 {
			http.Error(w, "missing file_paths", http.StatusBadRequest)
			return
		}
		if err := lm.AddToQueue(req.FilePaths); err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// POST /api/listen/control — host play/pause/seek
	handlePost(mux, "/api/listen/control", func(w http.ResponseWriter, r *http.Request, req struct {
		Action   string  `json:"action"`
		Position float64 `json:"position"`
	}) {
		var err error
		switch req.Action {
		case "play":
			err = lm.Play()
		case "pause":
			err = lm.Pause()
		case "seek":
			err = lm.Seek(req.Position)
		case "next":
			err = lm.Next()
		case "prev":
			err = lm.Prev()
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

	// POST /api/listen/join — listener joins a group
	handlePost(mux, "/api/listen/join", func(w http.ResponseWriter, r *http.Request, req struct {
		HostPeerID string `json:"host_peer_id"`
		GroupID    string `json:"group_id"`
	}) {
		if req.HostPeerID == "" || req.GroupID == "" {
			http.Error(w, "missing host_peer_id or group_id", http.StatusBadRequest)
			return
		}
		if err := lm.JoinGroup(req.HostPeerID, req.GroupID); err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]string{"status": "joined"})
	})

	// POST /api/listen/leave — listener leaves
	handlePostAction(mux, "/api/listen/leave", func(w http.ResponseWriter, r *http.Request) {
		if err := lm.LeaveGroup(); err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "left"})
	})

	// GET /api/listen/stream — audio stream (Content-Type: audio/mpeg)
	handleGet(mux, "/api/listen/stream", func(w http.ResponseWriter, r *http.Request) {
		reader, err := lm.AudioReader()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed: %v", err), http.StatusServiceUnavailable)
			return
		}
		defer reader.Close()

		// When the browser disconnects (audio.src="" or tab close), close the pipe
		// reader immediately so any goroutine blocked writing to the pipe unblocks.
		go func() {
			<-r.Context().Done()
			reader.Close()
		}()

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

	// GET /api/listen/state — current group state
	handleGet(mux, "/api/listen/state", func(w http.ResponseWriter, r *http.Request) {
		group := lm.GetGroup()
		if group == nil {
			writeJSON(w, map[string]any{"group": nil})
			return
		}
		names := make(map[string]string, len(group.Listeners))
		for _, pid := range group.Listeners {
			if n := peerName(pid); n != "" {
				names[pid] = n
			}
		}
		writeJSON(w, map[string]any{"group": group, "listener_names": names})
	})

	// GET /api/listen/events — SSE for state updates
	handleGet(mux, "/api/listen/events", func(w http.ResponseWriter, r *http.Request) {
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
			case group, ok := <-evtCh:
				if !ok {
					return
				}
				data, err := json.Marshal(map[string]any{"group": group})
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
