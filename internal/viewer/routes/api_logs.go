// internal/viewer/routes/api_logs.go

package routes

import "net/http"

func registerAPILogRoutes(mux *http.ServeMux, d Deps) {
	if d.Logs == nil {
		return
	}
	mux.HandleFunc("/api/logs", d.Logs.ServeLogsJSON)
	mux.HandleFunc("/api/logs/stream", d.Logs.ServeLogsSSE)
}
