
package routes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/petervdpas/goop2/internal/p2p"
)

// ClientLogger interface for writing client-side logs
type ClientLogger interface {
	Write(p []byte) (n int, err error)
}

func registerAPILogRoutes(mux *http.ServeMux, d Deps) {
	if d.Logs == nil {
		return
	}
	mux.HandleFunc("/api/logs", d.Logs.ServeLogsJSON)
	mux.HandleFunc("/api/logs/stream", d.Logs.ServeLogsSSE)

	mux.HandleFunc("/api/logs/verbose", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var req struct {
				On bool `json:"on"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			if w, ok := d.Logs.(io.Writer); ok {
				p2p.SetVerbose(req.On, w)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	// Client-side logging endpoint - allows frontend JS to write to the logs page
	handlePost(mux, "/api/logs/client", func(w http.ResponseWriter, r *http.Request, req struct {
		Level   string `json:"level"`   // "debug", "info", "warn", "error"
		Source  string `json:"source"`  // e.g., "webrtc", "call", "realtime"
		Message string `json:"message"` // the log message
	}) {
		if req.Message == "" {
			http.Error(w, "message required", http.StatusBadRequest)
			return
		}

		level := req.Level
		if level == "" {
			level = "info"
		}
		source := req.Source
		if source == "" {
			source = "client"
		}

		// Skip debug-level client logs to reduce noise.
		if level == "debug" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		logLine := fmt.Sprintf("[%s] [%s] %s\n", level, source, req.Message)

		if writer, ok := d.Logs.(io.Writer); ok {
			writer.Write([]byte(logLine))
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
