// internal/viewer/routes/self.go
package routes

import (
	"net/http"

	"goop/internal/config"
	"goop/internal/ui/render"
	"goop/internal/ui/viewmodels"
)

func registerSelfRoutes(mux *http.ServeMux, d Deps, csrf string) {
	mux.HandleFunc("/self", func(w http.ResponseWriter, r *http.Request) {
		avatarHash := ""
		if d.AvatarStore != nil {
			avatarHash = d.AvatarStore.Hash()
		}

		cfg, err := config.Load(d.CfgPath)
		if err != nil {
			vm := viewmodels.SettingsVM{
				BaseVM:     baseVM("Me", "self", "page.self", d),
				CfgPath:    d.CfgPath,
				AvatarHash: avatarHash,
				Error:      err.Error(),
				CSRF:       csrf,
			}
			render.Render(w, vm)
			return
		}

		vm := viewmodels.SettingsVM{
			BaseVM:     baseVM("Me", "self", "page.self", d),
			CfgPath:    d.CfgPath,
			AvatarHash: avatarHash,
			Cfg:        cfg,
			Saved:      (r.URL.Query().Get("saved") == "1"),
			CSRF:       csrf,
		}
		render.Render(w, vm)
	})
}
