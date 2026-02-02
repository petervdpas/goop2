// internal/rendezvous/server.go
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
	"sort"
	"strings"
	"sync"
	"time"

	"goop/internal/proto"
	"goop/internal/util"
)

//go:embed assets/index.html assets/style.css
var embedded embed.FS

type Server struct {
	addr string
	srv  *http.Server

	mu      sync.Mutex
	clients map[chan []byte]struct{}

	// simple in-memory peer view for the web page
	peers map[string]peerRow

	// log buffer for web UI
	logMu   sync.Mutex
	logs    []string
	maxLogs int

	tmpl  *template.Template
	style []byte

	templateStore *TemplateStore
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
}

type indexVM struct {
	Title          string
	Endpoint       string
	ConnectURLs    []string
	PeerCount      int
	Peers          []peerRow
	Now            string
	StoreTemplates []StoreMeta
}

func New(addr string, templatesDir string) *Server {
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

	t := template.New("index.html").Funcs(funcs)

	// Parse AFTER Funcs are registered.
	tmpl, err := t.ParseFS(embedded, "assets/index.html")
	if err != nil {
		panic(err)
	}

	css, err := embedded.ReadFile("assets/style.css")
	if err != nil {
		// If you prefer: panic(err)
		css = []byte("/* missing style.css */")
	}

	s := &Server{
		addr:    addr,
		clients: map[chan []byte]struct{}{},
		peers:   map[string]peerRow{},
		logs:    make([]string, 0, 500),
		maxLogs: 500,
		tmpl:    tmpl,
		style:   css,
	}

	s.templateStore = NewTemplateStore(templatesDir)

	return s
}

func (s *Server) Start(ctx context.Context) error {
	// Start peer cleanup goroutine
	go s.cleanupStalePeers(ctx)

	mux := http.NewServeMux()

	// Human + machine endpoints
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/peers.json", s.handlePeersJSON)
	mux.HandleFunc("/logs.json", s.handleLogsJSON)

	// Static (embedded) CSS
	mux.HandleFunc("/assets/style.css", s.handleStyle)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})

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
		s.addClient(ch)
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

		var pm proto.PresenceMsg
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&pm); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}

		if err := validatePresence(pm); err != nil {
			http.Error(w, "bad message: "+err.Error(), http.StatusBadRequest)
			return
		}

		// normalize timestamp if caller didn't set it
		if pm.TS == 0 {
			pm.TS = proto.NowMillis()
		}

		// Calculate message size for tracking
		b, _ := json.Marshal(pm)
		msgSize := int64(len(b))

		// update peer snapshot for / and /peers.json
		s.upsertPeer(pm, msgSize)
		s.addLog(fmt.Sprintf("Received %s from %s: %q", pm.Type, pm.PeerID, pm.Content))

		s.broadcast(b)

		w.WriteHeader(http.StatusNoContent)
	})

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
	return "http://" + s.addr
}

// connectURLs returns HTTP URLs that remote peers can use to reach this
// rendezvous server. It discovers non-loopback IPv4 addresses and pairs
// them with the server's listen port.
func (s *Server) connectURLs() []string {
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

func (s *Server) addClient(ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[ch] = struct{}{}
}

func (s *Server) removeClient(ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, ch)
	close(ch)
}

func (s *Server) broadcast(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msgSize := int64(len(b))

	// Attribute received bytes to all online peers
	for peerID, peer := range s.peers {
		peer.BytesReceived += msgSize
		s.peers[peerID] = peer
	}

	for ch := range s.clients {
		select {
		case ch <- b:
		default:
			// slow client; drop message rather than blocking server
		}
	}
}

func (s *Server) upsertPeer(pm proto.PresenceMsg, msgSize int64) {
	now := time.Now().UnixMilli()

	s.mu.Lock()
	defer s.mu.Unlock()

	// If peer sends offline, remove them immediately
	if pm.Type == proto.TypeOffline {
		delete(s.peers, pm.PeerID)
		s.addLog(fmt.Sprintf("Peer went offline and removed: %s", pm.PeerID))
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

	s.peers[pm.PeerID] = peerRow{
		PeerID:        pm.PeerID,
		Type:          pm.Type,
		Content:       pm.Content,
		Email:         pm.Email,
		AvatarHash:    pm.AvatarHash,
		TS:            pm.TS,
		LastSeen:      now,
		BytesSent:     bytesSent,
		BytesReceived: bytesReceived,
	}
}

func (s *Server) snapshotPeers() []peerRow {
	s.mu.Lock()
	defer s.mu.Unlock()

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

			for peerID, peer := range s.peers {
				if peer.LastSeen < staleThreshold {
					delete(s.peers, peerID)
					s.addLog(fmt.Sprintf("Removed stale peer: %s (last seen: %v)", peerID, time.UnixMilli(peer.LastSeen).Format("15:04:05")))
				}
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

func (s *Server) handlePeersJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	peers := s.snapshotPeers()

	var storeTemplates []StoreMeta
	if s.templateStore != nil {
		storeTemplates = s.templateStore.List()
	}

	w.Header().Set("content-type", "text/html; charset=utf-8")
	_ = s.tmpl.Execute(w, indexVM{
		Title:          "Goop² Rendezvous",
		Endpoint:       s.URL(),
		ConnectURLs:    s.connectURLs(),
		PeerCount:      len(peers),
		Peers:          peers,
		Now:            time.Now().Format("2006-01-02 15:04:05"),
		StoreTemplates: storeTemplates,
	})
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
		_, ok := s.templateStore.GetManifest(dir)
		if !ok {
			http.NotFound(w, r)
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

	return nil
}
