// internal/viewer/routes/database.go

package routes

import (
	"net/http"

	"goop/internal/ui/render"
	"goop/internal/ui/viewmodels"
)

func registerDatabaseRoutes(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/database", func(w http.ResponseWriter, r *http.Request) {
		vm := viewmodels.DatabaseVM{
			BaseVM: baseVM("Database", "database", "page.database", d),
		}
		render.Render(w, vm)
	})
}
