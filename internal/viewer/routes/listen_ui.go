package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/ui/viewmodels"
)

// RegisterListenUI registers the listen room HTML page route.
func RegisterListenUI(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/listen", func(w http.ResponseWriter, r *http.Request) {
		vm := viewmodels.ListenVM{
			BaseVM: baseVM("Listen", "listen", "page.listen", d),
		}
		render.Render(w, vm)
	})
}
