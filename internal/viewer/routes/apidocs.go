package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/ui/docs"
	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/ui/viewmodels"
)

func registerApiDocs(mux *http.ServeMux, d Deps) {
	rendered := docs.Render()

	mux.HandleFunc("/apidocs", func(w http.ResponseWriter, r *http.Request) {
		vm := viewmodels.ApiDocsVM{
			BaseVM: baseVM("API Docs", "create", "page.apidocs", d),
			SDKDoc: rendered.SDK,
			LuaDoc: rendered.Lua,
		}
		render.Render(w, vm)
	})
}
