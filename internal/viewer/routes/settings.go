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

	// Quick settings API — partial update, only non-nil fields are written.
	// Used by settings popup (all fields), theme toggle (theme only), etc.
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
			Label        *string `json:"label"`
			Email        *string `json:"email"`
			Theme        *string `json:"theme"`
			PreferredCam *string `json:"preferred_cam"`
			PreferredMic *string `json:"preferred_mic"`
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

		if req.Label != nil {
			cfg.Profile.Label = strings.TrimSpace(*req.Label)
		}
		if req.Email != nil {
			cfg.Profile.Email = strings.TrimSpace(*req.Email)
		}
		if req.Theme != nil && isValidTheme(*req.Theme) {
			cfg.Viewer.Theme = *req.Theme
		}
		if req.PreferredCam != nil {
			cfg.Viewer.PreferredCam = *req.PreferredCam
		}
		if req.PreferredMic != nil {
			cfg.Viewer.PreferredMic = *req.PreferredMic
		}

		if err := config.Save(d.CfgPath, cfg); err != nil {
			http.Error(w, "failed to save", http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]string{"status": "ok"})
	})

	// GET quick settings (used by settings popup + call JS to read device prefs)
	mux.HandleFunc("/api/settings/quick/get", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cfg, err := config.Load(d.CfgPath)
		if err != nil {
			http.Error(w, "failed to load config", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{
			"label":         cfg.Profile.Label,
			"email":         cfg.Profile.Email,
			"theme":         cfg.Viewer.Theme,
			"preferred_cam": cfg.Viewer.PreferredCam,
			"preferred_mic": cfg.Viewer.PreferredMic,
		})
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

		// Profile, theme, and devices are managed via /api/settings/quick (navbar popup).
		cfg.Viewer.HTTPAddr = getTrimmedPostFormValue(r.PostForm, "viewer_http_addr")
		cfg.Viewer.Debug = formBool(r.PostForm, "viewer_debug")

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

		if formBool(r.PostForm, "presence_rendezvous_server") {
			cfg.Presence.RendezvousHost = true
			cfg.Presence.RendezvousOnly = true
		} else {
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

		cfg.Presence.RegistrationRequired = formBool(r.PostForm, "presence_registration_required")
		cfg.Lua.Enabled = formBool(r.PostForm, "lua_enabled")

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
