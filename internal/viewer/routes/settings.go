package routes

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/ui/viewmodels"
)

func registerSettingsRoutes(mux *http.ServeMux, d Deps, csrf string) {
	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/self#settings", http.StatusFound)
	})

	// Quick settings API — partial update, only non-nil fields are written.
	// Used by settings popup (all fields), theme toggle (theme only), etc.
	mux.HandleFunc("/api/settings/quick", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if !requireLocal(w, r) {
			return
		}

		var req struct {
			Label          *string `json:"label"`
			Email          *string `json:"email"`
			Theme          *string `json:"theme"`
			PreferredCam   *string `json:"preferred_cam"`
			PreferredMic   *string `json:"preferred_mic"`
			VideoDisabled  *bool   `json:"video_disabled"`
			HideUnverified *bool   `json:"hide_unverified"`
			UseServices    *bool   `json:"use_services"`
		}
		if decodeJSON(w, r, &req) != nil {
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
		if req.VideoDisabled != nil {
			cfg.Viewer.VideoDisabled = *req.VideoDisabled
		}
		if req.HideUnverified != nil {
			cfg.Viewer.HideUnverified = *req.HideUnverified
		}
		if req.UseServices != nil {
			cfg.Presence.UseServices = *req.UseServices
		}

		if err := config.Save(d.CfgPath, cfg); err != nil {
			http.Error(w, "failed to save", http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]string{"status": "ok"})
	})

	// GET quick settings (used by settings popup + call JS to read device prefs)
	mux.HandleFunc("/api/settings/quick/get", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		cfg, err := config.Load(d.CfgPath)
		if err != nil {
			http.Error(w, "failed to load config", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]interface{}{
			"label":           cfg.Profile.Label,
			"email":           cfg.Profile.Email,
			"theme":           cfg.Viewer.Theme,
			"preferred_cam":   cfg.Viewer.PreferredCam,
			"preferred_mic":   cfg.Viewer.PreferredMic,
			"video_disabled":  cfg.Viewer.VideoDisabled,
			"hide_unverified": cfg.Viewer.HideUnverified,
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
		cfg.Viewer.VideoDisabled = formBool(r.PostForm, "viewer_video_disabled")
		cfg.Viewer.HideUnverified = formBool(r.PostForm, "viewer_hide_unverified")

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

		// Relay settings
		if rp := getTrimmedPostFormValue(r.PostForm, "presence_relay_port"); rp != "" {
			cfg.Presence.RelayPort, _ = strconv.Atoi(rp)
		} else {
			cfg.Presence.RelayPort = 0
		}
		if rkf := getTrimmedPostFormValue(r.PostForm, "presence_relay_key_file"); rkf != "" {
			cfg.Presence.RelayKeyFile = rkf
		}

		// Relay timing
		if v := getTrimmedPostFormValue(r.PostForm, "presence_relay_cleanup_delay_sec"); v != "" {
			cfg.Presence.RelayCleanupDelaySec, _ = strconv.Atoi(v)
		}
		if v := getTrimmedPostFormValue(r.PostForm, "presence_relay_poll_deadline_sec"); v != "" {
			cfg.Presence.RelayPollDeadlineSec, _ = strconv.Atoi(v)
		}
		if v := getTrimmedPostFormValue(r.PostForm, "presence_relay_connect_timeout_sec"); v != "" {
			cfg.Presence.RelayConnectTimeoutSec, _ = strconv.Atoi(v)
		}
		if v := getTrimmedPostFormValue(r.PostForm, "presence_relay_refresh_interval_sec"); v != "" {
			cfg.Presence.RelayRefreshIntervalSec, _ = strconv.Atoi(v)
		}
		if v := getTrimmedPostFormValue(r.PostForm, "presence_relay_recovery_grace_sec"); v != "" {
			cfg.Presence.RelayRecoveryGraceSec, _ = strconv.Atoi(v)
		}

		// Services toggle + URLs + admin tokens
		cfg.Presence.UseServices = formBool(r.PostForm, "presence_use_services")
		cfg.Presence.CreditsURL = getTrimmedPostFormValue(r.PostForm, "presence_credits_url")
		cfg.Presence.RegistrationURL = getTrimmedPostFormValue(r.PostForm, "presence_registration_url")
		cfg.Presence.EmailURL = getTrimmedPostFormValue(r.PostForm, "presence_email_url")
		cfg.Presence.TemplatesURL = getTrimmedPostFormValue(r.PostForm, "presence_templates_url")
		cfg.Presence.TemplatesDir = getTrimmedPostFormValue(r.PostForm, "presence_templates_dir")
		cfg.Presence.CreditsAdminToken = getTrimmedPostFormValue(r.PostForm, "presence_credits_admin_token")
		cfg.Presence.RegistrationAdminToken = getTrimmedPostFormValue(r.PostForm, "presence_registration_admin_token")
		cfg.Presence.TemplatesAdminToken = getTrimmedPostFormValue(r.PostForm, "presence_templates_admin_token")

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

	// Health check endpoint for external services
	mux.HandleFunc("/api/services/health", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}

		cfg, err := config.Load(d.CfgPath)
		if err != nil {
			http.Error(w, "failed to load config", http.StatusInternalServerError)
			return
		}

		client := &http.Client{Timeout: 3 * time.Second}

		writeJSON(w, map[string]interface{}{
			"registration": fetchServiceHealth(client, cfg.Presence.RegistrationURL, "/api/reg/status", []string{"registration_required", "dummy_mode"}),
			"credits":      fetchServiceHealth(client, cfg.Presence.CreditsURL, "/api/credits/store-data", []string{"dummy_mode"}),
			"email":        fetchServiceHealth(client, cfg.Presence.EmailURL, "/api/email/status", []string{"dummy_mode"}),
			"templates":    fetchServiceHealth(client, cfg.Presence.TemplatesURL, "/api/templates/status", []string{"dummy_mode"}),
		})
	})

	// Single-service health check using a URL from the form (not saved config)
	mux.HandleFunc("/api/services/check", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}

		svcURL := strings.TrimSpace(r.URL.Query().Get("url"))
		svcType := r.URL.Query().Get("type") // "registration", "credits", "email", or "templates"
		if svcURL == "" {
			writeJSON(w, map[string]interface{}{"ok": false, "error": "no url"})
			return
		}

		statusPaths := map[string]string{
			"registration": "/api/reg/status",
			"credits":      "/api/credits/store-data",
			"email":        "/api/email/status",
			"templates":    "/api/templates/status",
		}

		client := &http.Client{Timeout: 3 * time.Second}
		result := fetchServiceHealth(client, svcURL, statusPaths[svcType], []string{"dummy_mode"})
		if !result["ok"].(bool) {
			result["error"] = "not reachable"
		}
		writeJSON(w, result)
	})
}
