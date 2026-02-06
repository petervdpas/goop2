// internal/viewer/routes/settings.go
package routes

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"goop/internal/config"
	"goop/internal/ui/render"
	"goop/internal/ui/viewmodels"
)

func registerSettingsRoutes(mux *http.ServeMux, d Deps, csrf string) {
	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/self#settings", http.StatusFound)
	})

	// API endpoint to save theme (used by header toggle)
	mux.HandleFunc("/api/theme", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		theme := r.URL.Query().Get("theme")
		if theme != "light" && theme != "dark" {
			http.Error(w, "invalid theme", http.StatusBadRequest)
			return
		}

		cfg, err := config.Load(d.CfgPath)
		if err != nil {
			http.Error(w, "failed to load config", http.StatusInternalServerError)
			return
		}

		cfg.Viewer.Theme = theme
		if err := config.Save(d.CfgPath, cfg); err != nil {
			http.Error(w, "failed to save config", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	// Quick settings API — saves only profile + theme (no CSRF form needed)
	mux.HandleFunc("/api/settings/quick", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isLocalRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var req struct {
			Label string `json:"label"`
			Email string `json:"email"`
			Theme string `json:"theme"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		cfg, err := config.Load(d.CfgPath)
		if err != nil {
			http.Error(w, "failed to load config", http.StatusInternalServerError)
			return
		}

		cfg.Profile.Label = strings.TrimSpace(req.Label)
		cfg.Profile.Email = strings.TrimSpace(req.Email)
		if req.Theme == "light" || req.Theme == "dark" {
			cfg.Viewer.Theme = req.Theme
		}

		if err := config.Save(d.CfgPath, cfg); err != nil {
			http.Error(w, "failed to save", http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/settings/save", func(w http.ResponseWriter, r *http.Request) {
		if err := validatePOSTRequest(w, r, csrf); err != nil {
			return
		}

		cfg, err := config.Load(d.CfgPath)
		if err != nil {
			http.Error(w, "failed to load config: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Profile (label/email) is managed via /api/settings/quick (navbar popup).
		cfg.Viewer.HTTPAddr = getTrimmedPostFormValue(r.PostForm, "viewer_http_addr")

		// Handle theme
		theme := getTrimmedPostFormValue(r.PostForm, "viewer_theme")
		if theme == "light" || theme == "dark" {
			cfg.Viewer.Theme = theme
		}

		// Handle debug checkbox
		switch strings.ToLower(getTrimmedPostFormValue(r.PostForm, "viewer_debug")) {
		case "on", "1", "true", "yes":
			cfg.Viewer.Debug = true
		default:
			cfg.Viewer.Debug = false
		}

		// P2P / presence fields are only in the form when not in rendezvous-only mode.
		if v := getTrimmedPostFormValue(r.PostForm, "p2p_mdns_tag"); v != "" {
			cfg.P2P.MdnsTag = v
		}
		if p := getTrimmedPostFormValue(r.PostForm, "p2p_listen_port"); p != "" {
			cfg.P2P.ListenPort = atoiOrNeg(p)
		}
		if ttl := getTrimmedPostFormValue(r.PostForm, "presence_ttl_sec"); ttl != "" {
			cfg.Presence.TTLSec = atoiOrNeg(ttl)
		}
		if hb := getTrimmedPostFormValue(r.PostForm, "presence_heartbeat_sec"); hb != "" {
			cfg.Presence.HeartbeatSec = atoiOrNeg(hb)
		}

		switch strings.ToLower(getTrimmedPostFormValue(r.PostForm, "presence_rendezvous_server")) {
		case "on", "1", "true", "yes":
			cfg.Presence.RendezvousHost = true
			cfg.Presence.RendezvousOnly = true
		default:
			cfg.Presence.RendezvousHost = false
			cfg.Presence.RendezvousOnly = false
		}

		if rp := getTrimmedPostFormValue(r.PostForm, "presence_rendezvous_port"); rp != "" {
			cfg.Presence.RendezvousPort = atoiOrNeg(rp)
		}

		cfg.Presence.RendezvousBind = getTrimmedPostFormValue(r.PostForm, "presence_rendezvous_bind")
		cfg.Presence.RendezvousWAN = getTrimmedPostFormValue(r.PostForm, "presence_rendezvous_wan")
		cfg.Presence.AdminPassword = getTrimmedPostFormValue(r.PostForm, "presence_admin_password")
		cfg.Presence.ExternalURL = getTrimmedPostFormValue(r.PostForm, "presence_external_url")
		cfg.Presence.RegistrationWebhook = getTrimmedPostFormValue(r.PostForm, "presence_registration_webhook")

		// SMTP settings
		cfg.Presence.SMTPHost = getTrimmedPostFormValue(r.PostForm, "presence_smtp_host")
		if sp := getTrimmedPostFormValue(r.PostForm, "presence_smtp_port"); sp != "" {
			cfg.Presence.SMTPPort, _ = strconv.Atoi(sp)
		}
		cfg.Presence.SMTPUsername = getTrimmedPostFormValue(r.PostForm, "presence_smtp_username")
		cfg.Presence.SMTPPassword = getTrimmedPostFormValue(r.PostForm, "presence_smtp_password")
		cfg.Presence.SMTPFrom = getTrimmedPostFormValue(r.PostForm, "presence_smtp_from")

		switch strings.ToLower(getTrimmedPostFormValue(r.PostForm, "presence_registration_required")) {
		case "on", "1", "true", "yes":
			cfg.Presence.RegistrationRequired = true
		default:
			cfg.Presence.RegistrationRequired = false
		}

		switch strings.ToLower(getTrimmedPostFormValue(r.PostForm, "lua_enabled")) {
		case "on", "1", "true", "yes":
			cfg.Lua.Enabled = true
		default:
			cfg.Lua.Enabled = false
		}

		if err := config.Save(d.CfgPath, cfg); err != nil {
			vm := viewmodels.SettingsVM{
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
