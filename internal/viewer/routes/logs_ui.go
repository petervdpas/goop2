
package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/ui/viewmodels"
)

func registerLogsUIRoutes(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		vm := viewmodels.LogsVM{
			BaseVM: baseVM("Logs", "logs", "page.logs", d),
		}
		render.Render(w, vm)
	})
}
