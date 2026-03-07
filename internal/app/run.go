package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/nacl/box"

	"github.com/petervdpas/goop2/internal/app/modes"
	"github.com/petervdpas/goop2/internal/app/shared"
	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/rendezvous"
	"github.com/petervdpas/goop2/internal/util"
	"github.com/petervdpas/goop2/internal/viewer"
)

type Options struct {
	PeerDir   string
	CfgPath   string
	Cfg       config.Config
	BridgeURL string
	Progress  func(step, total int, label string)
}

func Run(ctx context.Context, opt Options) error {
	logBuf := viewer.NewLogBuffer(800)
	log.SetOutput(logBuf)

	logBanner(opt.PeerDir, opt.CfgPath)

	mo := shared.ModeOpts{
		PeerDir:   opt.PeerDir,
		CfgPath:   opt.CfgPath,
		Logs:      logBuf,
		BridgeURL: opt.BridgeURL,
	}
	return runPeer(ctx, mo, opt.Cfg, opt.Progress)
}

func runPeer(ctx context.Context, o shared.ModeOpts, cfg config.Config, progress func(int, int, string)) error {
	if cfg.P2P.NaClPublicKey == "" || cfg.P2P.NaClPrivateKey == "" {
		pub, priv, err := box.GenerateKey(rand.Reader)
		if err != nil {
			return fmt.Errorf("generate NaCl keypair: %w", err)
		}
		cfg.P2P.NaClPublicKey = base64.StdEncoding.EncodeToString(pub[:])
		cfg.P2P.NaClPrivateKey = base64.StdEncoding.EncodeToString(priv[:])
		if err := config.Save(o.CfgPath, cfg); err != nil {
			return fmt.Errorf("save NaCl keypair: %w", err)
		}
		log.Printf("NaCl keypair generated and persisted")
	}

	emit := progress
	if emit == nil {
		emit = func(int, int, string) {}
	}
	progress = func(s, t int, label string) {
		emit(s, t, label)
		time.Sleep(time.Second)
	}

	// Calculate total steps based on config.
	// Rendezvous-only: rendezvous + viewer = 2 steps.
	// Full peer: rendezvous(opt) + relay discovery + p2p node + database + services + viewer.
	step := 0
	total := 6 // relay + p2p + db + services + viewer + online
	if cfg.Presence.RendezvousHost {
		total++ // rendezvous server
	}
	if cfg.Presence.RendezvousOnly {
		total = 2 // rendezvous + viewer only
		if !cfg.Presence.RendezvousHost {
			total = 1 // viewer only
		}
	}

	// ── Rendezvous server (optional)
	var rv *rendezvous.Server
	if cfg.Presence.RendezvousHost {
		bind := cfg.Presence.RendezvousBind
		if bind == "" {
			bind = "127.0.0.1"
		}
		addr := fmt.Sprintf("%s:%d", bind, cfg.Presence.RendezvousPort)

		peerDBPath := ""
		if cfg.Presence.PeerDBPath != "" {
			peerDBPath = util.ResolvePath(o.PeerDir, cfg.Presence.PeerDBPath)
		}

		relayKeyFile := ""
		if cfg.Presence.RelayKeyFile != "" {
			relayKeyFile = util.ResolvePath(o.PeerDir, cfg.Presence.RelayKeyFile)
		}
		rv = rendezvous.New(addr, peerDBPath, cfg.Presence.AdminPassword, cfg.Presence.ExternalURL, cfg.Presence.RelayPort, relayKeyFile, rendezvous.RelayTimingConfig{
			CleanupDelaySec:    cfg.Presence.RelayCleanupDelaySec,
			PollDeadlineSec:    cfg.Presence.RelayPollDeadlineSec,
			ConnectTimeoutSec:  cfg.Presence.RelayConnectTimeoutSec,
			RefreshIntervalSec: cfg.Presence.RelayRefreshIntervalSec,
			RecoveryGraceSec:   cfg.Presence.RelayRecoveryGraceSec,
		})

		// Wire external services (credits + registration + email + templates)
		if cfg.Presence.UseServices {
			setupMicroService("Credits", cfg.Presence.CreditsURL, func() {
				rv.SetCreditProvider(rendezvous.NewRemoteCreditProvider(
					cfg.Presence.CreditsURL, rv.GetEmailForPeer, rv.GetTokenForPeer, cfg.Presence.CreditsAdminToken))
			})
			setupMicroService("Registration", cfg.Presence.RegistrationURL, func() {
				rv.SetRegistrationProvider(rendezvous.NewRemoteRegistrationProvider(
					cfg.Presence.RegistrationURL, cfg.Presence.RegistrationAdminToken))
			})
			setupMicroService("Email", cfg.Presence.EmailURL, func() {
				rv.SetEmailProvider(rendezvous.NewRemoteEmailProvider(cfg.Presence.EmailURL))
			})
			setupMicroService("Templates", cfg.Presence.TemplatesURL, func() {
				rv.SetTemplatesProvider(rendezvous.NewRemoteTemplatesProvider(
					cfg.Presence.TemplatesURL, cfg.Presence.TemplatesAdminToken))
			})
			setupMicroService("Bridge", cfg.Presence.BridgeURL, func() {
				rv.SetBridgeProvider(rendezvous.NewRemoteBridgeProvider(
					cfg.Presence.BridgeURL, cfg.Presence.BridgeAdminToken))
			})
			setupMicroService("Encryption", cfg.Presence.EncryptionURL, func() {
				rv.SetEncryptionProvider(rendezvous.NewRemoteEncryptionProvider(
					cfg.Presence.EncryptionURL, cfg.Presence.EncryptionAdminToken))
			})
		}

		// Local template store fallback (works with or without services)
		if cfg.Presence.TemplatesDir != "" && (cfg.Presence.TemplatesURL == "" || !cfg.Presence.UseServices) {
			dir := util.ResolvePath(o.PeerDir, cfg.Presence.TemplatesDir)
			if store := rendezvous.NewLocalTemplateStore(dir); store != nil {
				log.Printf("Local template store: %s (%d templates)", dir, store.Count())
				rv.SetLocalTemplateStore(store)
			}
		}

		step++
		progress(step, total, "Starting rendezvous server")

		if err := rv.Start(ctx); err != nil {
			return err
		}
		log.Println("────────────────────────────────────────────────────────")
		log.Printf("🌐 Rendezvous Server: %s", rv.URL())
		log.Printf("📊 Monitor connected peers: %s", rv.URL())
		log.Println("────────────────────────────────────────────────────────")
	}

	selfContent := func() string {
		if cfg.Profile.Label != "" {
			return cfg.Profile.Label
		}
		return "hello"
	}

	selfEmail := func() string {
		return cfg.Profile.Email
	}

	selfVideoDisabled := func() bool {
		return cfg.Viewer.VideoDisabled
	}

	selfActiveTemplate := func() string {
		if c, err := config.LoadPartial(o.CfgPath); err == nil {
			return c.Viewer.ActiveTemplate
		}
		return cfg.Viewer.ActiveTemplate
	}

	selfVerificationToken := func() string {
		if c, err := config.LoadPartial(o.CfgPath); err == nil {
			return c.Profile.VerificationToken
		}
		return cfg.Profile.VerificationToken
	}

	if cfg.Presence.RendezvousOnly {
		return modes.RunRendezvous(ctx, o, cfg, rv, selfContent, selfEmail, progress)
	}

	if cfg.P2P.BridgeMode {
		return modes.RunBridge(ctx, o, cfg, selfContent, selfEmail, progress)
	}

	selfPublicKey := func() string { return cfg.P2P.NaClPublicKey }

	return modes.RunPeer(modes.PeerParams{
		Ctx:                   ctx,
		ModeOpts:              o,
		Cfg:                   cfg,
		SelfContent:           selfContent,
		SelfEmail:             selfEmail,
		SelfVideoDisabled:     selfVideoDisabled,
		SelfActiveTemplate:    selfActiveTemplate,
		SelfPublicKey:         selfPublicKey,
		SelfVerificationToken: selfVerificationToken,
		Progress:              progress,
		Step:                  step,
		Total:                 total,
	})
}
