package routes

import (
	"fmt"
	"net/http"
	"time"

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

	// POST /api/listen/control — host play/pause/seek/next/prev
	handlePost(mux, "/api/listen/control", func(w http.ResponseWriter, r *http.Request, req struct {
		Action   string  `json:"action"`
		Position float64 `json:"position"`
		Index    int     `json:"index"`
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
		case "skip":
			err = lm.SkipToTrack(req.Index)
		case "remove":
			err = lm.RemoveFromQueue(req.Index)
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
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		lastDataTime := time.Now()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				// If no data received for 3 seconds, host is likely gone
				if time.Since(lastDataTime) > 3*time.Second {
					return
				}
			default:
				n, err := reader.Read(buf)
				if n > 0 {
					lastDataTime = time.Now()
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

}
