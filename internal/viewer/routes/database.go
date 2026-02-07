// internal/viewer/routes/database.go

package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/ui/viewmodels"
)

func registerDatabaseRoutes(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/database", func(w http.ResponseWriter, r *http.Request) {
		vm := viewmodels.DatabaseVM{
			BaseVM: baseVM("Database", "database", "page.database", d),
		}
		render.Render(w, vm)
	})
}
