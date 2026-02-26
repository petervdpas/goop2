package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/avatar"
	"github.com/petervdpas/goop2/internal/content"
	"github.com/petervdpas/goop2/internal/docs"
	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/p2p"
	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
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
	Cfg     any // Config interface to avoid import cycle
	Logs    Logs
	Content *content.Store
	BaseURL string
	DB      *storage.DB

	AvatarStore *avatar.Store
	AvatarCache *avatar.Cache

	// Rendezvous-only mode (no p2p node, limited routes)
	RendezvousOnly bool
	RendezvousURL  string

	PeerDir string // root directory for this peer's data

	RVClients []*rendezvous.Client

	// Wails bridge URL for native dialogs (empty when not running in Wails)
	BridgeURL string

	// Document sharing
	DocsStore    *docs.Store
	GroupManager *group.Manager

	// EnsureLua is called when a template with Lua files is applied.
	// It starts the Lua engine (if not already running) and rescans the
	// functions directory so scripts are available immediately.
	EnsureLua func()
}

func Register(mux *http.ServeMux, d Deps) {
	csrf := newToken(32)

	RegisterOpenRoute(mux)
	RegisterOpenAPI(mux)

	registerAPILogRoutes(mux, d)

	registerHomeRoutes(mux, d)
	registerPeerRoutes(mux, d)
	registerSelfRoutes(mux, d, csrf)
	registerEditorRoutes(mux, d, csrf)
	registerSettingsRoutes(mux, d, csrf)
	registerSimplePages(mux, d, []simplePage{
		{"/logs", "Logs", "logs", "page.logs"},
		{"/database", "Database", "database", "page.database"},
		{"/documents", "Files", "documents", "page.documents"},
		{"/self/groups", "Groups", "self", "page.groups"},
		{"/create/groups", "Create Groups", "create", "page.create_groups"},
		{"/view", "View", "view", "page.view"},
	})
	registerOfflineRoutes(mux, d)
	registerSiteAPIRoutes(mux, d)
	registerTemplateRoutes(mux, d, csrf)
	registerCreditsUIRoutes(mux, d)
	registerExportRoutes(mux, d, csrf)
	registerLuaRoutes(mux, d, csrf)
	registerDocsRoutes(mux, d)
	registerAvatarRoutes(mux, d)
}

// RegisterMinimal registers only the routes that work without a p2p node.
// Used for rendezvous-only mode.
func RegisterMinimal(mux *http.ServeMux, d Deps) {
	csrf := newToken(32)

	// Redirect / to /self (settings page)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/self", http.StatusFound)
	})

	RegisterOpenRoute(mux)
	RegisterOpenAPI(mux)
	registerAPILogRoutes(mux, d)
	registerSelfRoutes(mux, d, csrf)
	registerSettingsRoutes(mux, d, csrf)
	registerSimplePages(mux, d, []simplePage{
		{"/logs", "Logs", "logs", "page.logs"},
	})
	registerAvatarRoutes(mux, d)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
}
