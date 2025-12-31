// internal/viewer/routes/logs_ui.go

package routes

import (
	"net/http"

	"goop/internal/ui/render"
	"goop/internal/ui/viewmodels"
)

func registerLogsUIRoutes(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		vm := viewmodels.LogsVM{
			BaseVM: baseVM("Logs", "logs", "page.logs", d),
		}
		render.Render(w, vm)
	})
}
