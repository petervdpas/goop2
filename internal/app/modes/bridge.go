package modes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/petervdpas/goop2/internal/app/shared"
	"github.com/petervdpas/goop2/internal/avatar"
	"github.com/petervdpas/goop2/internal/bridge"
	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/p2p"
	"github.com/petervdpas/goop2/internal/proto"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/util"
	"github.com/petervdpas/goop2/internal/viewer"
)

// RunBridge starts goop2 in thin-client mode. No libp2p, no entangler.
// All traffic flows through the bridge service via WSS.
func RunBridge(ctx context.Context, o shared.ModeOpts, cfg config.Config, selfContent, selfEmail func() string, progress func(int, int, string)) error {
	log.Printf("mode: thin-client (bridge)")

	bridgeURL := cfg.Presence.BridgeURL
	if bridgeURL == "" {
		return fmt.Errorf("bridge_mode requires bridge_url to be set")
	}

	keyPath := util.ResolvePath(o.PeerDir, cfg.Identity.KeyFile)
	peerID, err := p2p.PeerIDFromKeyFile(keyPath)
	if err != nil {
		return fmt.Errorf("load identity for bridge: %w", err)
	}
	log.Printf("peer id: %s (thin-client)", peerID)

	peers := state.NewPeerTable()
	avatarStore := avatar.NewStore(o.PeerDir)

	bc := bridge.New(
		bridgeURL,
		cfg.Profile.Email,
		cfg.Profile.BridgeToken,
		peerID,
		selfContent(),
		cfg.P2P.NaClPublicKey,
		cfg.P2P.NaClPrivateKey != "",
		peers,
	)

	go bc.Connect(ctx, func(data json.RawMessage) {
		var pm proto.PresenceMsg
		if json.Unmarshal(data, &pm) != nil {
			return
		}
		if pm.PeerID == "" || pm.PeerID == peerID {
			return
		}
		switch pm.Type {
		case proto.TypeOnline, proto.TypeUpdate:
			existing, _ := peers.Get(pm.PeerID)
			peers.Upsert(pm.PeerID, pm.Content, pm.Email, pm.AvatarHash, pm.VideoDisabled, pm.ActiveTemplate, pm.PublicKey, pm.EncryptionSupported, existing.Verified, pm.GoopClientVersion)
			peers.SetReachable(pm.PeerID, true)
		case proto.TypeOffline:
			peers.MarkOffline(pm.PeerID)
		}
	})

	progress(1, 2, "Starting viewer")

	if cfg.Viewer.HTTPAddr != "" {
		addr, url, _ := shared.NormalizeLocalViewer(cfg.Viewer.HTTPAddr)
		go viewer.StartMinimal(addr, viewer.MinimalViewer{
			SelfLabel:   selfContent,
			SelfEmail:   selfEmail,
			CfgPath:     o.CfgPath,
			Cfg:         cfg,
			Logs:        o.Logs,
			BaseURL:     url,
			AvatarStore: avatarStore,
			BridgeURL:   o.BridgeURL,
		})
		log.Printf("🌉 Thin-client viewer: %s (bridge: %s)", url, bridgeURL)
	}

	<-ctx.Done()
	return nil
}
