// internal/viewer/routes/self.go
package routes

import (
	"net/http"

	"goop/internal/config"
	"goop/internal/ui/render"
)

func registerSelfRoutes(mux *http.ServeMux, d Deps, csrf string) {
	mux.HandleFunc("/self", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := config.Load(d.CfgPath)
		if err != nil {
			vm := render.SettingsVM{
				BaseVM:  baseVM("Me", "self", "page.self", d),
				CfgPath: d.CfgPath,
				Error:   err.Error(),
				CSRF:    csrf,
			}
			render.Render(w, vm)
			return
		}

		vm := render.SettingsVM{
			BaseVM:  baseVM("Me", "self", "page.self", d),
			CfgPath: d.CfgPath,
			Cfg:     cfg,
			Saved:   (r.URL.Query().Get("saved") == "1"),
			CSRF:    csrf,
		}
		render.Render(w, vm)
	})
}
