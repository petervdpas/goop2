package modes

import (
	"context"
	"log"

	"github.com/petervdpas/goop2/internal/app/shared"
	"github.com/petervdpas/goop2/internal/avatar"
	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/viewer"
)

// RunRendezvous starts goop2 in rendezvous-only mode.
// No P2P node — just the rendezvous server and a minimal settings viewer.
func RunRendezvous(ctx context.Context, o shared.ModeOpts, cfg config.Config, rv *rendezvous.Server, selfContent, selfEmail func() string, progress func(int, int, string)) error {
	log.Printf("mode: rendezvous-only")

	progress(1, 2, "Starting viewer")

	if cfg.Viewer.HTTPAddr != "" {
		addr, url, _ := shared.NormalizeLocalViewer(cfg.Viewer.HTTPAddr)
		rvURL := ""
		if rv != nil {
			rvURL = rv.URL()
		}
		avatarStore := avatar.NewStore(o.PeerDir)
		var topoFn func() any
		if rv != nil {
			topoFn = func() any { return rv.Topology() }
		}
		go viewer.StartMinimal(addr, viewer.MinimalViewer{
			SelfLabel:     selfContent,
			SelfEmail:     selfEmail,
			CfgPath:       o.CfgPath,
			Cfg:           cfg,
			Logs:          o.Logs,
			BaseURL:       url,
			RendezvousURL: rvURL,
			AvatarStore:   avatarStore,
			BridgeURL:     o.BridgeURL,
			TopologyFunc:  topoFn,
		})
		log.Printf("📋 Settings viewer: %s", url)
	}

	<-ctx.Done()
	return nil
}
