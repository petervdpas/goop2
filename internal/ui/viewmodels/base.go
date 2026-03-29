
package viewmodels

type BaseVM struct {
	Title                 string
	Active                string
	SelfName              string
	SelfEmail             string
	SelfVerificationToken string
	SelfID                string
	ContentTmpl string
	BaseURL     string
	Debug       bool
	Theme       string

	// Rendezvous-only mode (no p2p node, limited nav)
	RendezvousOnly bool
	RendezvousURL  string

	// Wails bridge URL for native dialogs (empty when not running in Wails)
	BridgeURL string

	// Platform detection for UI warnings (e.g. "linux", "windows", "darwin")
	WhichOS string

	// When true, peer sites open in system browser instead of embedded tabs
	OpenSitesExternal bool

	// Server startup ID — used by JS to clear stale sessionStorage on restart
	AppRunID string

	// Split pane positions — JSON string hydrated into data attribute
	SplitPrefs string
}
