package rendezvous

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/swaggo/swag"
)

func (s *Server) handleStyle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("content-type", "text/css; charset=utf-8")
	_, _ = w.Write(s.style)
}

func (s *Server) handleDocsCSS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("content-type", "text/css; charset=utf-8")
	_, _ = w.Write(s.docsCSS)
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.favicon == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(s.favicon)
}

func (s *Server) handleSplash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.splash == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(s.splash)
}

func (s *Server) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	doc, err := swag.ReadDoc()
	if err != nil {
		http.Error(w, "spec unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(doc))
}

func (s *Server) handleExecutorAPISpec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(executorAPISpec)
}

func (s *Server) handleDocsRedirect(w http.ResponseWriter, r *http.Request) {
	if len(s.docsSite.Pages) == 0 {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/docs/"+s.docsSite.Pages[0].Slug, http.StatusFound)
}

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := strings.TrimPrefix(r.URL.Path, "/docs/")
	if slug == "" {
		s.handleDocsRedirect(w, r)
		return
	}

	page, ok := s.docsSite.BySlug[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Find prev/next pages.
	var prev, next *DocPage
	for i, p := range s.docsSite.Pages {
		if p.Slug == slug {
			if i > 0 {
				prev = &s.docsSite.Pages[i-1]
			}
			if i < len(s.docsSite.Pages)-1 {
				next = &s.docsSite.Pages[i+1]
			}
			break
		}
	}

	w.Header().Set("content-type", "text/html; charset=utf-8")
	_ = s.docsTmpl.Execute(w, docsVM{
		Title:   page.Title,
		Pages:   s.docsSite.Pages,
		Current: page,
		Prev:    prev,
		Next:    next,
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hasStore := false
	storeCount := 0
	if s.templates != nil {
		storeCount = s.templates.TemplateCount()
		hasStore = storeCount > 0
	} else if s.localTemplates != nil {
		storeCount = s.localTemplates.Count()
		hasStore = storeCount > 0
	}

	regRequired := false
	if s.registration != nil {
		regRequired = s.registration.RegistrationRequired()
	}

	_, hasCredits := s.credits.(*RemoteCreditProvider)

	w.Header().Set("content-type", "text/html; charset=utf-8")
	_ = s.tmpl.Execute(w, indexVM{
		Title:                "Goop² Rendezvous",
		Endpoint:             s.URL(),
		ConnectURLs:          s.connectURLs(),
		HasStore:             hasStore,
		StoreCount:           storeCount,
		HasAdmin:             s.adminPassword != "",
		RegistrationRequired: regRequired,
		HasCredits:           hasCredits,
		RegistrationCredits:  s.grantAmount(),
	})
}

func (s *Server) handleStore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Resolve the requesting peer's currently active template
	peerID := getPeerID(r)
	var activeTemplate string
	if peerID != "" {
		s.mu.Lock()
		if p, ok := s.peers[peerID]; ok {
			activeTemplate = p.ActiveTemplate
		}
		s.mu.Unlock()
	}

	var templates []storeTemplateVM
	if s.templates != nil {
		list, err := s.templates.FetchTemplates()
		if err != nil {
			log.Printf("templates: fetch list error: %v", err)
		}
		for _, meta := range list {
			info := s.credits.TemplateStoreInfo(r, meta)
			templates = append(templates, storeTemplateVM{
				Meta:       meta,
				PriceLabel: info.PriceLabel,
				IsActive:   meta.Dir == activeTemplate,
			})
		}
	} else if s.localTemplates != nil {
		for _, meta := range s.localTemplates.List() {
			templates = append(templates, storeTemplateVM{
				Meta:       meta,
				PriceLabel: `<span class="tpl-price-free">Free</span>`,
				IsActive:   meta.Dir == activeTemplate,
			})
		}
	}

	regRequired := false
	if s.registration != nil {
		regRequired = s.registration.RegistrationRequired()
	}

	w.Header().Set("content-type", "text/html; charset=utf-8")
	_, hasCredits := s.credits.(*RemoteCreditProvider)

	_ = s.storeTmpl.Execute(w, storeVM{
		Title:                "Template Store — Goop²",
		Templates:            templates,
		CreditData:           s.credits.StorePageData(r),
		HasAdmin:             s.adminPassword != "",
		HasCredits:           hasCredits,
		RegistrationRequired: regRequired,
		RegistrationCredits:  s.grantAmount(),
	})
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	peers := s.snapshotPeers()

	_, hasCredits := s.credits.(*RemoteCreditProvider)
	if !hasCredits {
		_, isNoCredits := s.credits.(NoCredits)
		hasCredits = !isNoCredits
	}

	var services []serviceStatus
	if s.registration != nil {
		ss := serviceStatus{Name: "Registration", URL: s.registration.baseURL}
		ss.OK = checkServiceHealth(s.registration.baseURL)
		if ss.OK {
			ss.DummyMode = !s.registration.RegistrationRequired()
			ss.Version = s.registration.Version()
			ss.APIVersion = s.registration.APIVersion()
			ss.APICompat = ss.APIVersion >= minRegistrationAPI
		}
		services = append(services, ss)
	}
	if cp, ok := s.credits.(*RemoteCreditProvider); ok {
		ss := serviceStatus{Name: "Credits", URL: cp.baseURL}
		ss.OK = checkServiceHealth(cp.baseURL)
		if ss.OK {
			cs := cp.fetchStoreStatus()
			ss.DummyMode = cs.DummyMode
			ss.Version = cs.Version
			ss.APIVersion = cs.APIVersion
			ss.APICompat = ss.APIVersion >= minCreditsAPI
		}
		services = append(services, ss)
	}
	if s.email != nil {
		ss := serviceStatus{Name: "Email", URL: s.email.baseURL}
		ss.OK = checkServiceHealth(s.email.baseURL)
		if ss.OK {
			ss.DummyMode = s.email.DummyMode()
			ss.Version = s.email.Version()
			ss.APIVersion = s.email.APIVersion()
			ss.APICompat = ss.APIVersion >= minEmailAPI
		}
		services = append(services, ss)
	}
	if s.templates != nil {
		ss := serviceStatus{Name: "Templates", URL: s.templates.baseURL}
		ss.OK = checkServiceHealth(s.templates.baseURL)
		if ss.OK {
			ss.DummyMode = s.templates.DummyMode()
			ss.Version = s.templates.Version()
			ss.APIVersion = s.templates.APIVersion()
			ss.APICompat = ss.APIVersion >= minTemplatesAPI
		}
		services = append(services, ss)
	}
	if s.bridge != nil {
		ss := serviceStatus{Name: "Bridge", URL: s.bridge.baseURL}
		ss.OK = checkServiceHealth(s.bridge.baseURL)
		if ss.OK {
			ss.DummyMode = s.bridge.DummyMode()
			ss.Version = s.bridge.Version()
			ss.APIVersion = s.bridge.APIVersion()
			ss.APICompat = ss.APIVersion >= minBridgeAPI
		}
		services = append(services, ss)
	}
	if s.encryption != nil {
		ss := serviceStatus{Name: "Encryption", URL: s.encryption.baseURL}
		ss.OK = checkServiceHealth(s.encryption.baseURL)
		if ss.OK {
			ss.DummyMode = s.encryption.DummyMode()
			ss.Version = s.encryption.Version()
			ss.APIVersion = s.encryption.APIVersion()
			ss.APICompat = ss.APIVersion >= minEncryptionAPI
			ss.KeyCount = s.encryption.KeyCount()
		}
		services = append(services, ss)
	}

	// Fetch topology from each running service
	var topologies []topologyInfo
	for _, svc := range services {
		if !svc.OK {
			continue
		}
		topo, err := fetchTopology(svc.URL, svc.Name)
		if err != nil {
			log.Printf("admin: topology %s: %v", svc.Name, err)
			continue
		}
		topologies = append(topologies, topo)
	}
	chainIssues := validateChain(topologies, services)

	// Merge services + topology into combined rows
	var serviceRows []adminServiceRow
	for _, svc := range services {
		row := adminServiceRow{serviceStatus: svc}
		for _, topo := range topologies {
			if strings.EqualFold(topo.Service, svc.Name) {
				row.Dependencies = topo.Dependencies
				break
			}
		}
		serviceRows = append(serviceRows, row)
	}

	// Only show data panels when the provider is configured AND has an admin token
	hasRegistrations := s.registration != nil && s.registration.adminToken != ""
	hasAccounts := false
	if cp, ok := s.credits.(*RemoteCreditProvider); ok {
		hasAccounts = cp.adminToken != ""
	}

	w.Header().Set("content-type", "text/html; charset=utf-8")
	relayPeerID := ""
	if s.relayInfo != nil {
		relayPeerID = s.relayInfo.PeerID
	}

	_ = s.adminTmpl.Execute(w, adminVM{
		Title:            "Goop² Admin",
		PeerCount:        len(peers),
		Peers:            peers,
		Now:              time.Now().Format("2006-01-02 15:04:05"),
		HasCredits:       hasCredits,
		HasRegistrations: hasRegistrations,
		HasAccounts:      hasAccounts,
		HasRelay:         s.relayHost != nil,
		RelayPeerID:      relayPeerID,
		RelayPort:        s.relayPort,
		RelayCleanup:     s.relayTiming.CleanupDelaySec,
		RelayPoll:        s.relayTiming.PollDeadlineSec,
		RelayConnect:     s.relayTiming.ConnectTimeoutSec,
		RelayRefresh:     s.relayTiming.RefreshIntervalSec,
		RelayGrace:       s.relayTiming.RecoveryGraceSec,
		Services:         services,
		ServiceRows:      serviceRows,
		ChainIssues:      chainIssues,
	})
}
