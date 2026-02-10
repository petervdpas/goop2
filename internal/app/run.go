package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/petervdpas/goop2/internal/avatar"
	"github.com/petervdpas/goop2/internal/chat"
	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/content"
	"github.com/petervdpas/goop2/internal/docs"
	"github.com/petervdpas/goop2/internal/group"
	luapkg "github.com/petervdpas/goop2/internal/lua"
	"github.com/petervdpas/goop2/internal/p2p"
	"github.com/petervdpas/goop2/internal/proto"
	"github.com/petervdpas/goop2/internal/realtime"
	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
	"github.com/petervdpas/goop2/internal/util"
	"github.com/petervdpas/goop2/internal/viewer"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

type Options struct {
	PeerDir   string
	CfgPath   string
	Cfg       config.Config
	BridgeURL string
	Progress  func(step, total int, label string)
}

type runtime struct {
	node  *p2p.Node
	peers *state.PeerTable
}

func Run(ctx context.Context, opt Options) error {
	logBuf := viewer.NewLogBuffer(800)
	log.SetOutput(logBuf)

	logBanner(opt.PeerDir, opt.CfgPath)

	return runPeer(ctx, runPeerOpts{
		PeerDir:   opt.PeerDir,
		CfgPath:   opt.CfgPath,
		Cfg:       opt.Cfg,
		Logs:      logBuf,
		BridgeURL: opt.BridgeURL,
		Progress:  opt.Progress,
	})
}

type runPeerOpts struct {
	PeerDir   string
	CfgPath   string
	Cfg       config.Config
	Logs      *viewer.LogBuffer
	BridgeURL string
	Progress  func(step, total int, label string)
}

func runPeer(ctx context.Context, o runPeerOpts) error {
	cfg := o.Cfg

	emit := o.Progress
	if emit == nil {
		emit = func(int, int, string) {}
	}
	progress := func(s, t int, label string) {
		emit(s, t, label)
		time.Sleep(time.Second)
	}

	// Calculate total steps based on config.
	// Rendezvous-only: rendezvous + viewer = 2 steps.
	// Full peer: rendezvous(opt) + relay discovery + p2p node + database + services + viewer.
	step := 0
	total := 6 // relay + p2p + db + services + viewer + online
	if cfg.Presence.RendezvousHost {
		total++ // rendezvous server
	}
	if cfg.Presence.RendezvousOnly {
		total = 2 // rendezvous + viewer only
		if !cfg.Presence.RendezvousHost {
			total = 1 // viewer only
		}
	}

	// ── Rendezvous server (optional)
	var rv *rendezvous.Server
	if cfg.Presence.RendezvousHost {
		bind := cfg.Presence.RendezvousBind
		if bind == "" {
			bind = "127.0.0.1"
		}
		addr := fmt.Sprintf("%s:%d", bind, cfg.Presence.RendezvousPort)

		peerDBPath := ""
		if cfg.Presence.PeerDBPath != "" {
			peerDBPath = util.ResolvePath(o.PeerDir, cfg.Presence.PeerDBPath)
		}

		relayKeyFile := ""
		if cfg.Presence.RelayKeyFile != "" {
			relayKeyFile = util.ResolvePath(o.PeerDir, cfg.Presence.RelayKeyFile)
		}
		rv = rendezvous.New(addr, peerDBPath, cfg.Presence.AdminPassword, cfg.Presence.ExternalURL, cfg.Presence.RelayPort, relayKeyFile)

		// Wire external services (credits + registration + email + templates)
		setupCredits(rv, cfg.Presence.CreditsURL, cfg.Presence.CreditsAdminToken)
		setupRegistration(rv, cfg.Presence.RegistrationURL, cfg.Presence.RegistrationAdminToken)
		setupEmail(rv, cfg.Presence.EmailURL)
		setupTemplates(rv, cfg.Presence.TemplatesURL)

		step++
		progress(step, total, "Starting rendezvous server")

		if err := rv.Start(ctx); err != nil {
			return err
		}
		log.Println("────────────────────────────────────────────────────────")
		log.Printf("🌐 Rendezvous Server: %s", rv.URL())
		log.Printf("📊 Monitor connected peers: %s", rv.URL())
		log.Println("────────────────────────────────────────────────────────")
	}

	selfContent := func() string {
		if cfg.Profile.Label != "" {
			return cfg.Profile.Label
		}
		return "hello"
	}

	selfEmail := func() string {
		return cfg.Profile.Email
	}

	selfVideoDisabled := func() bool {
		return cfg.Viewer.VideoDisabled
	}

	if cfg.Presence.RendezvousOnly {
		log.Printf("mode: rendezvous-only")

		step++
		progress(step, total, "Starting viewer")

		// Start minimal viewer for settings access
		if cfg.Viewer.HTTPAddr != "" {
			addr, url, _ := NormalizeLocalViewer(cfg.Viewer.HTTPAddr)
			rvURL := ""
			if rv != nil {
				rvURL = rv.URL()
			}
			avatarStore := avatar.NewStore(o.PeerDir)
			go viewer.StartMinimal(addr, viewer.MinimalViewer{
				SelfLabel:      selfContent,
				SelfEmail:      selfEmail,
				CfgPath:        o.CfgPath,
				Cfg:            cfg,
				Logs:           o.Logs,
				BaseURL:        url,
				RendezvousURL:  rvURL,
				AvatarStore:    avatarStore,
			})
			log.Printf("📋 Settings viewer: %s", url)
		}

		<-ctx.Done()
		return nil
	}

	// ── Rendezvous bridges
	var rvClients []*rendezvous.Client
	if cfg.Presence.RendezvousHost {
		rvClients = append(rvClients,
			rendezvous.NewClient(fmt.Sprintf("http://127.0.0.1:%d", cfg.Presence.RendezvousPort)))
	}
	if strings.TrimSpace(cfg.Presence.RendezvousWAN) != "" {
		rvClients = append(rvClients,
			rendezvous.NewClient(strings.TrimRight(cfg.Presence.RendezvousWAN, "/")))
	}

	peers := state.NewPeerTable()

	// Fetch relay info from WAN rendezvous (if available) so we can enable
	// circuit relay transport and hole-punching for NAT traversal.
	step++
	progress(step, total, "Discovering relay")

	var relayInfo *rendezvous.RelayInfo
	for _, c := range rvClients {
		ri, err := c.FetchRelayInfo(ctx)
		if err == nil && ri != nil {
			relayInfo = ri
			log.Printf("relay: discovered relay peer %s (%d addrs)", ri.PeerID, len(ri.Addrs))
			break
		}
	}

	step++
	progress(step, total, "Creating P2P node")

	keyPath := util.ResolvePath(o.PeerDir, cfg.Identity.KeyFile)
	node, err := p2p.New(ctx, cfg.P2P.ListenPort, keyPath, peers, selfContent, selfEmail, selfVideoDisabled, relayInfo)
	if err != nil {
		return err
	}
	defer node.Close()

	node.EnableSite(util.ResolvePath(o.PeerDir, cfg.Paths.SiteRoot))

	// ── Avatar store
	avatarStore := avatar.NewStore(o.PeerDir)
	avatarCache := avatar.NewCache(o.PeerDir)
	node.EnableAvatar(avatarStore)

	step++
	progress(step, total, "Opening database")

	// Initialize SQLite database for peer data (unconditionally — needed for P2P data protocol)
	db, err := storage.Open(o.PeerDir)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	node.EnableData(db)
	log.Printf("peer id: %s", node.ID())

	step++
	progress(step, total, "Setting up services")

	// ── Chat manager
	chatMgr := chat.New(node.Host, 100) // 100 message buffer
	log.Printf("💬 Chat enabled: direct messaging via /goop/chat/1.0.0")

	// ── Lua scripting engine
	if cfg.Lua.Enabled {
		luaEngine, luaErr := luapkg.NewEngine(cfg.Lua, o.PeerDir, node.ID(), selfContent, peers)
		if luaErr != nil {
			log.Printf("WARNING: Lua engine failed to start: %v", luaErr)
		} else {
			// Phase 1: chat command dispatch
			chatMgr.SetCommandHandler(func(ctx context.Context, fromPeerID, content string, sender chat.DirectSender) {
				luaEngine.Dispatch(ctx, fromPeerID, content, luapkg.SenderFunc(func(ctx2 context.Context, toPeerID, msg string) error {
					return sender.SendDirect(ctx2, toPeerID, msg)
				}))
			})
			// Phase 2: data function dispatch + database access
			luaEngine.SetDB(db)
			node.SetLuaDispatcher(luaEngine)
			defer luaEngine.Close()
		}
	}

	// ── Group manager
	grpMgr := group.New(node.Host, db)
	log.Printf("👥 Group protocol enabled: /goop/group/1.0.0")

	// ── Realtime channels (wraps group protocol)
	rtMgr := realtime.New(grpMgr, node.ID())
	log.Printf("⚡ Realtime channels enabled")

	// ── File sharing store
	docStore, err := docs.NewStore(o.PeerDir)
	if err != nil {
		log.Printf("WARNING: Failed to create file sharing store: %v", err)
	} else {
		node.EnableDocs(docStore, grpMgr)
		log.Printf("📄 File sharing enabled: /goop/docs/1.0.0")
	}

	for _, c := range rvClients {
		cc := c
		go cc.SubscribeEvents(ctx, func(pm proto.PresenceMsg) {
			if pm.PeerID == node.ID() {
				return
			}
			switch pm.Type {
			case proto.TypeOnline, proto.TypeUpdate:
				peers.Upsert(pm.PeerID, pm.Content, pm.Email, pm.AvatarHash, pm.VideoDisabled, pm.Verified)
				addPeerAddrs(node.Host, pm.PeerID, pm.Addrs)
			case proto.TypeOffline:
				peers.Remove(pm.PeerID)
			}
		})
	}

	publish := func(pctx context.Context, typ string) {
		node.Publish(pctx, typ)
		addrs := wanAddrs(node.Host)
		for _, c := range rvClients {
			cc := c
			go func() {
				ctx2, cancel := context.WithTimeout(pctx, util.ShortTimeout)
				defer cancel()
				_ = cc.Publish(ctx2, proto.PresenceMsg{
					Type:          typ,
					PeerID:        node.ID(),
					Content:       selfContent(),
					Email:         selfEmail(),
					AvatarHash:    avatarStore.Hash(),
					VideoDisabled: selfVideoDisabled(),
					Addrs:         addrs,
					TS:            proto.NowMillis(),
				})
			}()
		}
	}

	step++
	progress(step, total, "Starting viewer")

	// ── Viewer
	if cfg.Viewer.HTTPAddr != "" {
		addr, url, _ := NormalizeLocalViewer(cfg.Viewer.HTTPAddr)
		store, err := content.NewStore(o.PeerDir, cfg.Paths.SiteRoot)
		if err != nil {
			return err
		}

		go viewer.Start(addr, viewer.Viewer{
			Node:        node,
			SelfLabel:   selfContent,
			SelfEmail:   selfEmail,
			Peers:       peers,
			CfgPath:     o.CfgPath,
			Cfg:         cfg, // always *config.Config
			Logs:        o.Logs,
			Content:     store,
			Chat:        chatMgr,
			Groups:      grpMgr,
			Realtime:    rtMgr,
			DB:          db,
			Docs:        docStore,
			BaseURL:     url,
			AvatarStore: avatarStore,
			AvatarCache: avatarCache,
			PeerDir:     o.PeerDir,
			RVClients:   rvClients,
			BridgeURL:   o.BridgeURL,
		})
	}

	node.RunPresenceLoop(ctx, func(m proto.PresenceMsg) {
		log.Printf("[%s] %s -> %q", m.Type, m.PeerID, m.Content)
	})

	publish(ctx, proto.TypeOnline)

	go func() {
		t := time.NewTicker(time.Duration(cfg.Presence.HeartbeatSec) * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				publish(ctx, proto.TypeUpdate)
			}
		}
	}()

	go func() {
		t := time.NewTicker(1 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				peers.PruneOlderThan(
					time.Now().Add(-time.Duration(cfg.Presence.TTLSec) * time.Second))
			}
		}
	}()

	<-ctx.Done()
	log.Println("========================================")
	log.Println("PEER: Context cancelled, sending offline message...")
	log.Println("========================================")
	publish(context.Background(), proto.TypeOffline)
	log.Println("PEER: Offline message sent")
	_ = rv
	return nil
}

// wanAddrs returns the host's multiaddresses filtered to exclude loopback
// and link-local addresses, suitable for sharing with WAN peers.
// Circuit relay addresses (p2p-circuit) are always included.
func wanAddrs(h host.Host) []string {
	var out []string
	for _, a := range h.Addrs() {
		// Always include circuit relay addresses.
		if isCircuitAddr(a) {
			out = append(out, a.String())
			continue
		}
		ip, err := manet.ToIP(a)
		if err != nil {
			continue
		}
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}
		out = append(out, a.String())
	}
	return out
}

// isCircuitAddr returns true if the multiaddr contains a /p2p-circuit component.
func isCircuitAddr(a ma.Multiaddr) bool {
	for _, p := range a.Protocols() {
		if p.Code == ma.P_CIRCUIT {
			return true
		}
	}
	return false
}

// addPeerAddrs parses multiaddr strings and adds them to the peerstore for the given peer.
func addPeerAddrs(h host.Host, peerID string, addrs []string) {
	if len(addrs) == 0 {
		return
	}
	pid, err := peer.Decode(peerID)
	if err != nil {
		return
	}
	var maddrs []ma.Multiaddr
	for _, s := range addrs {
		a, err := ma.NewMultiaddr(s)
		if err != nil {
			continue
		}
		// Only accept non-loopback addresses
		if ip, err := manet.ToIP(a); err == nil {
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
		}
		maddrs = append(maddrs, a)
	}
	if len(maddrs) > 0 {
		h.Peerstore().AddAddrs(pid, maddrs, 30*time.Second)
		log.Printf("WAN: added %d addrs for %s", len(maddrs), peerID)
	}
}

