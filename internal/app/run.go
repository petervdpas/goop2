package app

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/avatar"
	"github.com/petervdpas/goop2/internal/call"
	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/content"
	"github.com/petervdpas/goop2/internal/docs"
	"github.com/petervdpas/goop2/internal/entangle"
	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/listen"
	luapkg "github.com/petervdpas/goop2/internal/lua"
	"github.com/petervdpas/goop2/internal/mq"
	"github.com/petervdpas/goop2/internal/p2p"
	"github.com/petervdpas/goop2/internal/proto"
	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
	"github.com/petervdpas/goop2/internal/util"
	"github.com/petervdpas/goop2/internal/viewer"
)

// mqSignalerAdapter bridges *mq.Manager to call.Signaler.
// This is the only place that imports both packages — call knows nothing about mq.
type mqSignalerAdapter struct {
	mqMgr *mq.Manager

	mu    sync.Mutex
	peers map[string]string // channelID → peerID
}

// RegisterChannel associates a call channel ID with the remote peer ID.
// Must be called by run.go after StartCall/AcceptCall so Send knows the peer.
func (a *mqSignalerAdapter) RegisterChannel(channelID, peerID string) {
	a.mu.Lock()
	a.peers[channelID] = peerID
	a.mu.Unlock()
}

func (a *mqSignalerAdapter) Send(channelID string, payload any) error {
	a.mu.Lock()
	peerID, ok := a.peers[channelID]
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("mqSignaler: no peer registered for channel %s", channelID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := a.mqMgr.Send(ctx, peerID, "call:"+channelID, payload)
	return err
}

func (a *mqSignalerAdapter) PublishLocal(channelID string, payload any) {
	a.mqMgr.PublishLocal("call:"+channelID, "", payload)
}

func (a *mqSignalerAdapter) Subscribe() (chan *call.Envelope, func()) {
	callCh := make(chan *call.Envelope, 64)
	unsub := a.mqMgr.SubscribeTopic("call:", func(from, topic string, payload any) {
		channelID := strings.TrimPrefix(topic, "call:")
		select {
		case callCh <- &call.Envelope{Channel: channelID, From: from, Payload: payload}:
		default:
			log.Printf("mqSignaler: callCh full, dropping envelope for channel %s", channelID)
		}
	})
	cancel := func() {
		unsub()
		close(callCh)
	}
	return callCh, cancel
}

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
			setupMicroService("Credits", cfg.Presence.CreditsURL, func() {
				rv.SetCreditProvider(rendezvous.NewRemoteCreditProvider(
					cfg.Presence.CreditsURL, rv.GetEmailForPeer, rv.GetTokenForPeer, cfg.Presence.CreditsAdminToken))
			})
			setupMicroService("Registration", cfg.Presence.RegistrationURL, func() {
				rv.SetRegistrationProvider(rendezvous.NewRemoteRegistrationProvider(
					cfg.Presence.RegistrationURL, cfg.Presence.RegistrationAdminToken))
			})
			setupMicroService("Email", cfg.Presence.EmailURL, func() {
				rv.SetEmailProvider(rendezvous.NewRemoteEmailProvider(cfg.Presence.EmailURL))
			})
			setupMicroService("Templates", cfg.Presence.TemplatesURL, func() {
				rv.SetTemplatesProvider(rendezvous.NewRemoteTemplatesProvider(
					cfg.Presence.TemplatesURL, cfg.Presence.TemplatesAdminToken))
			})
		}

		// Local template store fallback (works with or without services)
		if cfg.Presence.TemplatesDir != "" && (cfg.Presence.TemplatesURL == "" || !cfg.Presence.UseServices) {
			dir := util.ResolvePath(o.PeerDir, cfg.Presence.TemplatesDir)
			if store := rendezvous.NewLocalTemplateStore(dir); store != nil {
				log.Printf("Local template store: %s (%d templates)", dir, store.Count())
				rv.SetLocalTemplateStore(store)
			}
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

	selfVerificationToken := func() string {
		if c, err := config.LoadPartial(o.CfgPath); err == nil {
			return c.Profile.VerificationToken
		}
		return cfg.Profile.VerificationToken
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

	// Register all stream handlers immediately after the host is created,
	// before any peer can connect and run Identify.
	mqMgr := mq.New(node.Host)
	log.Printf("📨 MQ enabled: message queue via /goop/mq/1.0.0")

	// Entangle handler registered here so the protocol appears in Identify
	// from the very first connection. The Connect() calls come later (after
	// peer cache load), but the handler must be ready immediately.
	entMgr := entangle.New(node.Host,
		func(peerID string) { peers.SetReachable(peerID, true) },
		func(peerID string) { peers.MarkOffline(peerID) },
	)
	defer entMgr.Close()
	log.Printf("🔗 Entangle enabled: persistent peer threads via /goop/entangle/1.0.0")

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

	if cachedPeers, err := db.ListCachedPeers(); err == nil {
		for _, cp := range cachedPeers {
			peers.Seed(cp.PeerID, cp.Content, cp.Email, cp.AvatarHash, cp.VideoDisabled, cp.ActiveTemplate, cp.Verified, cp.Favorite)
			if len(cp.Addrs) > 0 {
				node.AddPeerAddrs(cp.PeerID, cp.Addrs)
			}
			// Pre-populate peerstore with cached protocol lists so mq.Send()
			// can fast-fail for peers that don't support /goop/mq/1.0.0.
			node.SetPeerProtocols(cp.PeerID, cp.Protocols)
		}
		if len(cachedPeers) > 0 {
			log.Printf("peer cache: loaded %d known peers", len(cachedPeers))
		}
	}

	step++
	progress(step, total, "Setting up services")

	// Bridge: PeerTable → MQ so the browser's mq.js maintains a peer name cache.
	// Every peer presence change (online/update/offline/prune) is forwarded as
	// peer:announce (or peer:gone) via PublishLocal, making peer metadata
	// available to all MQ subscribers without a separate API call.
	go func() {
		peerCh := peers.Subscribe()
		defer peers.Unsubscribe(peerCh)
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-peerCh:
				if !ok {
					return
				}
				if evt.Type == "update" && evt.Peer != nil && evt.PeerID != "" {
					mqMgr.PublishPeerAnnounce(mq.PeerAnnouncePayload{
						PeerID:         evt.PeerID,
						Content:        evt.Peer.Content,
						Email:          evt.Peer.Email,
						AvatarHash:     evt.Peer.AvatarHash,
						VideoDisabled:  evt.Peer.VideoDisabled,
						ActiveTemplate: evt.Peer.ActiveTemplate,
						Verified:       evt.Peer.Verified,
						Reachable:      evt.Peer.Reachable,
						Offline:        !evt.Peer.OfflineSince.IsZero(),
						LastSeen:       evt.Peer.LastSeen.UnixMilli(),
						Favorite:       evt.Peer.Favorite,
					})
				} else if evt.Type == "remove" && evt.PeerID != "" {
					mqMgr.PublishPeerGone(evt.PeerID)
					// Sync DB cache with in-memory prune: delete from _peer_cache.
					// Favorites survive in _favorites; non-favorites are gone for good.
					go db.DeleteCachedPeer(evt.PeerID)
				}
			}
		}
	}()

	// Persist peer protocol lists whenever libp2p Identify completes.
	// This keeps the DB cache warm across restarts so peerSupportsMQ()
	// can fast-fail for old clients without a dial attempt.
	node.SubscribeIdentify(ctx, func(peerID string, protocols []string) {
		go db.UpsertPeerProtocols(peerID, protocols)
	})

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
	grpMgr := group.New(node.Host, db, mqMgr)
	log.Printf("👥 Group manager enabled (MQ transport)")

	// ── Native call manager (Go/Pion WebRTC — Linux only)
	// Mode is determined by platform: Linux uses Go/Pion (WebKitGTK has no RTCPeerConnection),
	// all other platforms use browser-native WebRTC. No config toggle needed.
	var callMgr *call.Manager
	if runtime.GOOS == "linux" {
		sigAdapter := &mqSignalerAdapter{mqMgr: mqMgr, peers: make(map[string]string)}
		// callLogFn publishes structured log events from the call layer (e.g. hardware
		// capture errors) to the MQ bus so they appear in the browser's Video log tab.
		callLogFn := func(level, msg string) {
			mqMgr.PublishLocal("log:call", "", map[string]any{
				"level":  level,
				"source": "media",
				"msg":    msg,
				"ts":     time.Now().UnixMilli(),
			})
		}
		callMgr = call.New(sigAdapter, node.ID(), callLogFn, runtime.GOOS)
		defer callMgr.Close()
		log.Printf("📞 Experimental native call stack enabled (Go/Pion WebRTC)")
	}

	// ── Listen room (wraps group protocol + binary audio stream)
	listenMgr := listen.New(node.Host, grpMgr, mqMgr, node.ID(), o.PeerDir)
	defer listenMgr.Close()
	grpMgr.RegisterHandler("listen", listenMgr)
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

	// Entangle all peers already in the table at startup.
	for _, peerID := range peers.IDs() {
		go entMgr.Connect(ctx, peerID)
	}

	// Entangle whenever libp2p establishes a new connection (mDNS, relay, direct).
	node.SubscribeConnectionEvents(ctx, func(peerID string) {
		go entMgr.Connect(ctx, peerID)
	})

	for _, c := range rvClients {
		cc := c
		go cc.SubscribeEvents(ctx, func(pm proto.PresenceMsg) {
			if pm.PeerID == node.ID() {
				return
			}
			switch pm.Type {
			case proto.TypeOnline, proto.TypeUpdate:
				sp, known := peers.Get(pm.PeerID)
				peers.Upsert(pm.PeerID, pm.Content, pm.Email, pm.AvatarHash, pm.VideoDisabled, pm.ActiveTemplate, pm.Verified)
				go db.UpsertCachedPeer(storage.CachedPeer{
					PeerID:         pm.PeerID,
					Content:        pm.Content,
					Email:          pm.Email,
					AvatarHash:     pm.AvatarHash,
					VideoDisabled:  pm.VideoDisabled,
					ActiveTemplate: pm.ActiveTemplate,
					Verified:       pm.Verified,
					Addrs:          pm.Addrs,
				})
				node.AddPeerAddrs(pm.PeerID, pm.Addrs)
				// Probe on first sight or when a known-unreachable peer reappears.
				if !known || (pm.Type == proto.TypeUpdate && !sp.Reachable) {
					go node.ProbePeer(ctx, pm.PeerID)
				}
				// Entangle: open (or reuse) the persistent heartbeat stream.
				go entMgr.Connect(ctx, pm.PeerID)
			case proto.TypeOffline:
				peers.MarkOffline(pm.PeerID)
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
					ActiveTemplate:    selfActiveTemplate(),
					VerificationToken: selfVerificationToken(),
					Addrs:             addrs,
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
			MQ:          mqMgr,
			Groups:      grpMgr,
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
			Call:        callMgr,
		})
	}

	// Track known peer content to suppress repetitive update logs.
	seenContent := make(map[string]string)
	node.RunPresenceLoop(ctx, func(m proto.PresenceMsg) {
		switch m.Type {
		case proto.TypeOnline:
			seenContent[m.PeerID] = m.Content
			log.Printf("[%s] %s -> %q", m.Type, m.PeerID, m.Content)
			// Use the peer table's Verified value — it is set exclusively by the
			// rendezvous server and must not be overwritten by P2P gossip.
			sp, _ := peers.Get(m.PeerID)
			go db.UpsertCachedPeer(storage.CachedPeer{
				PeerID:         m.PeerID,
				Content:        m.Content,
				Email:          m.Email,
				AvatarHash:     m.AvatarHash,
				VideoDisabled:  m.VideoDisabled,
				ActiveTemplate: m.ActiveTemplate,
				Verified:       sp.Verified,
				Addrs:          m.Addrs,
			})
			go node.ProbePeer(ctx, m.PeerID)
		case proto.TypeUpdate:
			prev, known := seenContent[m.PeerID]
			if !known || prev != m.Content {
				seenContent[m.PeerID] = m.Content
				log.Printf("[%s] %s -> %q", m.Type, m.PeerID, m.Content)
			}
			sp, _ := peers.Get(m.PeerID)
			go db.UpsertCachedPeer(storage.CachedPeer{
				PeerID:         m.PeerID,
				Content:        m.Content,
				Email:          m.Email,
				AvatarHash:     m.AvatarHash,
				VideoDisabled:  m.VideoDisabled,
				ActiveTemplate: m.ActiveTemplate,
				Verified:       sp.Verified,
				Addrs:          m.Addrs,
			})
			// If the peer is currently unreachable, their relay circuit may have
			// just appeared — probe immediately rather than waiting for the next
			// browser-triggered round (up to 5 s away).
			if sp, ok := peers.Get(m.PeerID); ok && !sp.Reachable {
				go node.ProbePeer(ctx, m.PeerID)
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

	// Re-publish and re-probe when our addresses change (network switch,
	// relay address appears/disappears).  Always subscribe — not just when
	// relay is configured — so LAN↔WAN transitions trigger probes.
	node.SubscribeAddressChanges(ctx, func() {
		publish(ctx, proto.TypeUpdate)
	})
	node.SubscribeConnectionEvents(ctx, nil)
	if relayInfo != nil {
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
		graceMin := cfg.Viewer.PeerOfflineGraceMin
		if graceMin < 1 || graceMin > 60 {
			graceMin = 15
		}
		var graceRefresh int
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				// Re-read grace period from config once every 5 minutes.
				graceRefresh++
				if graceRefresh >= 300 {
					graceRefresh = 0
					if live, err := config.LoadPartial(o.CfgPath); err == nil {
						v := live.Viewer.PeerOfflineGraceMin
						if v >= 1 && v <= 60 {
							graceMin = v
						}
					}
				}
				ttlCutoff := time.Now().Add(-time.Duration(cfg.Presence.TTLSec) * time.Second)
				graceCutoff := time.Now().Add(-time.Duration(graceMin) * time.Minute)
				peers.PruneStale(ttlCutoff, graceCutoff)
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


