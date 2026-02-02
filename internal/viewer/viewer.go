// internal/viewer/viewer.go
package viewer

import (
	"net/http"

	"goop/internal/avatar"
	"goop/internal/chat"
	"goop/internal/content"
	"goop/internal/group"
	"goop/internal/p2p"
	"goop/internal/rendezvous"
	"goop/internal/state"
	"goop/internal/storage"
	viewerassets "goop/internal/ui/assets"
	"goop/internal/ui/render"
	"goop/internal/viewer/routes"
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
	Groups  *group.Manager
	DB      *storage.DB // SQLite database for peer data

	AvatarStore *avatar.Store
	AvatarCache *avatar.Cache

	// NEW: canonical base URL for templates (e.g. http://127.0.0.1:7777)
	BaseURL string

	PeerDir string // root directory for this peer's data

	RVClients []*rendezvous.Client
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

	routes.Register(mux, routes.Deps{
		Node:        v.Node,
		SelfLabel:   v.SelfLabel,
		SelfEmail:   v.SelfEmail,
		Peers:       v.Peers,
		CfgPath:     v.CfgPath,
		Cfg:         v.Cfg,
		Logs:        v.Logs,
		Content:     v.Content,
		BaseURL:     baseURL,
		DB:          v.DB,
		AvatarStore: v.AvatarStore,
		AvatarCache: v.AvatarCache,
		PeerDir:     v.PeerDir,
		RVClients:   v.RVClients,
	})

	// Register chat endpoints if chat manager is available
	if v.Chat != nil {
		routes.RegisterChat(mux, v.Chat)
	}

	// Register data/storage endpoints if DB is available
	if v.DB != nil {
		routes.RegisterData(mux, v.DB, v.Node.ID(), v.SelfEmail)
	}

	// Register group endpoints if group manager is available
	if v.Groups != nil {
		routes.RegisterGroups(mux, v.Groups, v.Node.ID())
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
	})

	return http.ListenAndServe(addr, mux)
}
