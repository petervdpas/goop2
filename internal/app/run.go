// internal/app/run.go
package app

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"goop/internal/avatar"
	"goop/internal/chat"
	"goop/internal/config"
	"goop/internal/content"
	"goop/internal/group"
	luapkg "goop/internal/lua"
	"goop/internal/p2p"
	"goop/internal/proto"
	"goop/internal/rendezvous"
	"goop/internal/state"
	"goop/internal/storage"
	"goop/internal/util"
	"goop/internal/viewer"
)

type Options struct {
	PeerDir string
	CfgPath string
	Cfg     config.Config
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
		PeerDir: opt.PeerDir,
		CfgPath: opt.CfgPath,
		Cfg:     opt.Cfg,
		Logs:    logBuf,
	})
}

type runPeerOpts struct {
	PeerDir string
	CfgPath string
	Cfg     config.Config
	Logs    *viewer.LogBuffer
}

func runPeer(ctx context.Context, o runPeerOpts) error {
	cfg := o.Cfg

	// ── Rendezvous server (optional)
	var rv *rendezvous.Server
	if cfg.Presence.RendezvousHost {
		addr := fmt.Sprintf("127.0.0.1:%d", cfg.Presence.RendezvousPort)
		rv = rendezvous.New(addr)
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

	if cfg.Presence.RendezvousOnly {
		log.Printf("mode: rendezvous-only")

		// Start minimal viewer for settings access
		if cfg.Viewer.HTTPAddr != "" {
			addr, url, _ := NormalizeLocalViewer(cfg.Viewer.HTTPAddr)
			rvURL := ""
			if rv != nil {
				rvURL = rv.URL()
			}
			go viewer.StartMinimal(addr, viewer.MinimalViewer{
				SelfLabel:      selfContent,
				SelfEmail:      selfEmail,
				CfgPath:        o.CfgPath,
				Cfg:            cfg,
				Logs:           o.Logs,
				BaseURL:        url,
				RendezvousURL:  rvURL,
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

	keyPath := filepath.Join(o.PeerDir, cfg.Identity.KeyFile)
	node, err := p2p.New(ctx, cfg.P2P.ListenPort, keyPath, peers, selfContent, selfEmail)
	if err != nil {
		return err
	}
	defer node.Close()

	node.EnableSite(filepath.Join(o.PeerDir, "site"))

	// ── Avatar store
	avatarStore := avatar.NewStore(o.PeerDir)
	avatarCache := avatar.NewCache(o.PeerDir)
	node.EnableAvatar(avatarStore)

	// Initialize SQLite database for peer data (unconditionally — needed for P2P data protocol)
	db, err := storage.Open(o.PeerDir)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	node.EnableData(db)
	log.Printf("peer id: %s", node.ID())

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

	for _, c := range rvClients {
		cc := c
		go cc.SubscribeEvents(ctx, func(pm proto.PresenceMsg) {
			if pm.PeerID == node.ID() {
				return
			}
			switch pm.Type {
			case proto.TypeOnline, proto.TypeUpdate:
				peers.Upsert(pm.PeerID, pm.Content, pm.Email, pm.AvatarHash)
			case proto.TypeOffline:
				peers.Remove(pm.PeerID)
			}
		})
	}

	publish := func(pctx context.Context, typ string) {
		node.Publish(pctx, typ)
		for _, c := range rvClients {
			cc := c
			go func() {
				ctx2, cancel := context.WithTimeout(pctx, util.ShortTimeout)
				defer cancel()
				_ = cc.Publish(ctx2, proto.PresenceMsg{
					Type:       typ,
					PeerID:     node.ID(),
					Content:    selfContent(),
					Email:      selfEmail(),
					AvatarHash: avatarStore.Hash(),
					TS:         proto.NowMillis(),
				})
			}()
		}
	}

	// ── Viewer
	if cfg.Viewer.HTTPAddr != "" {
		addr, url, _ := NormalizeLocalViewer(cfg.Viewer.HTTPAddr)
		store, err := content.NewStore(o.PeerDir, "site")
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
			DB:          db,
			BaseURL:     url,
			AvatarStore: avatarStore,
			AvatarCache: avatarCache,
			PeerDir:     o.PeerDir,
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
