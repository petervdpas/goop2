package rendezvous

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
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
	"github.com/tdewolff/minify/v2"
	mincss "github.com/tdewolff/minify/v2/css"
)

//go:embed all:assets
var embedded embed.FS

const (
	maxSSEClients      = 1024 // global SSE connection limit
	maxSSEClientsPerIP = 10   // per-IP SSE connection limit
)

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

	tmpl         *template.Template
	adminTmpl    *template.Template
	pricesTmpl   *template.Template
	docsTmpl     *template.Template
	storeTmpl    *template.Template
	creditsTmpl  *template.Template
	registerTmpl *template.Template
	style        []byte
	docsCSS      []byte
	favicon      []byte
	splash       []byte
	docsSite     *DocSite

	templateStore  *TemplateStore
	peerDB         *peerDB         // nil when persistence is disabled
	credits             CreditProvider  // default: NoCredits{}
	registration        *RemoteRegistrationProvider // nil = use built-in registration
	email               *RemoteEmailProvider        // nil = email service not configured

	// Circuit relay v2
	relayHost    host.Host  // nil when relay is disabled
	relayInfo    *RelayInfo // nil when relay is disabled
	relayPort    int
	relayKeyFile string

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
	PeerID        string `json:"peer_id"`
	Type          string `json:"type"`
	Content       string `json:"content"`
	Email         string `json:"email,omitempty"`
	AvatarHash    string `json:"avatar_hash,omitempty"`
	TS            int64  `json:"ts"`
	LastSeen      int64  `json:"last_seen"`
	BytesSent     int64  `json:"bytes_sent"`
	BytesReceived int64  `json:"bytes_received"`
	Verified      bool   `json:"verified"`
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
	minRegistrationAPI = 1
	minCreditsAPI      = 1
	minEmailAPI        = 1
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

type adminVM struct {
	Title      string
	PeerCount  int
	Peers      []peerRow
	Now        string
	HasCredits bool
	Services   []serviceStatus
}

type pricesVM struct {
	Title string
}

type docsVM struct {
	Title   string
	Pages   []DocPage
	Current *DocPage
	Prev    *DocPage
	Next    *DocPage
}

func New(addr string, templatesDirs []string, peerDBPath string, adminPassword string, externalURL string, relayPort int, relayKeyFile string) *Server {
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

	pricesTmpl, err := template.New("prices.html").Funcs(funcs).ParseFS(embedded, "assets/prices.html")
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
		externalURL:   strings.TrimRight(externalURL, "/"),
		adminPassword: adminPassword,
		clients:       map[chan []byte]struct{}{},
		clientIPs:            map[chan []byte]string{},
		peers:                map[string]peerRow{},
		logs:                 make([]string, 0, 500),
		maxLogs:              500,
		tmpl:                 tmpl,
		adminTmpl:            adminTmpl,
		pricesTmpl:           pricesTmpl,
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

	s.templateStore = NewTemplateStore(templatesDirs)
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
// Checks in-memory peers first, then falls back to peerDB.
func (s *Server) GetEmailForPeer(peerID string) string {
	s.mu.Lock()
	if p, ok := s.peers[peerID]; ok && p.Email != "" {
		s.mu.Unlock()
		return p.Email
	}
	s.mu.Unlock()

	if s.peerDB != nil {
		return s.peerDB.lookupEmail(peerID)
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

func (s *Server) Start(ctx context.Context) error {
	// Start circuit relay v2 host if configured
	if s.relayPort > 0 {
		rh, ri, err := StartRelay(s.relayPort, s.relayKeyFile, s.externalURL)
		if err != nil {
			return fmt.Errorf("start relay: %w", err)
		}
		s.relayHost = rh
		s.relayInfo = ri

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
	mux.HandleFunc("/admin/prices", s.handlePrices)
	mux.HandleFunc("/peers.json", s.handlePeersJSON)
	mux.HandleFunc("/logs.json", s.handleLogsJSON)
	mux.HandleFunc("/registrations.json", s.handleRegistrationsJSON)

	// Registration endpoints
	if s.registration != nil {
		// Remote registration service — proxy /verify and /api/reg/*
		s.registration.RegisterRoutes(mux)
		// /register is always served locally (form + POST proxy)
		mux.HandleFunc("/register", s.handleRegisterRemote)
	} else {
		// Built-in registration handlers
		mux.HandleFunc("/register", s.handleRegister)
		mux.HandleFunc("/verify", s.handleVerify)
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
			if pm.Email == "" {
				isRegistered = false
			} else {
				isRegistered = s.registration.IsEmailVerified(pm.Email)
			}
		}

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
		s.upsertPeer(pm, msgSize, isRegistered)
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

	// Template store API
	if s.templateStore != nil {
		mux.HandleFunc("/api/templates", s.handleTemplateList)
		mux.HandleFunc("/api/templates/", s.handleTemplateRoutes)
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

func (s *Server) upsertPeer(pm proto.PresenceMsg, msgSize int64, verified bool) {
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
		PeerID:        pm.PeerID,
		Type:          pm.Type,
		Content:       pm.Content,
		Email:         pm.Email,
		AvatarHash:    pm.AvatarHash,
		TS:            pm.TS,
		LastSeen:      now,
		BytesSent:     bytesSent,
		BytesReceived: bytesReceived,
		Verified:      verified,
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
	if s.templateStore != nil {
		list := s.templateStore.List()
		hasStore = len(list) > 0
		storeCount = len(list)
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

	var templates []storeTemplateVM
	if s.templateStore != nil {
		for _, meta := range s.templateStore.List() {
			info := s.credits.TemplateStoreInfo(r, meta)
			templates = append(templates, storeTemplateVM{
				Meta:       meta,
				PriceLabel: info.PriceLabel,
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
		if peerID != "" {
			reqURL += "?peer_id=" + url.QueryEscape(peerID)
		}

		resp, err := cp.client.Get(reqURL)
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

	w.Header().Set("content-type", "text/html; charset=utf-8")
	_ = s.adminTmpl.Execute(w, adminVM{
		Title:      "Goop² Admin",
		PeerCount:  len(peers),
		Peers:      peers,
		Now:        time.Now().Format("2006-01-02 15:04:05"),
		HasCredits: hasCredits,
		Services:   services,
	})
}

func (s *Server) handlePrices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	_ = s.pricesTmpl.Execute(w, pricesVM{
		Title: "Template Prices — Goop²",
	})
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
	resp, err := client.Get(strings.TrimRight(baseURL, "/") + "/healthz")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
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

func (s *Server) handleTemplateList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(s.templateStore.List())
}

func (s *Server) handleTemplateRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// /api/templates/<dir>/manifest.json  or  /api/templates/<dir>/bundle
	path := strings.TrimPrefix(r.URL.Path, "/api/templates/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	dir, action := parts[0], parts[1]

	switch action {
	case "manifest.json":
		meta, ok := s.templateStore.GetManifest(dir)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(meta)

	case "bundle":
		meta, ok := s.templateStore.GetManifest(dir)
		if !ok {
			http.NotFound(w, r)
			return
		}
		// Registration gate: require verified email for paid template downloads
		if s.registration != nil && s.registration.RegistrationRequired() {
			peerID := getPeerID(r)
			if peerID == "" {
				http.Error(w, "registration required — provide peer_id", http.StatusForbidden)
				return
			}
			s.mu.Lock()
			peer, known := s.peers[peerID]
			s.mu.Unlock()
			if !known || peer.Email == "" || !s.registration.IsEmailVerified(peer.Email) {
				http.Error(w, "registration required", http.StatusForbidden)
				return
			}
		}
		if !s.credits.TemplateAccessAllowed(r, meta) {
			http.Error(w, "payment required", http.StatusPaymentRequired)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", dir+".tar.gz"))
		if err := s.templateStore.WriteBundle(w, dir); err != nil {
			log.Printf("template bundle write error: %v", err)
		}

	default:
		http.NotFound(w, r)
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
	body := fmt.Sprintf(`{"email":%q}`, email)
	resp, err := http.Post(
		s.registration.baseURL+"/api/reg/register",
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		vm.Error = "Registration service unavailable"
		vm.Email = email
		s.renderRegister(w, vm)
		return
	}
	defer resp.Body.Close()

	var result struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode/100 != 2 || !result.OK {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = result.Message
		}
		if errMsg == "" {
			errMsg = "Registration failed"
		}
		vm.Error = errMsg
		vm.Email = email
		s.renderRegister(w, vm)
		return
	}

	vm.Success = true
	vm.Email = email
	s.renderRegister(w, vm)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if s.registerTmpl == nil {
		http.Error(w, "registration not available", http.StatusNotFound)
		return
	}

	if s.peerDB == nil {
		http.Error(w, "registration requires database", http.StatusServiceUnavailable)
		return
	}

	vm := registerVM{Title: "Register — Goop² Rendezvous"}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			vm.Error = "Invalid form data"
			s.renderRegister(w, vm)
			return
		}

		email := strings.TrimSpace(r.FormValue("email"))
		if email == "" {
			vm.Error = "Email is required"
			vm.Email = email
			s.renderRegister(w, vm)
			return
		}

		// Basic email validation
		if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
			vm.Error = "Please enter a valid email address"
			vm.Email = email
			s.renderRegister(w, vm)
			return
		}

		// Check if already verified
		if s.peerDB.isEmailVerified(email) {
			vm.Error = "This email is already registered and verified"
			vm.Email = email
			s.renderRegister(w, vm)
			return
		}

		// Generate verification token
		token := generateToken()

		// Save registration
		if err := s.peerDB.createRegistration(email, token); err != nil {
			vm.Error = "Failed to create registration"
			vm.Email = email
			s.renderRegister(w, vm)
			return
		}

		// Build verification URL
		baseURL := s.externalURL
		if baseURL == "" {
			baseURL = "http://" + r.Host
		}
		verifyURL := baseURL + "/verify?token=" + token

		// Log the verification link to console (SMTP is handled by
		// the standalone registration service when configured)
		log.Printf("────────────────────────────────────────────────────────")
		log.Printf("VERIFICATION LINK for %s:", email)
		log.Printf("   %s", verifyURL)
		log.Printf("────────────────────────────────────────────────────────")

		s.addLog(fmt.Sprintf("Registration requested: %s", email))

		vm.Success = true
		vm.Email = email
		s.renderRegister(w, vm)
		return
	}

	// GET: show registration form
	s.renderRegister(w, vm)
}

func (s *Server) renderRegister(w http.ResponseWriter, vm registerVM) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.registerTmpl.Execute(w, vm); err != nil {
		log.Printf("register template error: %v", err)
	}
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.peerDB == nil {
		http.Error(w, "verification requires database", http.StatusServiceUnavailable)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	email, ok := s.peerDB.verifyRegistration(token)
	if !ok {
		http.Error(w, "invalid or expired token", http.StatusBadRequest)
		return
	}

	s.addLog(fmt.Sprintf("Email verified: %s", email))
	log.Printf("Email verified: %s", email)

	// Show success page
	if s.registerTmpl != nil {
		vm := registerVM{
			Title:    "Verified — Goop² Rendezvous",
			Email:    email,
			Success:  true,
			Verified: true,
		}
		s.renderRegister(w, vm)
		return
	}

	// Fallback text response
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Email %s verified successfully!\n", email)
}

func (s *Server) handleRegistrationsJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	if s.peerDB == nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
		return
	}

	regs, err := s.peerDB.listRegistrations()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(regs)
}

// generateToken creates a random URL-safe token.
func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based token
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)
}
