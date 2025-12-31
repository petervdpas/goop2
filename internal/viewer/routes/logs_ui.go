// internal/viewer/routes/logs_ui.go

package routes

import (
	"net/http"

	"goop/internal/ui/render"
)

func registerLogsUIRoutes(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		vm := render.LogsVM{
			BaseVM: baseVM("Logs", "logs", "page.logs", d),
		}
		render.Render(w, vm)
	})
}
