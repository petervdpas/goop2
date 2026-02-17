
package routes

import (
	"fmt"
	"io"
	"net/http"
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
