package viewer

import (
	"context"
	"net/http"

	"github.com/petervdpas/goop2/internal/avatar"
	"github.com/petervdpas/goop2/internal/call"
	"github.com/petervdpas/goop2/internal/directchat"
	"github.com/petervdpas/goop2/internal/group_types/chat"
	"github.com/petervdpas/goop2/internal/group_types/cluster"
	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/group_types/datafed"
	"github.com/petervdpas/goop2/internal/orm/gql"
	"github.com/petervdpas/goop2/internal/content"
	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/group_types/files"
	"github.com/petervdpas/goop2/internal/group_types/listen"
	templateType "github.com/petervdpas/goop2/internal/group_types/template"
	"github.com/petervdpas/goop2/internal/mq"
	"github.com/petervdpas/goop2/internal/p2p"
	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/sdk"
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
	MQ      *mq.Manager
	Groups  *group.Manager
	Listen    *listen.Manager
	ChatRooms *chat.Manager
	DB      *storage.DB  // SQLite database for peer data
	Docs    *files.Store // shared documents store

	AvatarStore *avatar.Store
	AvatarCache *avatar.Cache

	BaseURL string

	PeerDir string // root directory for this peer's data

	RVClients []*rendezvous.Client

	// Canonical peer identity resolver — single instance shared by all routes.
	ResolvePeer func(string) state.PeerIdentityPayload

	// DirectChat manager — owns message persistence, Lua dispatch, history endpoints.
	DirectChat *directchat.Manager

	// Wails bridge URL for native dialogs (empty when not running in Wails)
	BridgeURL string

	// EnsureLua starts the Lua engine if needed and rescans functions.
	EnsureLua func()

	// LuaCall invokes a named Lua data function as the local peer.
	LuaCall func(ctx context.Context, function string, params map[string]any) (any, error)

	// Call manager for native Go/Pion WebRTC (nil = use browser WebRTC).
	// Set automatically on Linux; nil on all other platforms.
	Call *call.Manager

	// Cluster compute manager (nil when cluster not configured).
	Cluster *cluster.Manager

	// GraphQL engine for data federation (nil when DB not available).
	GQL *gql.Engine

	// Data federation manager (nil when not available).
	DataFed *datafed.Manager

	// Template group handler.
	TemplateHandler *templateType.Handler
}

func Start(addr string, v Viewer) error {
	if err := render.InitTemplates(); err != nil {
		return err
	}

	mux := http.NewServeMux()

	mux.Handle("/assets/", http.StripPrefix("/assets/",
		noCache(viewerassets.Handler()),
	))
	mux.Handle("/sdk/", http.StripPrefix("/sdk/",
		noCache(sdk.Handler()),
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
		ResolvePeer:     v.ResolvePeer,
		DocsStore:    v.Docs,
		GroupManager:    v.Groups,
		TemplateHandler: v.TemplateHandler,
		EnsureLua:       v.EnsureLua,
		LuaCall:         v.LuaCall,
	}
	routes.Register(mux, deps)

	// Register MQ endpoints
	if v.MQ != nil {
		var onChatSent func(string, string)
		if v.DirectChat != nil {
			onChatSent = v.DirectChat.PersistOutbound
		}
		routes.RegisterMQ(mux, v.MQ, onChatSent)
		routes.RegisterChat(mux, v.DirectChat)
	}

	// Register data/storage endpoints if DB is available
	var rebuildGQL func()
	if v.GQL != nil {
		rebuildGQL = func() { _ = v.GQL.Rebuild() }
	}
	if v.DB != nil {
		routes.RegisterData(mux, v.DB, v.Node.ID(), v.SelfEmail, rebuildGQL)
		routes.RegisterGraphQL(mux, v.GQL)
	}

	// Register transformation + schema endpoints (file-based, in peerDir)
	if v.PeerDir != "" {
		routes.RegisterTransformations(mux, v.PeerDir, v.DB)
		routes.RegisterSchema(mux, v.PeerDir, v.DB, rebuildGQL)
	}

	// Register group endpoints if group manager is available
	if v.Groups != nil {
		routes.RegisterGroups(mux, v.Groups, v.Node.ID(), v.ResolvePeer, v.MQ)
	}

	// Register filesystem browsing
	routes.RegisterFS(mux)

	// Register cluster compute endpoints
	if v.Cluster != nil {
		routes.RegisterCluster(mux, v.Cluster, v.Groups, v.Node.ID(), func(path, mode string) {
			if cfg, err := config.Load(v.CfgPath); err == nil {
				cfg.Viewer.ClusterBinaryPath = path
				cfg.Viewer.ClusterBinaryMode = mode
				_ = config.Save(v.CfgPath, cfg)
			}
		})
	}

	// Register native call endpoints (always register mode endpoint; full API when Call != nil)
	routes.RegisterCall(mux, v.Call, v.MQ)

	// Register listen room endpoints if listen manager is available
	if v.Listen != nil {
		routes.RegisterListen(mux, v.Listen, v.ResolvePeer)
	}

	// Register chat room endpoints if chat manager is available
	if v.ChatRooms != nil {
		routes.RegisterChatRooms(mux, v.ChatRooms, v.ResolvePeer)
	}

	// Register data proxy for remote peer data operations
	if v.Node != nil {
		routes.RegisterDataProxy(mux, v.Node)
	}

	// Register data federation endpoints
	routes.RegisterDataFed(mux, v.DataFed)

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
	TopologyFunc  func() any
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
	mux.Handle("/sdk/", http.StripPrefix("/sdk/",
		noCache(sdk.Handler()),
	))

	baseURL := v.BaseURL
	if baseURL == "" {
		baseURL = "http://" + addr
	}

	routes.RegisterFS(mux)

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
		TopologyFunc:   v.TopologyFunc,
	})

	return http.ListenAndServe(addr, mux)
}
