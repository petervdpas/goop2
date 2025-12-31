// internal/viewer/routes/settings.go
package routes

import (
	"net/http"
	"strings"

	"goop/internal/config"
	"goop/internal/viewer/render"
)

func registerSettingsRoutes(mux *http.ServeMux, d Deps, csrf string) {
	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/self#settings", http.StatusFound)
	})

	mux.HandleFunc("/settings/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("csrf") != csrf {
			http.Error(w, "bad csrf", http.StatusForbidden)
			return
		}

		cfg, err := config.Load(d.CfgPath)
		if err != nil {
			http.Error(w, "failed to load config: "+err.Error(), http.StatusInternalServerError)
			return
		}

		cfg.Profile.Label = strings.TrimSpace(r.PostForm.Get("profile_label"))
		cfg.Viewer.HTTPAddr = strings.TrimSpace(r.PostForm.Get("viewer_http_addr"))
		cfg.P2P.MdnsTag = strings.TrimSpace(r.PostForm.Get("p2p_mdns_tag"))

		if p := strings.TrimSpace(r.PostForm.Get("p2p_listen_port")); p != "" {
			cfg.P2P.ListenPort = atoiOrNeg(p)
		}
		if ttl := strings.TrimSpace(r.PostForm.Get("presence_ttl_sec")); ttl != "" {
			cfg.Presence.TTLSec = atoiOrNeg(ttl)
		}
		if hb := strings.TrimSpace(r.PostForm.Get("presence_heartbeat_sec")); hb != "" {
			cfg.Presence.HeartbeatSec = atoiOrNeg(hb)
		}

		switch strings.ToLower(strings.TrimSpace(r.PostForm.Get("presence_rendezvous_host"))) {
		case "on", "1", "true", "yes":
			cfg.Presence.RendezvousHost = true
		default:
			cfg.Presence.RendezvousHost = false
		}

		if rp := strings.TrimSpace(r.PostForm.Get("presence_rendezvous_port")); rp != "" {
			cfg.Presence.RendezvousPort = atoiOrNeg(rp)
		}

		cfg.Presence.RendezvousWAN = strings.TrimSpace(r.PostForm.Get("presence_rendezvous_wan"))

		if err := config.Save(d.CfgPath, cfg); err != nil {
			vm := render.SettingsVM{
				BaseVM:  baseVM("Me", "self", "page.self", d),
				CfgPath: d.CfgPath,
				Cfg:     cfg,
				Error:   err.Error(),
				CSRF:    csrf,
			}
			render.Render(w, vm)
			return
		}

		http.Redirect(w, r, "/self?saved=1#settings", http.StatusFound)
	})
}
