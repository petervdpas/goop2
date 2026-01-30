// internal/viewer/routes/register.go
package routes

import (
	"net/http"

	"goop/internal/content"
	"goop/internal/p2p"
	"goop/internal/state"
	"goop/internal/storage"
)

type Logs interface {
	ServeLogsJSON(w http.ResponseWriter, r *http.Request)
	ServeLogsSSE(w http.ResponseWriter, r *http.Request)
}

type Deps struct {
	Node      *p2p.Node
	SelfLabel func() string
	SelfEmail func() string
	Peers     *state.PeerTable

	CfgPath string
	Cfg     interface{} // Config interface to avoid import cycle
	Logs    Logs
	Content *content.Store
	BaseURL string
	DB      *storage.DB
}

func Register(mux *http.ServeMux, d Deps) {
	csrf := newToken(32)

	RegisterOpenRoute(mux)

	registerAPILogRoutes(mux, d)

	registerHomeRoutes(mux, d)
	registerPeerRoutes(mux, d)
	registerSelfRoutes(mux, d, csrf)
	registerEditorRoutes(mux, d, csrf)
	registerSettingsRoutes(mux, d, csrf)
	registerLogsUIRoutes(mux, d)
	registerDatabaseRoutes(mux, d)
	registerOfflineRoutes(mux, d)
	registerSiteAPIRoutes(mux, d)
	registerTemplateRoutes(mux, d, csrf)
}
