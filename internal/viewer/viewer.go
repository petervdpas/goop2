// internal/viewer/viewer.go
package viewer

import (
	"net/http"

	"goop/internal/chat"
	"goop/internal/content"
	"goop/internal/p2p"
	"goop/internal/state"
	"goop/internal/storage"
	viewerassets "goop/internal/ui/assets"
	"goop/internal/ui/render"
	"goop/internal/viewer/routes"
)

type Viewer struct {
	Node      *p2p.Node
	SelfLabel func() string
	Peers     *state.PeerTable

	CfgPath string
	Cfg     interface{} // Config interface to avoid import cycle
	Logs    *LogBuffer
	Content *content.Store
	Chat    *chat.Manager
	DB      *storage.DB // SQLite database for peer data

	// NEW: canonical base URL for templates (e.g. http://127.0.0.1:7777)
	BaseURL string
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
		Node:      v.Node,
		SelfLabel: v.SelfLabel,
		Peers:     v.Peers,
		CfgPath:   v.CfgPath,
		Cfg:       v.Cfg,
		Logs:      v.Logs,
		Content:   v.Content,
		BaseURL:   baseURL,
		DB:        v.DB,
	})

	// Register chat endpoints if chat manager is available
	if v.Chat != nil {
		routes.RegisterChat(mux, v.Chat)
	}

	// Register data/storage endpoints if DB is available
	if v.DB != nil {
		routes.RegisterData(mux, v.DB, v.Node.ID())
	}

	return http.ListenAndServe(addr, mux)
}
