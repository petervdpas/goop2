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
	"github.com/petervdpas/goop2/internal/chat"
	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/content"
	goopCrypto "github.com/petervdpas/goop2/internal/crypto"
	"github.com/petervdpas/goop2/internal/group"
	clusterType "github.com/petervdpas/goop2/internal/group_types/cluster"
	"github.com/petervdpas/goop2/internal/group_types/datafed"
	"github.com/petervdpas/goop2/internal/orm/gql"
	filesType "github.com/petervdpas/goop2/internal/group_types/files"
	"github.com/petervdpas/goop2/internal/group_types/listen"
	chatType "github.com/petervdpas/goop2/internal/group_types/chat"
	templateType "github.com/petervdpas/goop2/internal/group_types/template"
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

	var reachableClients []*rendezvous.Client
	for _, c := range rvClients {
		c.WarmDNS(ctx)
		if c.DNSReady() {
			reachableClients = append(reachableClients, c)
		}
	}
	rvClients = reachableClients

	var relayInfo *rendezvous.RelayInfo
	if len(rvClients) > 0 {
		type relayResult struct {
			info *rendezvous.RelayInfo
		}
		ch := make(chan relayResult, len(rvClients))
		for _, c := range rvClients {
			go func(c *rendezvous.Client) {
				ri, err := c.FetchRelayInfo(ctx)
				if err != nil {
					log.Printf("relay: fetch from %s failed: %v", c.BaseURL, err)
					ch <- relayResult{}
				} else if ri == nil {
					log.Printf("relay: %s has no relay configured", c.BaseURL)
					ch <- relayResult{}
				} else {
					ch <- relayResult{info: ri}
				}
			}(c)
		}
		for range rvClients {
			if r := <-ch; r.info != nil && relayInfo == nil {
				relayInfo = r.info
				log.Printf("relay: discovered relay peer %s (%d addrs)", r.info.PeerID, len(r.info.Addrs))
			}
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

	// Start watching connection events immediately so mDNS connections
	// (which can happen inside p2p.New) mark peers reachable right away.
	node.SubscribeConnectionEvents(ctx, nil)

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
	if o.GoopClientVersion != "" {
		node.SetGoopClientVersion(o.GoopClientVersion)
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

	// ── Canonical peer identity resolver ─────────────────────────────────
	// Single function for resolving a peer ID to its full identity. Every
	// subsystem (chat, groups, listen, viewer) uses this same instance.
	// Identity comes from presence (WebSocket/gossipsub → PeerTable) or
	// the DB cache. Returns empty PeerIdentity if the peer is unknown.
	resolvePeer := func(id string) state.PeerIdentity {
		if id == node.ID() {
			return state.PeerIdentity{
				Name:  selfContent(),
				Email: selfEmail(),
				Known: true,
			}
		}
		if sp, ok := peers.Get(id); ok {
			return state.FromSeenPeer(sp)
		}
		if cp, ok := db.GetCachedPeer(id); ok {
			return state.PeerIdentity{
				Name:       cp.Content,
				Email:      cp.Email,
				AvatarHash: cp.AvatarHash,
				Reachable:  len(cp.Addrs) > 0,
				Known:      true,
			}
		}
		// Unknown peer — request identity over MQ. The response handler
		// above will upsert into PeerTable asynchronously, so next lookup
		// will have the data. Fire-and-forget: we don't block for the response.
		go func() {
			reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			_, _ = mqMgr.Send(reqCtx, id, mq.TopicIdentity, nil)
		}()
		return state.PeerIdentity{}
	}

	// ── Identity MQ handler ──────────────────────────────────────────────
	// When a peer sends us "identity", respond with our full identity on
	// "identity.response". This handles the timing race where an MQ message
	// arrives before the WebSocket presence has propagated.
	mqMgr.SubscribeTopic(mq.TopicIdentity, func(from, topic string, _ any) {
		if topic != mq.TopicIdentity {
			return
		}
		resp := mq.IdentityPayload{
			PeerID:              node.ID(),
			Content:             selfContent(),
			Email:               selfEmail(),
			AvatarHash:          avatarStore.Hash(),
			GoopClientVersion:   o.GoopClientVersion,
			PublicKey:           selfPublicKey(),
			EncryptionSupported: selfPublicKey() != "",
			ActiveTemplate:      selfActiveTemplate(),
			VideoDisabled:       selfVideoDisabled(),
		}
		sendCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, _ = mqMgr.Send(sendCtx, from, mq.TopicIdentityResponse, resp)
	})

	// Handle incoming identity responses — upsert into PeerTable.
	mqMgr.SubscribeTopic(mq.TopicIdentityResponse, func(from, topic string, payload any) {
		if topic != mq.TopicIdentityResponse {
			return
		}
		// The payload arrives as map[string]any from JSON dispatch.
		pm, ok := payload.(map[string]any)
		if !ok {
			return
		}
		content, _ := pm["content"].(string)
		email, _ := pm["email"].(string)
		avatarHash, _ := pm["avatarHash"].(string)
		version, _ := pm["goopClientVersion"].(string)
		publicKey, _ := pm["publicKey"].(string)
		encSupported, _ := pm["encryptionSupported"].(bool)
		activeTemplate, _ := pm["activeTemplate"].(string)
		videoDisabled, _ := pm["videoDisabled"].(bool)
		if content != "" {
			peers.Upsert(from, content, email, avatarHash, videoDisabled, activeTemplate, publicKey, encSupported, false, version)
		}
	})

	// Proactively fetch avatars when a peer announces a hash we don't have cached.
	warmAvatar := func(peerID, hash string) {
		if hash == "" || avatarCache == nil {
			return
		}
		if cached, _ := avatarCache.Get(peerID, hash); cached != nil {
			return
		}
		go func() {
			ctx2, cancel := context.WithTimeout(ctx, AvatarWarmTimeout)
			defer cancel()
			data, err := node.FetchAvatar(ctx2, peerID)
			if err == nil && data != nil {
				_ = avatarCache.Put(peerID, hash, data)
			}
		}()
	}

	// Start rendezvous WS connections as early as possible so peer discovery
	// begins while we wire up services. All dependencies (peers, node, db) are ready.
	announced := make(map[string]bool)
	rvOnMsg := func(pm proto.PresenceMsg) {
		if pm.PeerID == node.ID() {
			return
		}
		switch pm.Type {
		case proto.TypeOnline, proto.TypeUpdate:
			_, known := peers.Get(pm.PeerID)
			if !announced[pm.PeerID] {
				announced[pm.PeerID] = true
				name := pm.Content
				if name == "" {
					name = pm.PeerID[:min(16, len(pm.PeerID))]
				}
				log.Printf("[online] %s (%s) — %d addrs", pm.PeerID[:min(16, len(pm.PeerID))], name, len(pm.Addrs))
			}
			peers.Upsert(pm.PeerID, pm.Content, pm.Email, pm.AvatarHash, pm.VideoDisabled, pm.ActiveTemplate, pm.PublicKey, pm.EncryptionSupported, pm.Verified, pm.GoopClientVersion)
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
			warmAvatar(pm.PeerID, pm.AvatarHash)
		case proto.TypePunch:
			if pm.Target != node.ID() {
				break
			}
			log.Printf("punch hint: peer %s at %d addrs", pm.PeerID[:min(8, len(pm.PeerID))], len(pm.Addrs))
			node.AddPeerAddrs(pm.PeerID, pm.Addrs)
			go node.ProbePeer(ctx, pm.PeerID)
		case proto.TypeOffline:
			log.Printf("[offline] %s", pm.PeerID[:min(16, len(pm.PeerID))])
			peers.MarkOffline(pm.PeerID)
		}
	}
	for _, c := range rvClients {
		cc := c
		go cc.ConnectWebSocket(ctx, node.ID(), rvOnMsg)
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
						GoopClientVersion:   evt.Peer.GoopClientVersion,
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

	// ── Chat manager
	chatMgr := chat.New(node.ID(), chat.NewDBStore(db), mqMgr)
	chatMgr.Start()

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
			chatMgr.SetLuaDispatcher(luaEngine)
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
	var setLuaContent func()
	var setLuaGroups func()
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
		if setLuaContent != nil {
			setLuaContent()
		}
		if setLuaGroups != nil {
			setLuaGroups()
		}
		node.RescanLuaFunctions()
	}

	// ── Group manager
	grpMgr := group.New(node.Host, db, mqMgr, resolvePeer)
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

	// ── Chat group type (chat rooms)
	chatRoomMgr := chatType.New(grpMgr, mqMgr, node.ID(), resolvePeer)
	defer chatRoomMgr.Close()

	if luaEngine != nil {
		luaEngine.SetListen(listenMgr)
		luaEngine.SetChatRooms(chatRoomMgr)
		luaEngine.SetGroupChecker(grpMgr)
		luaEngine.SetGroupManager(grpMgr)
		luaEngine.SetMQ(mqMgr)
	}
	setLuaListen = func() {
		if luaEngine != nil {
			luaEngine.SetListen(listenMgr)
			luaEngine.SetChatRooms(chatRoomMgr)
		}
	}
	setLuaGroups = func() {
		if luaEngine != nil {
			luaEngine.SetGroupChecker(grpMgr)
			luaEngine.SetGroupManager(grpMgr)
			luaEngine.SetMQ(mqMgr)
		}
	}
	log.Printf("🎵 Listen room enabled")

	// ── Cluster compute
	clusterMgr := clusterType.New(mqMgr, grpMgr, node.ID())
	clusterMgr.SetDB(clusterType.NewJobStore(db))
	if cfg.Viewer.ClusterBinaryPath != "" {
		clusterMgr.SetSavedBinary(cfg.Viewer.ClusterBinaryPath, cfg.Viewer.ClusterBinaryMode)
	}
	defer clusterMgr.Close()
	if hosted, err := grpMgr.ListHostedGroups(); err == nil {
		for _, g := range hosted {
			if g.GroupType == "cluster" {
				if err := grpMgr.RestoreGroup(g.ID); err == nil {
					if err := clusterMgr.CreateCluster(g.ID); err == nil {
						log.Printf("🖥️ Cluster auto-activated: %s (%s)", g.Name, g.ID)
					}
				}
				break
			}
		}
	}
	log.Printf("🖥️ Cluster compute enabled")

	// ── File sharing store
	docStore, err := filesType.NewStore(o.PeerDir)
	if err != nil {
		log.Printf("WARNING: Failed to create file sharing store: %v", err)
	} else {
		node.EnableDocs(docStore, grpMgr)
		filesType.New(mqMgr, grpMgr, docStore)
		log.Printf("📄 File sharing enabled: /goop/docs/1.0.0")
	}

	// ── Data federation (GraphQL over P2P)
	gqlEngine := gql.New(db, node.ID(), selfEmail)
	_ = gqlEngine.Rebuild()
	dataFedMgr := datafed.New(mqMgr, grpMgr, node.ID(), gqlEngine.ContextTables)
	log.Printf("🔗 Data federation enabled (GraphQL)")

	// ── Template group type
	tplHandler := templateType.New(grpMgr)
	tplHandler.AddCleaner(chatRoomMgr)



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
			GoopClientVersion:   o.GoopClientVersion,
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
				if err := cc.Publish(ctx2, pm); err != nil {
					log.Printf("rendezvous: publish to %s failed: %v", cc.BaseURL, err)
				}
			}()
		}
	}

	// Publish immediately — announce ourselves as early as possible so peers
	// can discover us while we finish wiring up services and the viewer.
	publish(ctx, proto.TypeOnline)

	if relayInfo != nil {
		mqMgr.PublishLocal("relay:status", "", map[string]any{
			"status": "waiting",
			"msg":    "Connecting to relay — WAN peers will be reachable once circuit is obtained",
		})
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
		if luaEngine != nil {
			luaEngine.SetContent(store)
		}
		setLuaContent = func() {
			if luaEngine != nil {
				luaEngine.SetContent(store)
			}
		}

		dataFedMgr.SetOnChange(func() {
			peerSources := dataFedMgr.AllPeerSources()
			var peers []gql.PeerSource
			for peerID, tables := range peerSources {
				peers = append(peers, gql.PeerSource{PeerID: peerID, Tables: tables})
			}
			queryFn := gql.DefaultPeerQueryFunc(url)
			if err := gqlEngine.RebuildFederated(gqlEngine.ContextTables(), peers, queryFn); err != nil {
				log.Printf("DATA-FED: rebuild failed: %v", err)
			}
		})

		go viewer.Start(addr, viewer.Viewer{
			Node:        node,
			SelfLabel:   selfContent,
			SelfEmail:   selfEmail,
			Peers:       peers,
			ResolvePeer: resolvePeer,
			CfgPath:     o.CfgPath,
			Cfg:         cfg,
			Logs:        o.Logs,
			Content:     store,
			MQ:          mqMgr,
			Groups:      grpMgr,
			Listen:      listenMgr,
			ChatRooms:   chatRoomMgr,
			DB:          db,
			Docs:        docStore,
			BaseURL:     url,
			AvatarStore: avatarStore,
			AvatarCache: avatarCache,
			PeerDir:     o.PeerDir,
			RVClients:   rvClients,
			BridgeURL:   o.BridgeURL,
			Chat:        chatMgr,
			EnsureLua:   ensureLua,
			LuaCall: func(ctx context.Context, function string, params map[string]any) (any, error) {
				if luaEngine == nil {
					return nil, fmt.Errorf("lua engine not running")
				}
				return luaEngine.CallFunction(ctx, node.ID(), function, params)
			},
			Call: callMgr,
			Cluster:         clusterMgr,
			GQL:             gqlEngine,
			DataFed:         dataFedMgr,
			TemplateHandler: tplHandler,
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
			warmAvatar(m.PeerID, m.AvatarHash)
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
			go node.ProbeAllPeers(ctx)
		} else {
			mqMgr.PublishLocal("relay:status", "", map[string]any{
				"status": "lost",
				"msg":    "Relay circuit lost — recovering...",
			})
		}
	})
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
	avatarCache.Clear()
	return nil
}
