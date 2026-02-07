package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/ui/viewmodels"
)

func registerDocsUIRoutes(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/documents", func(w http.ResponseWriter, r *http.Request) {
		vm := viewmodels.DocsVM{
			BaseVM: baseVM("Files", "documents", "page.documents", d),
		}
		render.Render(w, vm)
	})
}
