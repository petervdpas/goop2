package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/ui/viewmodels"
)

func registerGroupsUIRoutes(mux *http.ServeMux, d Deps) {
	// Groups sub-tab under Me
	mux.HandleFunc("/self/groups", func(w http.ResponseWriter, r *http.Request) {
		vm := viewmodels.GroupsVM{
			BaseVM: baseVM("Groups", "self", "page.groups", d),
		}
		render.Render(w, vm)
	})

	// Create > Groups sub-tab
	mux.HandleFunc("/create/groups", func(w http.ResponseWriter, r *http.Request) {
		vm := viewmodels.CreateGroupsVM{
			BaseVM: baseVM("Create Groups", "create", "page.create_groups", d),
		}
		render.Render(w, vm)
	})
}
