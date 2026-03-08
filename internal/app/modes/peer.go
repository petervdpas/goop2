package modes

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/app/shared"
	"github.com/petervdpas/goop2/internal/avatar"
	"github.com/petervdpas/goop2/internal/call"
	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/content"
	goopCrypto "github.com/petervdpas/goop2/internal/crypto"
	"github.com/petervdpas/goop2/internal/docs"
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

type PeerParams struct {
	Ctx                   context.Context
	ModeOpts              shared.ModeOpts
	Cfg                   config.Config
	SelfContent           func() string
	SelfEmail             func() string
	SelfVideoDisabled     func() bool
	SelfActiveTemplate    func() string
	SelfPublicKey         func() string
	SelfVerificationToken func() string
	Progress              func(int, int, string)
	Step                  int
	Total                 int
}

func RunPeer(p PeerParams) error {
	ctx := p.Ctx
	o := p.ModeOpts
	cfg := p.Cfg
	selfContent := p.SelfContent
	selfEmail := p.SelfEmail
	selfVideoDisabled := p.SelfVideoDisabled
	selfActiveTemplate := p.SelfActiveTemplate
	selfPublicKey := p.SelfPublicKey
	selfVerificationToken := p.SelfVerificationToken
	progress := p.Progress
	step := p.Step
	total := p.Total

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
	node, err := p2p.New(ctx, cfg.P2P.ListenPort, keyPath, peers, selfContent, selfEmail, selfVideoDisabled, selfActiveTemplate, selfPublicKey, relayInfo, time.Duration(cfg.Presence.TTLSec)*time.Second)
	if err != nil {
		return err
	}
	defer node.Close()

	// Register all stream handlers immediately after the host is created,
	// before any peer can connect and run Identify.
	mqMgr := mq.New(node.Host)
	log.Printf("📨 MQ enabled: message queue via /goop/mq/1.0.0")

	// ── Wire E2E encryption (NaCl box) to all protocol layers
	// sealKeyFor: only encrypt for peers that advertise EncryptionSupported.
	// openKeyFor: always decrypt if we know the peer's public key (no flag check).
	// This prevents the race where a server encrypts a response before the
	// client has received the server's EncryptionSupported presence update.
	sealKeyFor := func(peerID string) (string, bool) {
		sp, ok := peers.Get(peerID)
		if !ok || sp.PublicKey == "" || !sp.EncryptionSupported {
			return "", false
		}
		return sp.PublicKey, true
	}
	openKeyFor := func(peerID string) (string, bool) {
		sp, ok := peers.Get(peerID)
		if ok && sp.PublicKey != "" {
			return sp.PublicKey, true
		}
		// Key not in local table — fetch from rendezvous over HTTPS.
		for _, c := range rvClients {
			ctx2, cancel := context.WithTimeout(context.Background(), PeerKeyFetchTimeout)
			key, err := c.FetchPeerKey(ctx2, peerID)
			cancel()
			if err == nil && key != "" {
				log.Printf("crypto: fetched public key for %s from rendezvous", peerID[:8])
				peers.SetPublicKey(peerID, key)
				return key, true
			}
		}
		return "", false
	}
	enc, err := goopCrypto.New(cfg.P2P.NaClPrivateKey, sealKeyFor, openKeyFor)
	if err != nil {
		log.Printf("crypto: failed to create encryptor: %v (continuing without encryption)", err)
	} else {
		mqMgr.SetEncryptor(enc)
		node.SetEncryptor(enc)
		log.Printf("🔐 E2E encryption enabled (NaCl box)")
	}

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
			peers.Seed(cp.PeerID, cp.Content, cp.Email, cp.AvatarHash, cp.VideoDisabled, cp.ActiveTemplate, cp.PublicKey, cp.Verified, cp.Favorite)
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
						PeerID:              evt.PeerID,
						Content:             evt.Peer.Content,
						Email:               evt.Peer.Email,
						AvatarHash:          evt.Peer.AvatarHash,
						VideoDisabled:       evt.Peer.VideoDisabled,
						ActiveTemplate:      evt.Peer.ActiveTemplate,
						PublicKey:            evt.Peer.PublicKey,
						EncryptionSupported: evt.Peer.EncryptionSupported,
						Verified:            evt.Peer.Verified,
						Reachable:           evt.Peer.Reachable,
						Offline:             !evt.Peer.OfflineSince.IsZero(),
						LastSeen:            evt.Peer.LastSeen.UnixMilli(),
						Favorite:            evt.Peer.Favorite,
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
	if enc != nil {
		listenMgr.SetEncryptor(enc)
	}
	defer listenMgr.Close()
	grpMgr.RegisterType("listen", listenMgr)
	if luaEngine != nil {
		luaEngine.SetListen(listenMgr)
	}
	setLuaListen = func() {
		if luaEngine != nil {
			luaEngine.SetListen(listenMgr)
		}
	}
	log.Printf("🎵 Listen room enabled")

	// ── Cluster compute
	clusterMgr, _ := setupCluster(mqMgr, grpMgr, node.ID())
	defer clusterMgr.Close()
	log.Printf("🖥️ Cluster compute enabled")

	// ── File sharing store
	docStore, err := docs.NewStore(o.PeerDir)
	if err != nil {
		log.Printf("WARNING: Failed to create file sharing store: %v", err)
	} else {
		node.EnableDocs(docStore, grpMgr)
		log.Printf("📄 File sharing enabled: /goop/docs/1.0.0")
	}

	rvOnMsg := func(pm proto.PresenceMsg) {
		if pm.PeerID == node.ID() {
			return
		}
		switch pm.Type {
		case proto.TypeOnline, proto.TypeUpdate:
			_, known := peers.Get(pm.PeerID)
			peers.Upsert(pm.PeerID, pm.Content, pm.Email, pm.AvatarHash, pm.VideoDisabled, pm.ActiveTemplate, pm.PublicKey, pm.EncryptionSupported, pm.Verified)
			go db.UpsertCachedPeer(storage.CachedPeer{
				PeerID:         pm.PeerID,
				Content:        pm.Content,
				Email:          pm.Email,
				AvatarHash:     pm.AvatarHash,
				VideoDisabled:  pm.VideoDisabled,
				ActiveTemplate: pm.ActiveTemplate,
				PublicKey:      pm.PublicKey,
				Verified:       pm.Verified,
				Addrs:          pm.Addrs,
			})
			node.AddPeerAddrs(pm.PeerID, pm.Addrs)
			if !known {
				go node.ProbePeer(ctx, pm.PeerID)
			}
		case proto.TypePunch:
			if pm.Target != node.ID() {
				break
			}
			log.Printf("punch hint: peer %s at %d addrs", pm.PeerID[:min(8, len(pm.PeerID))], len(pm.Addrs))
			node.AddPeerAddrs(pm.PeerID, pm.Addrs)
			go node.ProbePeer(ctx, pm.PeerID)
		case proto.TypeOffline:
			peers.MarkOffline(pm.PeerID)
		}
	}

	// Connect to each rendezvous server via WebSocket (preferred) with SSE fallback.
	for _, c := range rvClients {
		cc := c
		go cc.ConnectWebSocket(ctx, node.ID(), rvOnMsg)
	}

	publish := func(pctx context.Context, typ string) {
		node.Publish(pctx, typ)
		addrs := node.WanAddrs()
		pm := proto.PresenceMsg{
			Type:                typ,
			PeerID:              node.ID(),
			Content:             selfContent(),
			Email:               selfEmail(),
			AvatarHash:          avatarStore.Hash(),
			VideoDisabled:       selfVideoDisabled(),
			ActiveTemplate:      selfActiveTemplate(),
			PublicKey:           selfPublicKey(),
			EncryptionSupported: enc != nil,
			VerificationToken:   selfVerificationToken(),
			Addrs:               addrs,
			TS:                  proto.NowMillis(),
		}
		for _, c := range rvClients {
			cc := c
			go func() {
				// Prefer WebSocket; fall back to HTTP POST
				if cc.PublishWS(pm) {
					return
				}
				ctx2, cancel := context.WithTimeout(pctx, util.ShortTimeout)
				defer cancel()
				_ = cc.Publish(ctx2, pm)
			}()
		}
	}

	step++
	progress(step, total, "Starting viewer")

	// ── Viewer
	if cfg.Viewer.HTTPAddr != "" {
		addr, url, _ := shared.NormalizeLocalViewer(cfg.Viewer.HTTPAddr)
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
			Cfg:         cfg,
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
			Cluster:     clusterMgr,
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
				PublicKey:      m.PublicKey,
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
				PublicKey:      m.PublicKey,
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
					return nil
				}
			}
			return lastErr
		})
	}

	// Wait for relay circuit address before first publish so remote peers
	// receive our circuit address immediately (avoids backoff race).
	// Notify the browser about relay state so the user knows why WAN
	// connections may not be available yet.
	if relayInfo != nil {
		mqMgr.PublishLocal("relay:status", "", map[string]any{
			"status": "waiting",
			"msg":    "Connecting to relay — WAN peers require a relay circuit",
		})
		if node.WaitForRelay(ctx, RelayWaitTimeout) {
			mqMgr.PublishLocal("relay:status", "", map[string]any{
				"status": "connected",
				"msg":    "Relay connected — WAN peers are reachable",
			})
		} else {
			mqMgr.PublishLocal("relay:status", "", map[string]any{
				"status": "timeout",
				"msg":    "Relay unavailable — WAN connections will not work until relay recovers",
			})
		}
	}

	publish(ctx, proto.TypeOnline)

	// Register NaCl public key with encryption service(s) after first publish.
	if cfg.P2P.NaClPublicKey != "" {
		for _, c := range rvClients {
			cc := c
			go func() {
				regCtx, cancel := context.WithTimeout(ctx, EncryptionRegisterTimeout)
				defer cancel()
				if err := cc.RegisterEncryptionKey(regCtx, node.ID(), cfg.P2P.NaClPublicKey); err != nil {
					log.Printf("encryption: key registration failed: %v", err)
				} else {
					log.Printf("encryption: public key registered via %s", cc.BaseURL)
				}
			}()
		}
	}

	// Re-publish and re-probe when our addresses change (network switch,
	// relay address appears/disappears).  Always subscribe — not just when
	// relay is configured — so LAN↔WAN transitions trigger probes.
	node.SubscribeAddressChanges(ctx, func() {
		publish(ctx, proto.TypeUpdate)
	}, func(hasCircuit bool) {
		if hasCircuit {
			mqMgr.PublishLocal("relay:status", "", map[string]any{
				"status": "recovered",
				"msg":    "Relay circuit restored — WAN peers are reachable again",
			})
		} else {
			mqMgr.PublishLocal("relay:status", "", map[string]any{
				"status": "lost",
				"msg":    "Relay circuit lost — recovering...",
			})
		}
	})
	node.SubscribeConnectionEvents(ctx, nil)
	if relayInfo != nil {
		// Periodically refresh the relay connection to prevent stale state.
		// This ensures the relay reservation stays active even when the TCP
		// connection to the relay silently degrades.
		refreshInterval := DefaultRelayRefresh
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
		t := time.NewTicker(PruneCheckInterval)
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
				if graceRefresh >= ConfigRereadInterval {
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
