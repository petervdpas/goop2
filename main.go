// main.go
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/petervdpas/goop2/internal/app"
	"github.com/petervdpas/goop2/internal/config"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

var (
	showHelp = flag.Bool("h", false, "Show help")
	version  = flag.Bool("version", false, "Show version")
)

// appVersion is set at build time via -ldflags "-X main.appVersion=x.y.z"
var appVersion = "dev"

func main() {
	flag.Parse()

	// Show version
	if *version {
		fmt.Printf("Goop² v%s\n", appVersion)
		return
	}

	// Show help
	if *showHelp {
		showUsage()
		return
	}

	args := flag.Args()

	// No arguments - run desktop UI
	if len(args) == 0 {
		runDesktopApp()
		return
	}

	// Parse command
	command := args[0]

	switch command {
	case "peer":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: peer command requires directory path")
			fmt.Fprintln(os.Stderr, "Usage: goop2 peer <peer-directory>")
			os.Exit(1)
		}
		runCLIPeer(args[1])

	case "rendezvous":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: rendezvous command requires directory path")
			fmt.Fprintln(os.Stderr, "Usage: goop2 rendezvous <peer-directory>")
			os.Exit(1)
		}
		runCLIRendezvous(args[1])

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n", command)
		fmt.Fprintln(os.Stderr)
		showUsage()
		os.Exit(1)
	}
}

func runDesktopApp() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Goop²  ·  ephemeral web",
		Width:  1200,
		Height: 800,

		AssetServer: &assetserver.Options{
			Assets: assets,
		},

		Linux: &linux.Options{
			Icon: appIcon,
		},

		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind:       []any{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func runCLIPeer(peerDirArg string) {
	// Resolve absolute path
	absDir, err := filepath.Abs(peerDirArg)
	if err != nil {
		log.Fatalf("Invalid peer directory: %v", err)
	}

	// Verify directory exists
	if stat, err := os.Stat(absDir); err != nil || !stat.IsDir() {
		log.Fatalf("Peer directory does not exist: %s", absDir)
	}

	// Load config
	cfgPath := filepath.Join(absDir, "goop.json")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Print banner
	printPeerBanner(absDir, cfgPath, cfg)

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("\nShutting down gracefully...")
		cancel()
	}()

	// Run peer
	if err := app.Run(ctx, app.Options{
		PeerDir: absDir,
		CfgPath: cfgPath,
		Cfg:     cfg,
	}); err != nil {
		log.Fatalf("Peer failed: %v", err)
	}
}

func runCLIRendezvous(peerDirArg string) {
	absDir, err := filepath.Abs(peerDirArg)
	if err != nil {
		log.Fatalf("Invalid peer directory: %v", err)
	}

	if stat, err := os.Stat(absDir); err != nil || !stat.IsDir() {
		log.Fatalf("Peer directory does not exist: %s", absDir)
	}

	cfgPath := filepath.Join(absDir, "goop.json")
	cfg, _, err := config.Ensure(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Force rendezvous mode regardless of what the config file says.
	cfg.Presence.RendezvousOnly = true
	cfg.Presence.RendezvousHost = true

	printPeerBanner(absDir, cfgPath, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("\nShutting down gracefully...")
		cancel()
	}()

	if err := app.Run(ctx, app.Options{
		PeerDir: absDir,
		CfgPath: cfgPath,
		Cfg:     cfg,
	}); err != nil {
		log.Fatalf("Rendezvous server failed: %v", err)
	}
}

func showUsage() {
	fmt.Println("Goop² - Ephemeral Web")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  goop2                      Run desktop application (default)")
	fmt.Println("  goop2 peer <directory>     Run peer in CLI mode")
	fmt.Println("  goop2 rendezvous <directory>  Run peer configured as rendezvous server")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  peer <directory>")
	fmt.Println("        Run a peer from the specified directory without GUI")
	fmt.Println("        The directory must contain a goop.json configuration file")
	fmt.Println()
	fmt.Println("  rendezvous <directory>")
	fmt.Println("        Run a peer configured as rendezvous server")
	fmt.Println("        The peer's goop.json should have rendezvousHost enabled")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -h        Show this help message")
	fmt.Println("  -version  Show version information")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Run desktop app")
	fmt.Println("  goop2")
	fmt.Println()
	fmt.Println("  # Run a peer from CLI")
	fmt.Println("  goop2 peer ./peers/mysite")
	fmt.Println()
	fmt.Println("  # Run peer as rendezvous server")
	fmt.Println("  goop2 rendezvous ./peers/server")
	fmt.Println()
	fmt.Println("Documentation:")
	fmt.Println("  • Desktop usage: README.md")
	fmt.Println("  • CLI deployment: docs/CLI_TOOLS.md")
	fmt.Println("  • Rendezvous setup: docs/RENDEZVOUS_DEPLOYMENT.md")
}

func printPeerBanner(peerDir, cfgPath string, cfg config.Config) {
	fmt.Println("╔════════════════════════════════════════════════════════╗")
	fmt.Println("║                   Goop² Peer Runner                    ║")
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("Peer Directory: %s\n", peerDir)
	fmt.Printf("Config File:    %s\n", cfgPath)
	if cfg.Profile.Label != "" {
		fmt.Printf("Peer Label:     %s\n", cfg.Profile.Label)
	}
	fmt.Println()

	// Rendezvous monitoring (if hosting)
	if cfg.Presence.RendezvousHost {
		fmt.Println("┌─────────────────────────────────────────────────────┐")
		fmt.Printf("│ 📊 RENDEZVOUS MONITOR: http://127.0.0.1:%d        │\n", cfg.Presence.RendezvousPort)
		fmt.Println("└─────────────────────────────────────────────────────┘")
		fmt.Println()
		if cfg.Presence.RendezvousOnly {
			fmt.Println("Mode: Rendezvous Only (not serving content)")
		}
	}

	// Viewer URL (if serving content)
	if cfg.Viewer.HTTPAddr != "" && !cfg.Presence.RendezvousOnly {
		viewerURL := cfg.Viewer.HTTPAddr
		if viewerURL[0] == ':' {
			viewerURL = "http://127.0.0.1" + viewerURL
		}
		fmt.Printf("🌐 Content Viewer:  %s\n", viewerURL)
		fmt.Println()
	}

	if cfg.Presence.RendezvousWAN != "" {
		fmt.Printf("WAN Bridge:     %s\n", cfg.Presence.RendezvousWAN)
	}

	fmt.Println("Starting peer... (Press Ctrl+C to stop)")
	fmt.Println("────────────────────────────────────────────────────────")
	fmt.Println()
}
