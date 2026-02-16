package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/avatar"
	"github.com/petervdpas/goop2/internal/chat"
	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/content"
	"github.com/petervdpas/goop2/internal/docs"
	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/listen"
	luapkg "github.com/petervdpas/goop2/internal/lua"
	"github.com/petervdpas/goop2/internal/p2p"
	"github.com/petervdpas/goop2/internal/proto"
	"github.com/petervdpas/goop2/internal/realtime"
	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
	"github.com/petervdpas/goop2/internal/util"
	"github.com/petervdpas/goop2/internal/viewer"
)

type Options struct {
	PeerDir   string
	CfgPath   string
	Cfg       config.Config
	BridgeURL string
	Progress  func(step, total int, label string)
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
		rv = rendezvous.New(addr, peerDBPath, cfg.Presence.AdminPassword, cfg.Presence.ExternalURL, cfg.Presence.RelayPort, relayKeyFile, rendezvous.RelayTimingConfig{
			CleanupDelaySec:    cfg.Presence.RelayCleanupDelaySec,
			PollDeadlineSec:    cfg.Presence.RelayPollDeadlineSec,
			ConnectTimeoutSec:  cfg.Presence.RelayConnectTimeoutSec,
			RefreshIntervalSec: cfg.Presence.RelayRefreshIntervalSec,
			RecoveryGraceSec:   cfg.Presence.RelayRecoveryGraceSec,
		})

		// Wire external services (credits + registration + email + templates)
		if cfg.Presence.UseServices {
			setupCredits(rv, cfg.Presence.CreditsURL, cfg.Presence.CreditsAdminToken)
			setupRegistration(rv, cfg.Presence.RegistrationURL, cfg.Presence.RegistrationAdminToken)
			setupEmail(rv, cfg.Presence.EmailURL)
			setupTemplates(rv, cfg.Presence.TemplatesURL, cfg.Presence.TemplatesAdminToken, cfg.Presence.TemplatesDir, o.PeerDir)
		} else {
			// No services — still allow local template store
			setupTemplates(rv, "", "", cfg.Presence.TemplatesDir, o.PeerDir)
		}

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

	selfActiveTemplate := func() string {
		if c, err := config.LoadPartial(o.CfgPath); err == nil {
			return c.Viewer.ActiveTemplate
		}
		return cfg.Viewer.ActiveTemplate
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
				BridgeURL:      o.BridgeURL,
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
			rendezvous.NewClient(util.NormalizeURL(cfg.Presence.RendezvousWAN)))
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
	node, err := p2p.New(ctx, cfg.P2P.ListenPort, keyPath, peers, selfContent, selfEmail, selfVideoDisabled, selfActiveTemplate, relayInfo, time.Duration(cfg.Presence.TTLSec)*time.Second)
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
	var luaEngine *luapkg.Engine
	var luaOnce sync.Once

	startLua := func() {
		luaOnce.Do(func() {
			luaCfg := cfg.Lua
			// When auto-enabled via template apply, re-read config for latest values.
			if c, err := config.Load(o.CfgPath); err == nil {
				luaCfg = c.Lua
			}
			var luaErr error
			luaEngine, luaErr = luapkg.NewEngine(luaCfg, o.PeerDir, node.ID(), selfContent, peers)
			if luaErr != nil {
				log.Printf("WARNING: Lua engine failed to start: %v", luaErr)
				luaEngine = nil
				return
			}
			chatMgr.SetCommandHandler(func(ctx context.Context, fromPeerID, content string, sender chat.DirectSender) {
				luaEngine.Dispatch(ctx, fromPeerID, content, luapkg.SenderFunc(func(ctx2 context.Context, toPeerID, msg string) error {
					return sender.SendDirect(ctx2, toPeerID, msg)
				}))
			})
			luaEngine.SetDB(db)
			node.SetLuaDispatcher(luaEngine)
		})
	}

	if cfg.Lua.Enabled {
		startLua()
	}
	defer func() {
		if luaEngine != nil {
			luaEngine.Close()
		}
	}()

	// ensureLua is called by template apply when Lua files are detected.
	// It enables Lua in config, starts the engine if needed, and rescans.
	// setLuaListen is set after listenMgr is created, so ensureLua can wire it.
	var setLuaListen func()
	ensureLua := func() {
		if c, err := config.Load(o.CfgPath); err == nil {
			if !c.Lua.Enabled {
				c.Lua.Enabled = true
				config.Save(o.CfgPath, c)
				log.Printf("LUA: auto-enabled in config (template with Lua functions applied)")
			}
		}
		startLua()
		if setLuaListen != nil {
			setLuaListen()
		}
		node.RescanLuaFunctions()
	}

	// ── Group manager
	grpMgr := group.New(node.Host, db)
	log.Printf("👥 Group protocol enabled: /goop/group/1.0.0")

	// ── Realtime channels (wraps group protocol)
	rtMgr := realtime.New(grpMgr, node.ID())
	log.Printf("⚡ Realtime channels enabled")

	// ── Listen room (wraps group protocol + binary audio stream)
	listenMgr := listen.New(node.Host, grpMgr, node.ID())
	defer listenMgr.Close()
	if luaEngine != nil {
		luaEngine.SetListen(listenMgr)
	}
	setLuaListen = func() {
		if luaEngine != nil {
			luaEngine.SetListen(listenMgr)
		}
	}
	log.Printf("🎵 Listen room enabled")

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
				peers.Upsert(pm.PeerID, pm.Content, pm.Email, pm.AvatarHash, pm.VideoDisabled, pm.ActiveTemplate, pm.Verified)
				node.AddPeerAddrs(pm.PeerID, pm.Addrs)
			case proto.TypeOffline:
				peers.Remove(pm.PeerID)
			}
		})
	}

	publish := func(pctx context.Context, typ string) {
		node.Publish(pctx, typ)
		addrs := node.WanAddrs()
		for _, c := range rvClients {
			cc := c
			go func() {
				ctx2, cancel := context.WithTimeout(pctx, util.ShortTimeout)
				defer cancel()
				_ = cc.Publish(ctx2, proto.PresenceMsg{
					Type:           typ,
					PeerID:         node.ID(),
					Content:        selfContent(),
					Email:          selfEmail(),
					AvatarHash:     avatarStore.Hash(),
					VideoDisabled:  selfVideoDisabled(),
					ActiveTemplate: selfActiveTemplate(),
					Addrs:          addrs,
					TS:             proto.NowMillis(),
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
			Listen:      listenMgr,
			DB:          db,
			Docs:        docStore,
			BaseURL:     url,
			AvatarStore: avatarStore,
			AvatarCache: avatarCache,
			PeerDir:     o.PeerDir,
			RVClients:   rvClients,
			BridgeURL:   o.BridgeURL,
			EnsureLua:   ensureLua,
		})
	}

	// Track known peer content to suppress repetitive update logs.
	seenContent := make(map[string]string)
	node.RunPresenceLoop(ctx, func(m proto.PresenceMsg) {
		switch m.Type {
		case proto.TypeOnline:
			seenContent[m.PeerID] = m.Content
			log.Printf("[%s] %s -> %q", m.Type, m.PeerID, m.Content)
		case proto.TypeUpdate:
			prev, known := seenContent[m.PeerID]
			if !known || prev != m.Content {
				seenContent[m.PeerID] = m.Content
				log.Printf("[%s] %s -> %q", m.Type, m.PeerID, m.Content)
			}
		case proto.TypeOffline:
			delete(seenContent, m.PeerID)
			log.Printf("[%s] %s", m.Type, m.PeerID)
		}
	})

	// Wire pulse function — when FetchSiteFile can't reach a peer, it asks
	// the rendezvous to pulse the target peer's relay reservation.
	if len(rvClients) > 0 {
		node.SetPulseFn(func(pctx context.Context, peerID string) error {
			var lastErr error
			for _, c := range rvClients {
				if err := c.PulsePeer(pctx, peerID); err != nil {
					lastErr = err
				} else {
					return nil // success on any client is enough
				}
			}
			return lastErr
		})
	}

	// Wait for relay circuit address before first publish so remote peers
	// receive our circuit address immediately (avoids backoff race).
	if relayInfo != nil {
		node.WaitForRelay(ctx, 15*time.Second)
	}

	publish(ctx, proto.TypeOnline)

	// Re-publish when circuit relay addresses appear or disappear so remote
	// peers always have our latest reachable addresses.
	if relayInfo != nil {
		node.SubscribeAddressChanges(ctx, func() {
			publish(ctx, proto.TypeUpdate)
		})
		// Periodically refresh the relay connection to prevent stale state.
		// This ensures the relay reservation stays active even when the TCP
		// connection to the relay silently degrades.
		refreshInterval := 5 * time.Minute
		if relayInfo.RefreshIntervalSec > 0 {
			refreshInterval = time.Duration(relayInfo.RefreshIntervalSec) * time.Second
		}
		node.StartRelayRefresh(ctx, refreshInterval)
	}

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
	return nil
}


