// internal/app/prompt.go
package app

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/petervdpas/goop2/internal/config"
)

func PromptInteractive(peerDir, cfgPath string, cfg config.Config) config.Config {
	in := bufio.NewReader(os.Stdin)

	fmt.Println("────────────────────────────────────────")
	fmt.Println("Goop interactive setup")
	fmt.Printf(" Peer folder : %s\n", peerDir)
	fmt.Printf(" Config file : %s\n", cfgPath)
	fmt.Println("────────────────────────────────────────")
	fmt.Println()

	cfg.Profile.Label = askString(in, "Label", cfg.Profile.Label)
	cfg.Viewer.HTTPAddr = askString(in, "Viewer HTTP addr (empty=off)", cfg.Viewer.HTTPAddr)

	cfg.Presence.RendezvousHost = askBool(in, "Run local rendezvous service", cfg.Presence.RendezvousHost)
	if cfg.Presence.RendezvousHost {
		cfg.Presence.RendezvousPort = askInt(in, "Rendezvous port", cfg.Presence.RendezvousPort)
	}

	cfg.Presence.RendezvousWAN = askString(in, "WAN rendezvous URL (empty=off)", cfg.Presence.RendezvousWAN)
	cfg.Presence.RendezvousOnly = askBool(in, "Rendezvous-only (no peer)", cfg.Presence.RendezvousOnly)

	if cfg.Presence.RendezvousOnly {
		cfg.Presence.ExternalURL = askString(in, "External URL (empty=auto-detect)", cfg.Presence.ExternalURL)
	}

	if !cfg.Presence.RendezvousOnly {
		cfg.Presence.TTLSec = askInt(in, "Presence TTL seconds", cfg.Presence.TTLSec)
		cfg.Presence.HeartbeatSec = askInt(in, "Presence heartbeat seconds", cfg.Presence.HeartbeatSec)
		cfg.P2P.ListenPort = askInt(in, "Listen port (0=random)", cfg.P2P.ListenPort)
		cfg.P2P.MdnsTag = askString(in, "mDNS tag", cfg.P2P.MdnsTag)
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid config: %v\nKeeping defaults.\n", err)
		return config.Default()
	}
	return cfg
}

func askString(in *bufio.Reader, label, def string) string {
	fmt.Printf("%s [%s]: ", label, def)
	s, _ := in.ReadString('\n')
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	return s
}

func askInt(in *bufio.Reader, label string, def int) int {
	for {
		fmt.Printf("%s [%d]: ", label, def)
		s, _ := in.ReadString('\n')
		s = strings.TrimSpace(s)
		if s == "" {
			return def
		}
		if v, err := strconv.Atoi(s); err == nil {
			return v
		}
		fmt.Println("Please enter a number.")
	}
}

func askBool(in *bufio.Reader, label string, def bool) bool {
	defStr := "n"
	if def {
		defStr = "y"
	}
	for {
		fmt.Printf("%s [y/n] (default=%s): ", label, defStr)
		s, _ := in.ReadString('\n')
		s = strings.TrimSpace(strings.ToLower(s))
		if s == "" {
			return def
		}
		switch s {
		case "y", "yes", "true", "1":
			return true
		case "n", "no", "false", "0":
			return false
		default:
			fmt.Println("Please enter y or n.")
		}
	}
}
