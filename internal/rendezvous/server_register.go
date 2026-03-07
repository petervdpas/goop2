package rendezvous

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type registerVM struct {
	Title       string
	Email       string
	Error       string
	Success     bool
	Verified    bool
	NotRequired bool
}

// handleRegisterRemote serves /register when a remote registration service is configured.
// GET: shows form (or "not required" page). POST: proxies to the registration service.
func (s *Server) handleRegisterRemote(w http.ResponseWriter, r *http.Request) {
	if s.registerTmpl == nil {
		http.Error(w, "registration not available", http.StatusNotFound)
		return
	}

	vm := registerVM{Title: "Register — Goop² Rendezvous"}

	if r.Method == http.MethodGet {
		if !s.registration.RegistrationRequired() {
			vm.NotRequired = true
		}
		s.renderRegister(w, vm)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// POST: parse email from form and proxy to registration service as JSON
	if err := r.ParseForm(); err != nil {
		vm.Error = "Invalid form data"
		s.renderRegister(w, vm)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		vm.Error = "Email is required"
		s.renderRegister(w, vm)
		return
	}

	// Call registration service POST /api/reg/register
	// Send as form-encoded data (matching the original reverse-proxy behaviour).
	regURL := s.registration.baseURL + "/api/reg/register"
	form := url.Values{}
	form.Set("email", email)
	if s.externalURL != "" {
		form.Set("verify_base_url", s.externalURL)
	}
	resp, err := http.Post(
		regURL,
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		log.Printf("registration: POST %s failed: %v", regURL, err)
		vm.Error = "Registration service unavailable"
		vm.Email = email
		s.renderRegister(w, vm)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("registration: failed to read response body: %v", err)
		vm.Error = "Registration failed"
		vm.Email = email
		s.renderRegister(w, vm)
		return
	}

	var result struct {
		Status string `json:"status"`
		Email  string `json:"email"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("registration: POST %s returned %d, body not JSON: %s", regURL, resp.StatusCode, string(respBody))
		vm.Error = "Registration failed"
		vm.Email = email
		s.renderRegister(w, vm)
		return
	}

	if resp.StatusCode/100 != 2 || result.Status != "ok" {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "Registration failed"
		}
		log.Printf("registration: POST %s returned %d: status=%q error=%q", regURL, resp.StatusCode, result.Status, result.Error)
		vm.Error = errMsg
		vm.Email = email
		s.renderRegister(w, vm)
		return
	}

	vm.Success = true
	vm.Email = email
	s.renderRegister(w, vm)
}

func (s *Server) renderRegister(w http.ResponseWriter, vm registerVM) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.registerTmpl.Execute(w, vm); err != nil {
		log.Printf("register template error: %v", err)
	}
}

func (s *Server) handleRegistrationsJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	if s.registration != nil {
		data, err := s.registration.FetchRegistrations()
		if err != nil {
			log.Printf("admin: fetch registrations: %v", err)
			http.Error(w, "service error", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(data)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("[]"))
}

func (s *Server) handleAccountsJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	cp, ok := s.credits.(*RemoteCreditProvider)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
		return
	}

	data, err := cp.FetchAccounts()
	if err != nil {
		log.Printf("admin: fetch accounts: %v", err)
		http.Error(w, "service error", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(data)
}

// handleLocalTemplateList serves GET /api/templates for the local template store.
func (s *Server) handleLocalTemplateList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(s.localTemplates.List())
}

// handleLocalTemplateRoutes handles /api/templates/<dir>/manifest and
// /api/templates/<dir>/bundle for the local template store.
// No registration or credit gating — all templates are free.
func (s *Server) handleLocalTemplateRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/templates/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	dir := parts[0]
	action := parts[1]

	switch action {
	case "manifest":
		meta, ok := s.localTemplates.GetManifest(dir)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(meta)

	case "bundle":
		w.Header().Set("Content-Type", "application/gzip")
		if err := s.localTemplates.WriteBundle(w, dir); err != nil {
			http.NotFound(w, r)
			return
		}

	default:
		http.NotFound(w, r)
	}
}

// handleTemplateRoutesRemote handles /api/templates/* sub-routes by proxying
// to the remote templates service. Bundle downloads are gated by registration
// and credit checks before proxying.
func (s *Server) handleTemplateRoutesRemote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// /api/templates/<dir>/bundle needs access control
	path := strings.TrimPrefix(r.URL.Path, "/api/templates/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 2 && parts[1] == "bundle" {
		dir := parts[0]
		// Registration gate: require verified email for template downloads
		peerID := getPeerID(r)
		if s.registration != nil && s.registration.RegistrationRequired() {
			if peerID == "" {
				http.Error(w, "registration required — provide peer_id", http.StatusForbidden)
				return
			}
			s.mu.Lock()
			peer, known := s.peers[peerID]
			s.mu.Unlock()
			if !known || !peer.Verified {
				http.Error(w, "registration required — verify your email and enter the token in settings", http.StatusForbidden)
				return
			}
		}
		// Credit check: use a minimal StoreMeta with just the dir for the access check
		meta := StoreMeta{Dir: dir, Source: "store"}
		if !s.credits.TemplateAccessAllowed(r, meta) {
			http.Error(w, "payment required", http.StatusPaymentRequired)
			return
		}
		// Inject email + token headers so the templates service can do its own checks
		if email := s.GetEmailForPeer(peerID); email != "" {
			r.Header.Set("X-Goop-Email", email)
		}
		if token := s.GetTokenForPeer(peerID); token != "" {
			r.Header.Set("X-Verification-Token", token)
		}
	}

	// Proxy the request to the remote templates service
	s.templates.Proxy().ServeHTTP(w, r)
}
