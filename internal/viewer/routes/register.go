package routes

import (
	"net/http"

	"github.com/petervdpas/goop2/internal/avatar"
	"github.com/petervdpas/goop2/internal/content"
	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/group_types/files"
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

	// TopologyFunc returns topology data for the graph. Set by p2p node or rendezvous.
	TopologyFunc func() any

	// Document sharing
	DocsStore    *files.Store
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
		{"/groups/files", "Groups - Files", "groups", "page.groups_files"},
		{"/groups/cluster", "Groups - Cluster", "groups", "page.groups_cluster"},
		{"/groups/hosted", "Groups - Hosted", "groups", "page.groups_hosted"},
		{"/groups/joined", "Groups - Joined", "groups", "page.groups_joined"},
		{"/groups/events", "Groups - Events", "groups", "page.groups_events"},
		{"/view", "View", "view", "page.view"},
		{"/apidocs", "API Docs", "create", "page.apidocs"},
	})
	mux.HandleFunc("/groups", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, d.BaseURL+"/groups/hosted", http.StatusFound)
	})
	// Keep old routes as redirects
	mux.HandleFunc("/documents", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, d.BaseURL+"/groups/files", http.StatusFound)
	})
	mux.HandleFunc("/cluster", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, d.BaseURL+"/groups/cluster", http.StatusFound)
	})
	mux.HandleFunc("/self/groups", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, d.BaseURL+"/groups/hosted", http.StatusFound)
	})
	mux.HandleFunc("/create/groups", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, d.BaseURL+"/groups/hosted", http.StatusFound)
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

	// Topology graph — uses TopologyFunc when wired from rendezvous.
	handleGet(mux, "/api/topology", topologyHandler(d))

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
}
