package rendezvous

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/proto"
	"github.com/petervdpas/goop2/internal/util"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
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
	peersDirty  bool       // set when peers map changes; cleared by snapshotPeers
	cachedPeers []peerRow  // sorted snapshot cache

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
	creditsTmpl  *template.Template
	registerTmpl *template.Template
	style        []byte
	docsCSS      []byte
	favicon      []byte
	splash       []byte
	docsSite     *DocSite

	peerDB         *peerDB         // nil when persistence is disabled
	credits             CreditProvider  // default: NoCredits{}
	registration        *RemoteRegistrationProvider // nil = use built-in registration
	email               *RemoteEmailProvider        // nil = email service not configured
	templates           *RemoteTemplatesProvider    // nil = templates service not configured
	localTemplates      *LocalTemplateStore         // nil = no local template store

	// Circuit relay v2
	relayHost    host.Host  // nil when relay is disabled
	relayInfo    *RelayInfo // nil when relay is disabled
	relayPort    int
	relayKeyFile string
	relayTiming  RelayTimingConfig

	// per-IP rate limiter for /publish
	rateMu     sync.Mutex
	rateWindow map[string]*rateBucket
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
	PeerID         string   `json:"peer_id"`
	Type           string   `json:"type"`
	Content        string   `json:"content"`
	Email          string   `json:"email,omitempty"`
	AvatarHash     string   `json:"avatar_hash,omitempty"`
	ActiveTemplate string   `json:"active_template,omitempty"`
	Addrs          []string `json:"addrs,omitempty"`
	TS             int64    `json:"ts"`
	LastSeen       int64    `json:"last_seen"`
	BytesSent      int64    `json:"bytes_sent"`
	BytesReceived  int64    `json:"bytes_received"`
	Verified       bool     `json:"verified"`

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

type creditPackVM struct {
	Name   string
	Label  string
	Amount int
}

type creditsVM struct {
	Title                string
	HasAdmin             bool
	PeerID               string
	Balance              int
	CreditPacks          []creditPackVM
	RegistrationRequired bool
	RegistrationCredits  int
	HasCredits           bool
}

// Minimum API versions that this build of goop2 requires.
const (
	minRegistrationAPI  = 1
	minCreditsAPI       = 1
	minEmailAPI         = 1
	minTemplatesAPI     = 1
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

	creditsTmpl, err := template.New("credits.html").Funcs(funcs).ParseFS(embedded, "assets/credits.html")
	if err != nil {
		creditsTmpl = nil
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
		addr:          addr,
		externalURL:   util.NormalizeURL(externalURL),
		adminPassword: adminPassword,
		clients:       map[chan []byte]struct{}{},
		clientIPs:            map[chan []byte]string{},
		peers:                map[string]peerRow{},
		logs:                 make([]string, 0, 500),
		maxLogs:              500,
		relayLogs:            make([]string, 0, 500),
		maxRelayLogs:         500,
		tmpl:                 tmpl,
		adminTmpl:            adminTmpl,
		docsTmpl:             docsTmpl,
		storeTmpl:            storeTmpl,
		creditsTmpl:          creditsTmpl,
		registerTmpl:         registerTmpl,
		style:                css,
		docsCSS:              docsCSSData,
		favicon:              faviconData,
		splash:               splashData,
		docsSite:             newDocSite(),
		relayPort:            relayPort,
		relayKeyFile:         relayKeyFile,
		relayTiming:          relayTiming,
		rateWindow:           map[string]*rateBucket{},
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

	// Relay info endpoint (returns 404 when relay is disabled)
	mux.HandleFunc("/relay", func(w http.ResponseWriter, r *http.Request) {
		handleRelayInfo(w, r, s.relayInfo)
	})


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
		s.upsertPeer(pm, msgSize, isRegistered, peerToken)
		s.addLog(fmt.Sprintf("Received %s from %s: %q (verified=%v)", pm.Type, pm.PeerID, pm.Content, isRegistered))
		s.broadcast(b)

		w.WriteHeader(http.StatusNoContent)
	})

	// Store page
	mux.HandleFunc("/store", s.handleStore)

	// Credits page
	mux.HandleFunc("/credits", s.handleCredits)

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

func (s *Server) broadcast(b []byte) {
	s.mu.Lock()

	msgSize := int64(len(b))

	// Attribute received bytes to all online peers
	for peerID, peer := range s.peers {
		peer.BytesReceived += msgSize
		s.peers[peerID] = peer
	}
	s.peersDirty = true

	// Copy client channels so we can send outside the lock
	clients := make([]chan []byte, 0, len(s.clients))
	for ch := range s.clients {
		clients = append(clients, ch)
	}
	s.mu.Unlock()

	for _, ch := range clients {
		select {
		case ch <- b:
		default:
			// slow client; drop message rather than blocking server
		}
	}
}

func (s *Server) upsertPeer(pm proto.PresenceMsg, msgSize int64, verified bool, verificationToken string) {
	now := time.Now().UnixMilli()

	s.mu.Lock()
	defer s.mu.Unlock()

	// If peer sends offline, remove them immediately
	if pm.Type == proto.TypeOffline {
		delete(s.peers, pm.PeerID)
		s.peersDirty = true
		s.addLog(fmt.Sprintf("Peer went offline and removed: %s", pm.PeerID))
		if s.peerDB != nil {
			go s.peerDB.remove(pm.PeerID)
		}
		return
	}

	// Preserve existing byte counts
	existing, exists := s.peers[pm.PeerID]
	bytesSent := msgSize
	bytesReceived := int64(0)
	if exists {
		bytesSent += existing.BytesSent
		bytesReceived = existing.BytesReceived
	}

	row := peerRow{
		PeerID:            pm.PeerID,
		Type:              pm.Type,
		Content:           pm.Content,
		Email:             pm.Email,
		AvatarHash:        pm.AvatarHash,
		ActiveTemplate:    pm.ActiveTemplate,
		Addrs:             pm.Addrs,
		TS:                pm.TS,
		LastSeen:          now,
		BytesSent:         bytesSent,
		BytesReceived:     bytesReceived,
		Verified:          verified,
		verificationToken: verificationToken,
	}
	s.peers[pm.PeerID] = row
	s.peersDirty = true

	if s.peerDB != nil {
		go s.peerDB.upsert(row)
	}
}

func (s *Server) snapshotPeers() []peerRow {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.peersDirty && s.cachedPeers != nil {
		return s.cachedPeers
	}

	out := make([]peerRow, 0, len(s.peers))
	for _, v := range s.peers {
		out = append(out, v)
	}

	rank := func(t string) int {
		switch t {
		case proto.TypeOnline:
			return 0
		case proto.TypeUpdate:
			return 1
		case proto.TypeOffline:
			return 2
		default:
			return 9
		}
	}

	sort.Slice(out, func(i, j int) bool {
		ri, rj := rank(out[i].Type), rank(out[j].Type)
		if ri != rj {
			return ri < rj
		}
		return out[i].LastSeen > out[j].LastSeen
	})

	s.cachedPeers = out
	s.peersDirty = false
	return out
}

// cleanupStalePeers removes peers that haven't been seen in 30+ seconds
func (s *Server) cleanupStalePeers(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now().UnixMilli()
			staleThreshold := now - (30 * 1000) // 30 seconds

			removed := false
			for peerID, peer := range s.peers {
				if peer.LastSeen < staleThreshold {
					delete(s.peers, peerID)
					removed = true
					s.addLog(fmt.Sprintf("Removed stale peer: %s (last seen: %v)", peerID, time.UnixMilli(peer.LastSeen).Format("15:04:05")))
				}
			}
			if removed {
				s.peersDirty = true
			}
			s.mu.Unlock()

			if s.peerDB != nil {
				go s.peerDB.cleanupStale(staleThreshold)
			}

			// Clean up stale rate limiter entries
			s.cleanupRateLimiter()
		}
	}
}

// loadPeersFromDB restores peer state from SQLite on startup.
func (s *Server) loadPeersFromDB() {
	rows, err := s.peerDB.loadAll()
	if err != nil {
		log.Printf("peerdb: load error: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range rows {
		s.peers[r.PeerID] = r
	}
	if len(rows) > 0 {
		s.peersDirty = true
		log.Printf("peerdb: loaded %d peers from database", len(rows))
	}
}

// syncFromDB periodically merges peer state from SQLite so that peers
// registered by other instances become visible.
func (s *Server) syncFromDB(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	var lastKnownMax int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Quick check: skip full load if DB hasn't changed
			dbMax, dbCount, err := s.peerDB.maxLastSeenAndCount()
			if err != nil {
				continue
			}
			s.mu.Lock()
			memCount := len(s.peers)
			s.mu.Unlock()
			if dbMax == lastKnownMax && dbCount == memCount {
				continue
			}
			lastKnownMax = dbMax

			rows, err := s.peerDB.loadAll()
			if err != nil {
				continue
			}

			s.mu.Lock()
			changed := false
			dbPeers := make(map[string]struct{}, len(rows))
			for _, r := range rows {
				dbPeers[r.PeerID] = struct{}{}
				existing, ok := s.peers[r.PeerID]
				if !ok || r.LastSeen > existing.LastSeen {
					s.peers[r.PeerID] = r
					changed = true
				}
			}
			// Remove peers that were cleaned up by another instance
			for peerID := range s.peers {
				if _, inDB := dbPeers[peerID]; !inDB {
					delete(s.peers, peerID)
					changed = true
				}
			}
			if changed {
				s.peersDirty = true
			}
			s.mu.Unlock()
		}
	}
}

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

func (s *Server) handlePeersJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(s.snapshotPeers())
}

func (s *Server) handleLogsJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	s.logMu.Lock()
	logs := make([]string, len(s.logs))
	copy(logs, s.logs)
	s.logMu.Unlock()

	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(logs)
}

type relayPeerJSON struct {
	PeerID string `json:"peer_id"`
	Name   string `json:"name,omitempty"`
	Addr   string `json:"addr"`
	Dir    string `json:"dir"` // "inbound" or "outbound"
}

func dirString(d network.Direction) string {
	switch d {
	case network.DirInbound:
		return "inbound"
	case network.DirOutbound:
		return "outbound"
	default:
		return "unknown"
	}
}

type relayStatusJSON struct {
	Peers []relayPeerJSON `json:"peers"`
	Logs  []string        `json:"logs"`
}

func (s *Server) handleRelayStatusJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	// Build peer ID → name map from rendezvous peers
	s.mu.Lock()
	peerNames := make(map[string]string, len(s.peers))
	for _, p := range s.peers {
		if p.Content != "" {
			peerNames[p.PeerID] = p.Content
		}
	}
	s.mu.Unlock()

	var result relayStatusJSON

	if s.relayHost != nil {
		for _, pid := range s.relayHost.Network().Peers() {
			conns := s.relayHost.Network().ConnsToPeer(pid)
			for _, c := range conns {
				result.Peers = append(result.Peers, relayPeerJSON{
					PeerID: pid.String(),
					Name:   peerNames[pid.String()],
					Addr:   c.RemoteMultiaddr().String(),
					Dir:    dirString(c.Stat().Direction),
				})
			}
		}
	}

	s.relayLogMu.Lock()
	result.Logs = make([]string, len(s.relayLogs))
	copy(result.Logs, s.relayLogs)
	s.relayLogMu.Unlock()

	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(result)
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

// handleDiagPeer queries a peer's diagnostic info via the relay host connection.
// Admin-only. The relay host opens a /goop/diag/1.0.0 stream to the peer and
// returns the diagnostic snapshot.
func (s *Server) handleDiagPeer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	if s.relayHost == nil {
		http.Error(w, "relay not enabled", http.StatusServiceUnavailable)
		return
	}

	peerIDStr := r.URL.Query().Get("peer")
	if peerIDStr == "" {
		http.Error(w, "missing ?peer= parameter", http.StatusBadRequest)
		return
	}
	pid, err := peer.Decode(peerIDStr)
	if err != nil {
		http.Error(w, "invalid peer ID", http.StatusBadRequest)
		return
	}

	// Check if the relay host has a connection to this peer.
	conns := s.relayHost.Network().ConnsToPeer(pid)
	if len(conns) == 0 {
		http.Error(w, "peer not connected to relay", http.StatusNotFound)
		return
	}

	// Gather relay-side info about this peer.
	now := time.Now()
	var relayConns []map[string]any
	for _, c := range conns {
		age := now.Sub(c.Stat().Opened)
		relayConns = append(relayConns, map[string]any{
			"addr":    c.RemoteMultiaddr().String(),
			"dir":     dirString(c.Stat().Direction),
			"age":     age.Truncate(time.Second).String(),
			"streams": len(c.GetStreams()),
		})
	}

	// Resolve peer name from heartbeat data.
	s.mu.Lock()
	peerName := ""
	if p, ok := s.peers[peerIDStr]; ok {
		peerName = p.Content
	}
	s.mu.Unlock()

	// Open a diagnostic stream to the peer via the relay connection.
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stream, err := s.relayHost.NewStream(ctx, pid, protocol.ID("/goop/diag/1.0.0"))
	if err != nil {
		// Peer doesn't support the protocol or stream failed.
		// Return what we know from the relay side.
		result := map[string]any{
			"peer_id":      peerIDStr,
			"name":         peerName,
			"relay_view":   relayConns,
			"stream_error": err.Error(),
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	defer stream.Close()

	// Read the peer's diagnostic snapshot.
	var peerDiag map[string]any
	if err := json.NewDecoder(io.LimitReader(stream, 64*1024)).Decode(&peerDiag); err != nil {
		peerDiag = map[string]any{"decode_error": err.Error()}
	}

	// Merge relay-side view into the response.
	peerDiag["name"] = peerName
	peerDiag["relay_view"] = relayConns

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(peerDiag)
}

// handlePulse tells a target peer to refresh its relay reservation.
// Any peer can request this — when they can't reach a target peer, they
// call POST /api/pulse?peer=<id> and the rendezvous opens a relay-refresh
// stream to the target via the relay host.
func (s *Server) handlePulse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.relayHost == nil {
		http.Error(w, "relay not enabled", http.StatusServiceUnavailable)
		return
	}

	peerIDStr := r.URL.Query().Get("peer")
	if peerIDStr == "" {
		http.Error(w, "missing ?peer= parameter", http.StatusBadRequest)
		return
	}
	pid, err := peer.Decode(peerIDStr)
	if err != nil {
		http.Error(w, "invalid peer ID", http.StatusBadRequest)
		return
	}

	// Check if the relay host has a connection to this peer.
	conns := s.relayHost.Network().ConnsToPeer(pid)
	if len(conns) == 0 {
		http.Error(w, "peer not connected to relay", http.StatusNotFound)
		return
	}

	s.relayAddLog(fmt.Sprintf("pulse: refreshing relay for %s (requested by %s)", pid.String()[:16]+"...", extractIP(r.RemoteAddr)))

	// Open a relay-refresh stream to the target peer.
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	stream, err := s.relayHost.NewStream(ctx, pid, protocol.ID("/goop/relay-refresh/1.0.0"))
	if err != nil {
		s.relayAddLog(fmt.Sprintf("pulse: stream to %s failed: %v", pid.String()[:16]+"...", err))
		http.Error(w, "pulse stream failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer stream.Close()

	// Read the result from the peer.
	var result map[string]any
	if err := json.NewDecoder(io.LimitReader(stream, 4096)).Decode(&result); err != nil {
		result = map[string]any{"ok": false, "error": err.Error()}
	}

	s.relayAddLog(fmt.Sprintf("pulse: %s responded: %v", pid.String()[:16]+"...", result))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(result)
}

// serviceLogEntry is a single log line from a microservice.
type serviceLogEntry struct {
	Service string `json:"service"`
	Message string `json:"message"`
}

func (s *Server) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	type svcDef struct {
		name string
		url  string
	}
	var svcs []svcDef
	if s.registration != nil {
		svcs = append(svcs, svcDef{"registration", s.registration.baseURL})
	}
	if cp, ok := s.credits.(*RemoteCreditProvider); ok {
		svcs = append(svcs, svcDef{"credits", cp.baseURL})
	}
	if s.email != nil {
		svcs = append(svcs, svcDef{"email", s.email.baseURL})
	}
	if s.templates != nil {
		svcs = append(svcs, svcDef{"templates", s.templates.baseURL})
	}

	type result struct {
		name    string
		entries []string
	}
	ch := make(chan result, len(svcs))
	client := &http.Client{Timeout: 2 * time.Second}

	for _, svc := range svcs {
		go func(name, baseURL string) {
			resp, err := client.Get(baseURL + "/api/logs")
			if err != nil {
				ch <- result{name, nil}
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				ch <- result{name, nil}
				return
			}
			var lines []string
			json.NewDecoder(resp.Body).Decode(&lines)
			ch <- result{name, lines}
		}(svc.name, svc.url)
	}

	var all []serviceLogEntry
	for range svcs {
		res := <-ch
		for _, line := range res.entries {
			all = append(all, serviceLogEntry{Service: res.name, Message: line})
		}
	}

	// Sort by message (which starts with timestamp from Go's log package)
	sort.Slice(all, func(i, j int) bool {
		return all[i].Message < all[j].Message
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(all)
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

func (s *Server) handleCredits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.creditsTmpl == nil {
		http.NotFound(w, r)
		return
	}

	_, hasCredits := s.credits.(*RemoteCreditProvider)

	regRequired := false
	if s.registration != nil {
		regRequired = s.registration.RegistrationRequired()
	}

	vm := creditsVM{
		Title:                "Credits — Goop²",
		HasAdmin:             s.adminPassword != "",
		HasCredits:           hasCredits,
		RegistrationRequired: regRequired,
		RegistrationCredits:  s.grantAmount(),
	}

	// Fetch credit data from service
	cp, ok := s.credits.(*RemoteCreditProvider)
	if ok {
		peerID := getPeerID(r)
		vm.PeerID = peerID

		reqURL := cp.baseURL + "/api/credits/store-data"
		var token string
		if peerID != "" {
			if email := cp.emailResolver(peerID); email != "" {
				reqURL += "?email=" + url.QueryEscape(email)
			}
			if cp.tokenResolver != nil {
				token = cp.tokenResolver(peerID)
			}
		}

		req, _ := http.NewRequest("GET", reqURL, nil)
		if token != "" {
			req.Header.Set("X-Verification-Token", token)
		}
		resp, err := cp.client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			var data struct {
				Balance    int `json:"balance"`
				CreditPacks []struct {
					Amount int    `json:"amount"`
					Name   string `json:"name"`
					Label  string `json:"label"`
				} `json:"credit_packs"`
			}
			if json.NewDecoder(resp.Body).Decode(&data) == nil {
				vm.Balance = data.Balance
				for _, pk := range data.CreditPacks {
					vm.CreditPacks = append(vm.CreditPacks, creditPackVM{
						Name:   pk.Name,
						Label:  pk.Label,
						Amount: pk.Amount,
					})
				}
			}
		}
	}

	w.Header().Set("content-type", "text/html; charset=utf-8")
	_ = s.creditsTmpl.Execute(w, vm)
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

// checkServiceHealth pings a service's /healthz endpoint.
func checkServiceHealth(baseURL string) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(util.NormalizeURL(baseURL) + "/healthz")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// topologyPath returns the topology endpoint path for a given service name.
func topologyPath(name string) string {
	switch strings.ToLower(name) {
	case "credits":
		return "/api/credits/topology"
	case "registration":
		return "/api/reg/topology"
	case "email":
		return "/api/email/topology"
	case "templates":
		return "/api/templates/topology"
	default:
		return "/api/" + strings.ToLower(name) + "/topology"
	}
}

// fetchTopology calls a service's topology endpoint and decodes the response.
func fetchTopology(baseURL, serviceName string) (topologyInfo, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(util.NormalizeURL(baseURL) + topologyPath(serviceName))
	if err != nil {
		return topologyInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return topologyInfo{}, fmt.Errorf("status %d", resp.StatusCode)
	}
	var topo topologyInfo
	if err := json.NewDecoder(resp.Body).Decode(&topo); err != nil {
		return topologyInfo{}, err
	}
	return topo, nil
}

// validateChain checks the topology data for common misconfiguration issues.
func validateChain(topologies []topologyInfo, services []serviceStatus) []string {
	var issues []string

	// Build a map of service name → URL (as configured in the rendezvous)
	rvURLs := make(map[string]string)
	for _, svc := range services {
		rvURLs[strings.ToLower(svc.Name)] = svc.URL
	}

	for _, topo := range topologies {
		for _, dep := range topo.Dependencies {
			if dep.URL == "" {
				// Check if the rendezvous has this service configured
				if rvURL, ok := rvURLs[dep.Name]; ok && rvURL != "" {
					issues = append(issues, fmt.Sprintf(
						"%s service has no %s URL configured — but the rendezvous connects to %s at %s",
						topo.Service, dep.Name, dep.Name, rvURL))
				} else {
					issues = append(issues, fmt.Sprintf(
						"%s service has no %s URL configured", topo.Service, dep.Name))
				}
			} else if !dep.OK {
				issues = append(issues, fmt.Sprintf(
					"%s → %s (%s): %s", topo.Service, dep.Name, dep.URL, dep.Error))
			}
		}
	}

	// Specific critical check: credits must know about templates for pricing to work
	for _, topo := range topologies {
		if topo.Service == "credits" {
			for _, dep := range topo.Dependencies {
				if dep.Name == "templates" && dep.URL == "" {
					issues = append(issues, "Credits service has no templates_url — all templates will appear free (price=0)")
				}
			}
		}
	}

	return issues
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

// ─── Registration handlers ───

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

