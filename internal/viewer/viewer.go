// internal/viewer/viewer.go
package viewer

import (
	"net/http"

	"goop/internal/content"
	"goop/internal/p2p"
	"goop/internal/state"
	viewerassets "goop/internal/ui/assets"
	"goop/internal/ui/render"
	"goop/internal/viewer/routes"
)

type Viewer struct {
	Node      *p2p.Node
	SelfLabel func() string
	Peers     *state.PeerTable

	CfgPath string
	Logs    *LogBuffer
	Content *content.Store

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
		Logs:      v.Logs,
		Content:   v.Content,
		BaseURL:   baseURL,
	})

	return http.ListenAndServe(addr, mux)
}
