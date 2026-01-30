// internal/ui/viewmodels/base.go

package viewmodels

type BaseVM struct {
	Title       string
	Active      string
	SelfName    string
	SelfID      string
	ContentTmpl string
	BaseURL     string
	Debug       bool
	Theme       string

	// Rendezvous-only mode (no p2p node, limited nav)
	RendezvousOnly bool
	RendezvousURL  string
}
