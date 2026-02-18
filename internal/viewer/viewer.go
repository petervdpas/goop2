package viewer

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/avatar"
	"github.com/petervdpas/goop2/internal/chat"
	"github.com/petervdpas/goop2/internal/content"
	"github.com/petervdpas/goop2/internal/docs"
	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/listen"
	"github.com/petervdpas/goop2/internal/p2p"
	"github.com/petervdpas/goop2/internal/realtime"
	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
	viewerassets "github.com/petervdpas/goop2/internal/ui/assets"
	"github.com/petervdpas/goop2/internal/ui/render"
	"github.com/petervdpas/goop2/internal/viewer/routes"
)

type Viewer struct {
	Node      *p2p.Node
	SelfLabel func() string
	SelfEmail func() string
	Peers     *state.PeerTable

	CfgPath string
	Cfg     any // Config interface to avoid import cycle
	Logs    *LogBuffer
	Content *content.Store
	Chat    *chat.Manager
	Groups   *group.Manager
	Realtime *realtime.Manager
	Listen   *listen.Manager
	DB       *storage.DB // SQLite database for peer data
	Docs    *docs.Store // shared documents store

	AvatarStore *avatar.Store
	AvatarCache *avatar.Cache

	// NEW: canonical base URL for templates (e.g. http://127.0.0.1:7777)
	BaseURL string

	PeerDir string // root directory for this peer's data

	RVClients []*rendezvous.Client

	// Wails bridge URL for native dialogs (empty when not running in Wails)
	BridgeURL string

	// EnsureLua starts the Lua engine if needed and rescans functions.
	EnsureLua func()
}

func Start(addr string, v Viewer) error {
	if err := render.InitTemplates(); err != nil {
		return err
	}

	mux := http.NewServeMux()

	mux.Handle("/assets/", http.StripPrefix("/assets/",
		noCache(viewerassets.Handler()),
	))
	mux.HandleFunc("/p/", proxyPeerSite(v))

	baseURL := v.BaseURL
	if baseURL == "" {
		// fallback (should not happen if wired correctly)
		baseURL = "http://" + addr
	}

	deps := routes.Deps{
		Node:         v.Node,
		SelfLabel:    v.SelfLabel,
		SelfEmail:    v.SelfEmail,
		Peers:        v.Peers,
		CfgPath:      v.CfgPath,
		Cfg:          v.Cfg,
		Logs:         v.Logs,
		Content:      v.Content,
		BaseURL:      baseURL,
		DB:           v.DB,
		AvatarStore:  v.AvatarStore,
		AvatarCache:  v.AvatarCache,
		PeerDir:      v.PeerDir,
		RVClients:    v.RVClients,
		BridgeURL:    v.BridgeURL,
		DocsStore:    v.Docs,
		GroupManager: v.Groups,
		EnsureLua:    v.EnsureLua,
	}
	routes.Register(mux, deps)

	// Register chat endpoints if chat manager is available
	if v.Chat != nil {
		routes.RegisterChat(mux, v.Chat, v.Peers)
	}

	// Register data/storage endpoints if DB is available
	if v.DB != nil {
		routes.RegisterData(mux, v.DB, v.Node.ID(), v.SelfEmail)
	}

	// Register group endpoints if group manager is available
	if v.Groups != nil {
		routes.RegisterGroups(mux, v.Groups, v.Node.ID(), func(id string) string {
			if id == v.Node.ID() {
				return v.SelfLabel()
			}
			if v.Peers == nil {
				return ""
			}
			if sp, ok := v.Peers.Snapshot()[id]; ok {
				return sp.Content
			}
			return ""
		})
	}

	// Register realtime channel endpoints if realtime manager is available
	if v.Realtime != nil {
		routes.RegisterRealtime(mux, v.Realtime, v.Node.ID())
	}

	// Register listen room endpoints if listen manager is available
	if v.Listen != nil {
		routes.RegisterListen(mux, v.Listen)
	}

	// Register data proxy for remote peer data operations
	if v.Node != nil {
		routes.RegisterDataProxy(mux, v.Node)
	}

	return http.ListenAndServe(addr, mux)
}

// MinimalViewer holds the config needed for a rendezvous-only settings viewer.
type MinimalViewer struct {
	SelfLabel     func() string
	SelfEmail     func() string
	CfgPath       string
	Cfg           any
	Logs          *LogBuffer
	BaseURL       string
	RendezvousURL string
	AvatarStore   *avatar.Store
	BridgeURL     string
}

// StartMinimal starts a lightweight viewer with only self/settings and logs.
// Used for rendezvous-only mode where there is no p2p node.
func StartMinimal(addr string, v MinimalViewer) error {
	if err := render.InitTemplates(); err != nil {
		return err
	}

	mux := http.NewServeMux()

	mux.Handle("/assets/", http.StripPrefix("/assets/",
		noCache(viewerassets.Handler()),
	))

	baseURL := v.BaseURL
	if baseURL == "" {
		baseURL = "http://" + addr
	}

	routes.RegisterMinimal(mux, routes.Deps{
		SelfLabel:      v.SelfLabel,
		SelfEmail:      v.SelfEmail,
		CfgPath:        v.CfgPath,
		Cfg:            v.Cfg,
		Logs:           v.Logs,
		BaseURL:        baseURL,
		RendezvousOnly: true,
		RendezvousURL:  v.RendezvousURL,
		AvatarStore:    v.AvatarStore,
		BridgeURL:      v.BridgeURL,
	})

	return http.ListenAndServe(addr, mux)
}
