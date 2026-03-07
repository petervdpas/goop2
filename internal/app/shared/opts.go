package shared

import (
	"strings"

	"github.com/petervdpas/goop2/internal/viewer"
)

// ModeOpts holds the common options passed to each run mode.
type ModeOpts struct {
	PeerDir   string
	CfgPath   string
	Logs      *viewer.LogBuffer
	BridgeURL string
}

// NormalizeLocalViewer ensures the viewer only binds to localhost
// and returns listen addr, browser URL, and TCP check addr.
func NormalizeLocalViewer(cfgAddr string) (listenAddr string, url string, tcpAddr string) {
	a := strings.TrimSpace(cfgAddr)
	if strings.HasPrefix(a, ":") {
		a = "127.0.0.1" + a
	}
	if strings.HasPrefix(a, "0.0.0.0:") {
		a = "127.0.0.1:" + strings.TrimPrefix(a, "0.0.0.0:")
	}
	listenAddr = a
	url = "http://" + a
	tcpAddr = a
	return
}
