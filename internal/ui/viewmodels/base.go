
package viewmodels

type BaseVM struct {
	Title       string
	Active      string
	SelfName    string
	SelfEmail   string
	SelfID      string
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
}
