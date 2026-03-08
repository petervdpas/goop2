package rendezvous

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/proto"
	"github.com/petervdpas/goop2/internal/util"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/tdewolff/minify/v2"
	mincss "github.com/tdewolff/minify/v2/css"
)

//go:embed all:assets
var embedded embed.FS

const (
	maxSSEClients      = 1024 // global SSE connection limit
	maxSSEClientsPerIP = 10   // per-IP SSE connection limit
)

// RelayTimingConfig holds relay timing values from the config file.
// Zero values mean "use default".
type RelayTimingConfig struct {
	CleanupDelaySec    int
	PollDeadlineSec    int
	ConnectTimeoutSec  int
	RefreshIntervalSec int
	RecoveryGraceSec   int
}

type Server struct {
	addr          string
	externalURL   string // public URL for servers behind NAT/reverse proxy
	adminPassword string
	srv           *http.Server

	mu        sync.Mutex
	clients   map[chan []byte]struct{}
	clientIPs map[chan []byte]string // channel -> remote IP (for per-IP tracking)

	// simple in-memory peer view for the web page
	peers       map[string]peerRow
	peersDirty  bool      // set when peers map changes; cleared by snapshotPeers
	cachedPeers []peerRow // sorted snapshot cache

	// log buffer for web UI
	logMu   sync.Mutex
	logs    []string
	maxLogs int

	// relay-specific log buffer for admin Relay section
	relayLogMu   sync.Mutex
	relayLogs    []string
	maxRelayLogs int

	tmpl         *template.Template
	adminTmpl    *template.Template
	docsTmpl     *template.Template
	storeTmpl    *template.Template
	registerTmpl *template.Template
	style        []byte
	docsCSS      []byte
	favicon      []byte
	splash       []byte
	docsSite     *DocSite

	peerDB         *peerDB                     // nil when persistence is disabled
	credits        CreditProvider              // default: NoCredits{}
	registration   *RemoteRegistrationProvider // nil = use built-in registration
	email          *RemoteEmailProvider        // nil = email service not configured
	templates      *RemoteTemplatesProvider    // nil = templates service not configured
	localTemplates *LocalTemplateStore         // nil = no local template store

	// Bridge (HTTPS bridge microservice)
	bridge *RemoteBridgeProvider // nil = bridge service not configured

	// Encryption (E2E key distribution microservice)
	encryption *RemoteEncryptionProvider // nil = encryption service not configured

	// Circuit relay v2
	relayHost    host.Host  // nil when relay is disabled
	relayInfo    *RelayInfo // nil when relay is disabled
	relayPort    int
	relayKeyFile string
	relayTiming  RelayTimingConfig

	// per-IP rate limiter for /publish
	rateMu     sync.Mutex
	rateWindow map[string]*rateBucket

	// punch hint cooldowns: prevents spamming hole-punch attempts for the same peer pair
	punchCooldowns map[[2]string]time.Time

	// WebSocket clients: peerID → connection (authenticated, per-peer channel)
	wsClients   map[string]*wsClient
	wsClientsMu sync.RWMutex
}

// rateBucket is a fixed-size ring buffer of timestamps for rate limiting.
// Avoids per-request slice allocations.
const rateBucketCap = 60

type rateBucket struct {
	times [rateBucketCap]time.Time
	head  int
	count int
}

type peerRow struct {
	PeerID              string   `json:"peer_id"`
	Type                string   `json:"type"`
	Content             string   `json:"content"`
	Email               string   `json:"email,omitempty"`
	AvatarHash          string   `json:"avatar_hash,omitempty"`
	ActiveTemplate      string   `json:"active_template,omitempty"`
	PublicKey           string   `json:"public_key,omitempty"`
	EncryptionSupported bool     `json:"encryption_supported,omitempty"`
	Addrs               []string `json:"addrs,omitempty"`
	TS                  int64    `json:"ts"`
	LastSeen            int64    `json:"last_seen"`
	BytesSent           int64    `json:"bytes_sent"`
	BytesReceived       int64    `json:"bytes_received"`
	Verified            bool     `json:"verified"`
	WSConnected         bool     `json:"ws_connected,omitempty"`

	// Internal-only: stored server-side, never broadcast to peers.
	verificationToken string
}

type indexVM struct {
	Title                string
	Endpoint             string
	ConnectURLs          []string
	HasStore             bool
	StoreCount           int
	HasAdmin             bool
	RegistrationRequired bool
	HasCredits           bool
	RegistrationCredits  int
}

type storeTemplateVM struct {
	Meta       StoreMeta
	PriceLabel template.HTML
	IsActive   bool // currently applied by the requesting peer
}

type storeVM struct {
	Title                string
	Templates            []storeTemplateVM
	CreditData           StorePageData
	HasAdmin             bool
	HasCredits           bool
	RegistrationRequired bool
	RegistrationCredits  int
}

// Minimum API versions that this build of goop2 requires.
const (
	minRegistrationAPI = 1
	minCreditsAPI      = 1
	minEmailAPI        = 1
	minTemplatesAPI    = 1
	minBridgeAPI       = 1
	minEncryptionAPI   = 1
)

type serviceStatus struct {
	Name       string
	URL        string
	OK         bool
	DummyMode  bool
	Version    string
	APIVersion int
	APICompat  bool // true if api_version >= required minimum
}

type topologyDep struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type topologyInfo struct {
	Service      string        `json:"service"`
	Addr         string        `json:"addr"`
	Dependencies []topologyDep `json:"dependencies"`
}

type adminServiceRow struct {
	serviceStatus
	Dependencies []topologyDep
}

type adminVM struct {
	Title            string
	PeerCount        int
	Peers            []peerRow
	Now              string
	HasCredits       bool
	HasRegistrations bool
	HasAccounts      bool
	HasRelay         bool
	RelayPeerID      string
	RelayPort        int
	RelayCleanup     int
	RelayPoll        int
	RelayConnect     int
	RelayRefresh     int
	RelayGrace       int
	Services         []serviceStatus
	ServiceRows      []adminServiceRow
	ChainIssues      []string
}

type docsVM struct {
	Title   string
	Pages   []DocPage
	Current *DocPage
	Prev    *DocPage
	Next    *DocPage
}

func New(addr string, peerDBPath string, adminPassword string, externalURL string, relayPort int, relayKeyFile string, relayTiming RelayTimingConfig) *Server {
	funcs := template.FuncMap{
		"statusClass": func(t string) string {
			switch t {
			case proto.TypeOnline:
				return "on"
			case proto.TypeUpdate:
				return "up"
			case proto.TypeOffline:
				return "off"
			default:
				return ""
			}
		},
		"fmtMillis": func(ms int64) string {
			if ms <= 0 {
				return ""
			}
			return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
		},
		"fmtBytes": func(b int64) string {
			if b < 1024 {
				return fmt.Sprintf("%d B", b)
			} else if b < 1024*1024 {
				return fmt.Sprintf("%.1f KB", float64(b)/1024)
			} else if b < 1024*1024*1024 {
				return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
			}
			return fmt.Sprintf("%.1f GB", float64(b)/(1024*1024*1024))
		},
	}

	tmpl, err := template.New("index.html").Funcs(funcs).ParseFS(embedded, "assets/index.html")
	if err != nil {
		panic(err)
	}

	adminTmpl, err := template.New("admin.html").Funcs(funcs).ParseFS(embedded, "assets/admin.html")
	if err != nil {
		panic(err)
	}

	docsTmpl, err := template.New("docs.html").Funcs(funcs).ParseFS(embedded, "assets/docs.html")
	if err != nil {
		panic(err)
	}

	storeTmpl, err := template.New("store.html").Funcs(funcs).ParseFS(embedded, "assets/store.html")
	if err != nil {
		panic(err)
	}

	registerTmpl, err := template.New("register.html").Funcs(funcs).ParseFS(embedded, "assets/register.html")
	if err != nil {
		// Not fatal - registration template is optional
		registerTmpl = nil
	}

	css, err := embedded.ReadFile("assets/style.css")
	if err != nil {
		css = []byte("/* missing style.css */")
	}
	css = minifyCSS(css)

	docsCSSData, err := embedded.ReadFile("assets/docs.css")
	if err != nil {
		docsCSSData = []byte("/* missing docs.css */")
	}
	docsCSSData = minifyCSS(docsCSSData)

	faviconData, err := embedded.ReadFile("assets/favicon.ico")
	if err != nil {
		faviconData = nil
	}

	splashData, err := embedded.ReadFile("assets/goop2-splash.jpg")
	if err != nil {
		splashData = nil
	}

	s := &Server{
		addr:           addr,
		externalURL:    util.NormalizeURL(externalURL),
		adminPassword:  adminPassword,
		clients:        map[chan []byte]struct{}{},
		clientIPs:      map[chan []byte]string{},
		peers:          map[string]peerRow{},
		logs:           make([]string, 0, 500),
		maxLogs:        500,
		relayLogs:      make([]string, 0, 500),
		maxRelayLogs:   500,
		tmpl:           tmpl,
		adminTmpl:      adminTmpl,
		docsTmpl:       docsTmpl,
		storeTmpl:      storeTmpl,
		registerTmpl:   registerTmpl,
		style:          css,
		docsCSS:        docsCSSData,
		favicon:        faviconData,
		splash:         splashData,
		docsSite:       newDocSite(),
		relayPort:      relayPort,
		relayKeyFile:   relayKeyFile,
		relayTiming:    relayTiming,
		rateWindow:     map[string]*rateBucket{},
		punchCooldowns: map[[2]string]time.Time{},
		wsClients:      map[string]*wsClient{},
	}

	// Open peer DB if path provided (for multi-instance persistence)
	if peerDBPath != "" {
		db, err := openPeerDB(peerDBPath)
		if err != nil {
			log.Printf("WARNING: peer DB open failed: %v (running in-memory only)", err)
		} else {
			s.peerDB = db
		}
	}

	s.credits = NoCredits{}

	return s
}

// minifyCSS minifies CSS bytes using tdewolff/minify.
// If minification fails, the original bytes are returned unchanged.
func minifyCSS(data []byte) []byte {
	m := minify.New()
	m.AddFunc("text/css", mincss.Minify)
	out, err := m.Bytes("text/css", data)
	if err != nil {
		log.Printf("minify: warning: CSS minification failed: %v", err)
		return data
	}
	return out
}

// SetCreditProvider replaces the default NoCredits provider.
// Must be called before Start.
func (s *Server) SetCreditProvider(cp CreditProvider) {
	s.credits = cp
}

// GetEmailForPeer resolves a peer ID to an email address.
// Only returns the email if the peer is verified (has a valid token).
// Checks in-memory peers first, then falls back to peerDB.
func (s *Server) GetEmailForPeer(peerID string) string {
	s.mu.Lock()
	if p, ok := s.peers[peerID]; ok && p.Email != "" && p.Verified {
		s.mu.Unlock()
		return p.Email
	}
	s.mu.Unlock()

	if s.peerDB != nil {
		return s.peerDB.lookupEmail(peerID)
	}
	return ""
}

// GetTokenForPeer returns the stored verification token for a peer.
// Used to pass downstream to services for defense-in-depth validation.
func (s *Server) GetTokenForPeer(peerID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.peers[peerID]; ok && p.Verified {
		return p.verificationToken
	}
	return ""
}

// grantAmount returns the grant_amount from the registration service, or 0 if not configured.
func (s *Server) grantAmount() int {
	if s.registration == nil {
		return 0
	}
	return s.registration.GrantAmount()
}

// SetRegistrationProvider configures a remote registration service.
// When set, registration endpoints are proxied to the remote service
// and email verification checks delegate to it.
// Must be called before Start.
func (s *Server) SetRegistrationProvider(rp *RemoteRegistrationProvider) {
	s.registration = rp
}

// SetEmailProvider configures a remote email service.
// When set, email endpoints are proxied to the remote service.
// Must be called before Start.
func (s *Server) SetEmailProvider(ep *RemoteEmailProvider) {
	s.email = ep
}

// SetTemplatesProvider configures a remote templates service.
// When set, template API endpoints are proxied to the remote service.
// Must be called before Start.
func (s *Server) SetTemplatesProvider(tp *RemoteTemplatesProvider) {
	s.templates = tp
}

// SetLocalTemplateStore configures a local (disk-based) template store.
// Used when no remote templates service is configured. All templates are
// served for free without registration or credit gating.
// Must be called before Start.
func (s *Server) SetLocalTemplateStore(ts *LocalTemplateStore) {
	s.localTemplates = ts
}

// SetBridgeProvider configures a remote bridge service.
// When set, bridge endpoints are proxied to the remote service.
func (s *Server) SetBridgeProvider(bp *RemoteBridgeProvider) {
	s.bridge = bp
}

// SetEncryptionProvider configures a remote encryption service.
// When set, encryption endpoints are proxied to the remote service.
func (s *Server) SetEncryptionProvider(ep *RemoteEncryptionProvider) {
	s.encryption = ep
}

func (s *Server) Start(ctx context.Context) error {
	// Start circuit relay v2 host if configured
	if s.relayPort > 0 {
		rh, ri, err := StartRelay(s.relayPort, s.relayKeyFile, s.externalURL, s.relayAddLog)
		if err != nil {
			return fmt.Errorf("start relay: %w", err)
		}
		s.relayHost = rh
		s.relayInfo = ri

		// Inject timing config into relay info for clients.
		ri.CleanupDelaySec = s.relayTiming.CleanupDelaySec
		ri.PollDeadlineSec = s.relayTiming.PollDeadlineSec
		ri.ConnectTimeoutSec = s.relayTiming.ConnectTimeoutSec
		ri.RefreshIntervalSec = s.relayTiming.RefreshIntervalSec
		ri.RecoveryGraceSec = s.relayTiming.RecoveryGraceSec

		// Log relay network events to the dedicated relay log.
		rh.Network().Notify(&network.NotifyBundle{
			ConnectedF: func(_ network.Network, c network.Conn) {
				s.relayAddLog(fmt.Sprintf("peer connected: %s from %s (%s)", c.RemotePeer(), c.RemoteMultiaddr(), dirString(c.Stat().Direction)))
			},
			DisconnectedF: func(_ network.Network, c network.Conn) {
				s.relayAddLog(fmt.Sprintf("peer disconnected: %s (%s)", c.RemotePeer(), c.RemoteMultiaddr()))
			},
		})

		s.relayAddLog(fmt.Sprintf("started on port %d, peer ID %s (%d addrs)", s.relayPort, ri.PeerID, len(ri.Addrs)))

		// Periodically log relay connection count and check reservation health.
		// Peers connected to the relay but not publishing a circuit address
		// likely have a broken reservation — log them so the admin can see it
		// without clicking Diagnose on each peer.
		go func() {
			t := time.NewTicker(60 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					relayPeers := rh.Network().Peers()
					if len(relayPeers) == 0 {
						continue
					}

					var ids []string
					for _, p := range relayPeers {
						ids = append(ids, p.String()[:16]+"...")
					}
					s.relayAddLog(fmt.Sprintf("%d peers: %s", len(relayPeers), strings.Join(ids, ", ")))

					// Cross-reference: which relay-connected peers have no circuit address?
					s.mu.Lock()
					var noReservation []string
					for _, rp := range relayPeers {
						rpID := rp.String()
						if pr, ok := s.peers[rpID]; ok {
							hasCircuit := false
							for _, a := range pr.Addrs {
								if strings.Contains(a, "p2p-circuit") {
									hasCircuit = true
									break
								}
							}
							if !hasCircuit {
								name := pr.Content
								if name == "" {
									name = rpID[:16] + "..."
								}
								noReservation = append(noReservation, name)
							}
						}
					}
					s.mu.Unlock()

					if len(noReservation) > 0 {
						s.relayAddLog(fmt.Sprintf("NO_RESERVATION: %s (connected but no circuit address)", strings.Join(noReservation, ", ")))
					}
				}
			}
		}()

		// Shut down relay when context ends
		go func() {
			<-ctx.Done()
			_ = rh.Close()
		}()
	}

	// Load existing peers from SQLite on startup
	if s.peerDB != nil {
		s.loadPeersFromDB()
	}

	// Start peer cleanup goroutine
	go s.cleanupStalePeers(ctx)

	// Periodic sync from DB (catch peers from other instances)
	if s.peerDB != nil {
		go s.syncFromDB(ctx)
	}

	mux := http.NewServeMux()

	// Public endpoints
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/assets/style.css", s.handleStyle)
	mux.HandleFunc("/assets/docs.css", s.handleDocsCSS)
	mux.HandleFunc("/favicon.ico", s.handleFavicon)
	mux.HandleFunc("/assets/goop2-splash.jpg", s.handleSplash)
	mux.HandleFunc("/docs", s.handleDocsRedirect)
	mux.HandleFunc("/docs/", s.handleDocs)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})

	// Capabilities endpoint — tells peers what this rendezvous offers.
	mux.HandleFunc("/api/capabilities", func(w http.ResponseWriter, r *http.Request) {
		_, isNoCredits := s.credits.(NoCredits)
		caps := map[string]bool{
			"encryption":   s.encryption != nil,
			"registration": s.registration != nil,
			"credits":      !isNoCredits,
			"templates":    s.templates != nil,
			"bridge":       s.bridge != nil,
			"relay":        s.relayHost != nil,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(caps)
	})

	// Relay info endpoint (returns 404 when relay is disabled)
	mux.HandleFunc("/relay", func(w http.ResponseWriter, r *http.Request) {
		handleRelayInfo(w, r, s.relayInfo)
	})

	// Bridge endpoints (proxied to bridge service)
	if s.bridge != nil {
		s.bridge.RegisterRoutes(mux, s.registration)
	}

	// Encryption endpoints (proxied to encryption service)
	if s.encryption != nil {
		s.encryption.RegisterRoutes(mux)
	}

	// Admin-protected endpoints
	mux.HandleFunc("/admin", s.handleAdmin)
	mux.HandleFunc("/peers.json", s.handlePeersJSON)
	mux.HandleFunc("/logs.json", s.handleLogsJSON)
	mux.HandleFunc("/relay-status.json", s.handleRelayStatusJSON)
	mux.HandleFunc("/registrations.json", s.handleRegistrationsJSON)
	mux.HandleFunc("/accounts.json", s.handleAccountsJSON)
	mux.HandleFunc("/api/services/logs", s.handleServiceLogs)
	mux.HandleFunc("/diag", s.handleDiagPeer)
	mux.HandleFunc("/api/pulse", s.handlePulse)

	// Registration endpoints
	if s.registration != nil {
		// Remote registration service — proxy /api/reg/*
		s.registration.RegisterRoutes(mux)
		// /register is always served locally (form + POST proxy)
		mux.HandleFunc("/register", s.handleRegisterRemote)
		// /verify calls registration service and renders HTML
		mux.HandleFunc("/verify", func(w http.ResponseWriter, r *http.Request) {
			token := r.URL.Query().Get("token")
			if token == "" {
				http.Error(w, "missing token", http.StatusBadRequest)
				return
			}
			email, ok := s.registration.HandleVerify(token)
			vm := registerVM{Title: "Verified — Goop² Rendezvous"}
			if ok {
				vm.Email = email
				vm.Success = true
				vm.Verified = true
			} else {
				vm.Error = "Invalid or expired verification link"
			}
			if s.registerTmpl != nil {
				s.renderRegister(w, vm)
			} else {
				http.Error(w, "registration not available", http.StatusNotFound)
			}
		})
	}

	// SSE: stream messages to subscribers
	// WebSocket entangler: per-peer bidirectional channel for presence + punch hints.
	// Replaces SSE for peers that support it; SSE remains for backward compatibility.
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		s.handleWS(ctx, w, r)
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := make(chan []byte, 64)
		remoteIP := extractIP(r.RemoteAddr)
		if err := s.addClient(ch, remoteIP); err != nil {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
			return
		}
		defer s.removeClient(ch)

		// Initial comment so proxies flush headers
		_, _ = w.Write([]byte(": ok\n\n"))
		flusher.Flush()

		heartbeat := time.NewTicker(25 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ctx.Done():
				return
			case <-heartbeat.C:
				// keep-alive comment
				_, _ = w.Write([]byte(": ping\n\n"))
				flusher.Flush()
			case b := <-ch:
				// SSE "data:" line(s)
				_, _ = w.Write([]byte("data: "))
				_, _ = w.Write(b)
				_, _ = w.Write([]byte("\n\n"))
				flusher.Flush()
			}
		}
	})

	// Publish: accept PresenceMsg JSON and broadcast to SSE subscribers
	mux.HandleFunc("/publish", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Per-IP rate limiting: 60 requests per minute
		ip := extractIP(r.RemoteAddr)
		if !s.allowPublish(ip) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		var pm proto.PresenceMsg
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&pm); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}

		if err := validatePresence(pm); err != nil {
			http.Error(w, "bad message: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Check registration if the registration service requires it
		isRegistered := true
		if s.registration != nil && s.registration.RegistrationRequired() {
			if pm.Email == "" || pm.VerificationToken == "" {
				isRegistered = false
			} else {
				isRegistered = s.registration.IsEmailTokenValid(pm.Email, pm.VerificationToken)
			}
		}

		// Save token server-side before stripping from broadcast message
		peerToken := pm.VerificationToken
		pm.VerificationToken = ""

		// normalize timestamp if caller didn't set it
		if pm.TS == 0 {
			pm.TS = proto.NowMillis()
		}

		// Annotate with server-side verification status
		pm.Verified = isRegistered

		// Calculate message size for tracking
		b, _ := json.Marshal(pm)
		msgSize := int64(len(b))

		// update peer snapshot for / and /peers.json
		// Always store and broadcast — mark unverified peers
		addrsChanged := s.upsertPeer(pm, msgSize, isRegistered, peerToken)
		s.addLog(fmt.Sprintf("Received %s from %s: %q (verified=%v)", pm.Type, pm.PeerID, pm.Content, isRegistered))
		s.broadcast(b)

		if pm.Type == proto.TypeOnline || pm.Type == proto.TypeUpdate {
			s.emitPunchHints(pm, addrsChanged)
		}

		w.WriteHeader(http.StatusNoContent)
	})

	// Store page
	mux.HandleFunc("/store", s.handleStore)

	// Credit provider routes (e.g. /api/credits/*)
	s.credits.RegisterRoutes(mux)

	// Email service routes (e.g. /api/email/*)
	if s.email != nil {
		s.email.RegisterRoutes(mux)
	}

	// Template store API — proxy to remote templates service
	if s.templates != nil {
		s.templates.RegisterRoutes(mux) // /api/templates/prices (exact match, with auth)
		proxy := s.templates.Proxy()
		mux.HandleFunc("/api/templates", func(w http.ResponseWriter, r *http.Request) {
			// Gate listing: require verified email when registration is enabled.
			// Admin users (Basic Auth) bypass the gate so the admin panel can
			// load template metadata (icons, names) for the price editor.
			if s.registration != nil && s.registration.RegistrationRequired() && !s.isAdmin(r) {
				peerID := getPeerID(r)
				if peerID == "" {
					http.Error(w, "register and verify your email to access the template store", http.StatusForbidden)
					return
				}
				s.mu.Lock()
				peer, known := s.peers[peerID]
				s.mu.Unlock()
				if !known || !peer.Verified {
					http.Error(w, "register and verify your email, then enter the token in settings", http.StatusForbidden)
					return
				}
			}
			proxy.ServeHTTP(w, r)
		})
		mux.HandleFunc("/api/templates/", s.handleTemplateRoutesRemote)
	} else if s.localTemplates != nil {
		// Local template store — no pricing, no registration gating
		mux.HandleFunc("/api/templates", s.handleLocalTemplateList)
		mux.HandleFunc("/api/templates/", s.handleLocalTemplateRoutes)
	}

	s.srv = &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Stop server when ctx ends
	go func() {
		<-ctx.Done()
		shctx, cancel := context.WithTimeout(context.Background(), util.ShortTimeout)
		defer cancel()
		_ = s.srv.Shutdown(shctx)
		if s.peerDB != nil {
			_ = s.peerDB.close()
		}
	}()

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("rendezvous server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) URL() string {
	if s.externalURL != "" {
		return s.externalURL
	}
	return "http://" + s.addr
}

// connectURLs returns HTTP URLs that remote peers can use to reach this
// rendezvous server. If an external URL is configured, it returns that.
// Otherwise, it discovers non-loopback IPv4 addresses and pairs them with
// the server's listen port.
func (s *Server) connectURLs() []string {
	// If external URL is configured, use it instead of auto-discovery
	if s.externalURL != "" {
		return []string{s.externalURL}
	}

	_, port, _ := net.SplitHostPort(s.addr)
	if port == "" {
		port = "8787"
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var urls []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		// Skip Docker, veth, and other virtual bridge interfaces
		name := strings.ToLower(iface.Name)
		if strings.HasPrefix(name, "docker") ||
			strings.HasPrefix(name, "veth") ||
			strings.HasPrefix(name, "br-") ||
			strings.HasPrefix(name, "virbr") {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			urls = append(urls, fmt.Sprintf("http://%s:%s", ip.String(), port))
		}
	}
	return urls
}

func (s *Server) addClient(ch chan []byte, remoteIP string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.clients) >= maxSSEClients {
		return fmt.Errorf("too many SSE connections (%d)", maxSSEClients)
	}

	// Per-IP limit
	ipCount := 0
	for _, ip := range s.clientIPs {
		if ip == remoteIP {
			ipCount++
		}
	}
	if ipCount >= maxSSEClientsPerIP {
		return fmt.Errorf("too many SSE connections from %s (%d)", remoteIP, maxSSEClientsPerIP)
	}

	s.clients[ch] = struct{}{}
	s.clientIPs[ch] = remoteIP
	return nil
}

func (s *Server) removeClient(ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, ch)
	delete(s.clientIPs, ch)
	close(ch)
}

// extractIP returns the IP portion of a host:port address.
func extractIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func (s *Server) addLog(msg string) {
	s.logMu.Lock()
	defer s.logMu.Unlock()

	timestamp := time.Now().Format("15:04:05")
	logLine := fmt.Sprintf("[%s] %s", timestamp, msg)

	s.logs = append(s.logs, logLine)
	if len(s.logs) > s.maxLogs {
		s.logs = s.logs[len(s.logs)-s.maxLogs:]
	}

	// Also log to console
	log.Println(msg)
}

// relayAddLog appends a log entry to the relay-specific log buffer.
func (s *Server) relayAddLog(msg string) {
	s.relayLogMu.Lock()
	defer s.relayLogMu.Unlock()

	timestamp := time.Now().Format("15:04:05")
	logLine := fmt.Sprintf("[%s] %s", timestamp, msg)

	s.relayLogs = append(s.relayLogs, logLine)
	if len(s.relayLogs) > s.maxRelayLogs {
		s.relayLogs = s.relayLogs[len(s.relayLogs)-s.maxRelayLogs:]
	}

	log.Printf("relay: %s", msg)
}

// isAdmin returns true if the request carries valid admin Basic Auth credentials.
func (s *Server) isAdmin(r *http.Request) bool {
	if s.adminPassword == "" {
		return false
	}
	user, pass, ok := r.BasicAuth()
	return ok && user == "admin" && pass == s.adminPassword
}

// requireAdmin checks HTTP Basic Auth. Returns true if authorized.
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.adminPassword == "" {
		http.Error(w, "admin panel disabled", http.StatusForbidden)
		return false
	}
	user, pass, ok := r.BasicAuth()
	if !ok || user != "admin" || pass != s.adminPassword {
		w.Header().Set("WWW-Authenticate", `Basic realm="Goop2 Admin"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

// allowPublish checks the per-IP sliding window rate limit (60 req/min).
func (s *Server) allowPublish(ip string) bool {
	window := time.Minute
	now := time.Now()
	cutoff := now.Add(-window)

	s.rateMu.Lock()
	defer s.rateMu.Unlock()

	bucket, ok := s.rateWindow[ip]
	if !ok {
		bucket = &rateBucket{}
		s.rateWindow[ip] = bucket
	}

	// Trim expired entries from the front
	for bucket.count > 0 {
		oldest := bucket.times[bucket.head]
		if oldest.After(cutoff) {
			break
		}
		bucket.head = (bucket.head + 1) % rateBucketCap
		bucket.count--
	}

	if bucket.count >= rateBucketCap {
		return false
	}

	// Push new timestamp
	idx := (bucket.head + bucket.count) % rateBucketCap
	bucket.times[idx] = now
	bucket.count++
	return true
}

// cleanupRateLimiter removes stale entries from the rate limiter map.
func (s *Server) cleanupRateLimiter() {
	cutoff := time.Now().Add(-time.Minute)

	s.rateMu.Lock()
	defer s.rateMu.Unlock()

	for ip, bucket := range s.rateWindow {
		// Trim expired entries from the front
		for bucket.count > 0 {
			oldest := bucket.times[bucket.head]
			if oldest.After(cutoff) {
				break
			}
			bucket.head = (bucket.head + 1) % rateBucketCap
			bucket.count--
		}
		if bucket.count == 0 {
			delete(s.rateWindow, ip)
		}
	}
}

func validatePresence(pm proto.PresenceMsg) error {
	pm.Type = strings.TrimSpace(pm.Type)
	pm.PeerID = strings.TrimSpace(pm.PeerID)

	if pm.Type == "" || pm.PeerID == "" {
		return fmt.Errorf("type and peerId are required")
	}

	switch pm.Type {
	case proto.TypeOnline, proto.TypeUpdate, proto.TypeOffline:
	default:
		return fmt.Errorf("unknown type %q", pm.Type)
	}

	// minimal sanity: keep payload bounded
	if len(pm.PeerID) > 256 {
		return fmt.Errorf("peerId too long")
	}
	if len(pm.Content) > 4096 {
		return fmt.Errorf("content too long")
	}
	if len(pm.Email) > 320 {
		return fmt.Errorf("email too long")
	}
	if len(pm.Addrs) > 20 {
		return fmt.Errorf("too many addrs")
	}
	for _, a := range pm.Addrs {
		if len(a) > 256 {
			return fmt.Errorf("addr too long")
		}
	}

	return nil
}

// getPeerID extracts the peer ID from the request, checking the
// X-Goop-Peer-ID header first, then the peer_id query parameter.
func getPeerID(r *http.Request) string {
	if id := r.Header.Get("X-Goop-Peer-ID"); id != "" {
		return id
	}
	return r.URL.Query().Get("peer_id")
}
