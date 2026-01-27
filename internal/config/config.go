// internal/config/config.go
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"goop/internal/util"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Identity Identity `json:"identity"`
	Paths    Paths    `json:"paths"`
	P2P      P2P      `json:"p2p"`
	Presence Presence `json:"presence"`
	Profile  Profile  `json:"profile"`
	Viewer   Viewer   `json:"viewer"`
}

type Identity struct {
	KeyFile string `json:"key_file"`
}

type Paths struct {
	SiteRoot   string `json:"site_root"`
	SiteSource string `json:"site_source"`
	SiteStage  string `json:"site_stage"`
}

type P2P struct {
	ListenPort int    `json:"listen_port"`
	MdnsTag    string `json:"mdns_tag"`
}

type Presence struct {
	Topic        string `json:"topic"`
	TTLSec       int    `json:"ttl_seconds"`
	HeartbeatSec int    `json:"heartbeat_seconds"`

	// If true, run a local rendezvous service on 127.0.0.1:RendezvousPort.
	RendezvousHost bool `json:"rendezvous_host"`

	// Local rendezvous service port (used only when RendezvousHost=true).
	RendezvousPort int `json:"rendezvous_port"`

	// Optional WAN rendezvous address to join WAN presence mesh (LAN + WAN).
	// Example: https://rv.example.org  or  http://1.2.3.4:8787
	RendezvousWAN string `json:"rendezvous_wan"`

	// If true: run ONLY rendezvous server; do NOT start libp2p peer node.
	// This implies RendezvousHost=true and requires a valid RendezvousPort.
	RendezvousOnly bool `json:"rendezvous_only"`
}

type Profile struct {
	Label string `json:"label"`
}

type Viewer struct {
	HTTPAddr string `json:"http_addr"`
	Debug    bool   `json:"debug"`
}

func Default() Config {
	return Config{
		Identity: Identity{
			KeyFile: "data/identity.key",
		},
		Paths: Paths{
			SiteRoot:   "site",
			SiteSource: "site/src",
			SiteStage:  "site/stage",
		},
		P2P: P2P{
			ListenPort: 0,
			MdnsTag:    "goop-mdns",
		},
		Presence: Presence{
			Topic:          "goop.presence.v1",
			TTLSec:         20,
			HeartbeatSec:   5,
			RendezvousHost: false,
			RendezvousPort: 8787,
			RendezvousWAN:  "",
			RendezvousOnly: false,
		},
		Profile: Profile{
			Label: "hello",
		},
		Viewer: Viewer{
			HTTPAddr: "",
			Debug:    false,
		},
	}
}

func (c *Config) Validate() error {
	// Identity
	if strings.TrimSpace(c.Identity.KeyFile) == "" {
		return errors.New("identity.key_file is required")
	}

	// Paths
	if strings.TrimSpace(c.Paths.SiteRoot) == "" {
		return errors.New("paths.site_root is required")
	}
	if strings.TrimSpace(c.Paths.SiteSource) == "" {
		return errors.New("paths.site_source is required")
	}
	if strings.TrimSpace(c.Paths.SiteStage) == "" {
		return errors.New("paths.site_stage is required")
	}
	if filepath.Clean(c.Paths.SiteSource) == filepath.Clean(c.Paths.SiteStage) {
		return errors.New("paths.site_source and paths.site_stage must differ")
	}

	// P2P
	if c.P2P.ListenPort < 0 || c.P2P.ListenPort > 65535 {
		return errors.New("p2p.listen_port must be 0..65535")
	}
	if strings.TrimSpace(c.P2P.MdnsTag) == "" {
		return errors.New("p2p.mdns_tag is required")
	}

	// Presence (general)
	if strings.TrimSpace(c.Presence.Topic) == "" {
		return errors.New("presence.topic is required")
	}
	if c.Presence.TTLSec <= 0 {
		return errors.New("presence.ttl_seconds must be > 0")
	}
	if c.Presence.HeartbeatSec <= 0 {
		return errors.New("presence.heartbeat_seconds must be > 0")
	}
	if c.Presence.HeartbeatSec >= c.Presence.TTLSec {
		return errors.New("presence.heartbeat_seconds must be < presence.ttl_seconds")
	}

	// Presence (rendezvous-only semantics)
	if c.Presence.RendezvousOnly && !c.Presence.RendezvousHost {
		return errors.New("presence.rendezvous_only requires presence.rendezvous_host=true")
	}

	// Rendezvous (local server)
	if c.Presence.RendezvousHost {
		if c.Presence.RendezvousPort <= 0 || c.Presence.RendezvousPort > 65535 {
			return errors.New("presence.rendezvous_port must be 1..65535 when rendezvous_host is enabled")
		}
	}

	// Rendezvous (WAN mesh join)
	rw := strings.TrimSpace(c.Presence.RendezvousWAN)
	if rw != "" {
		if err := validateWANRendezvous(rw); err != nil {
			return fmt.Errorf("presence.rendezvous_wan: %w", err)
		}
	}

	return nil
}

func validateWANRendezvous(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("scheme must be http or https")
	}
	if u.Host == "" {
		return errors.New("missing host")
	}

	host := u.Hostname()
	if host == "" {
		return errors.New("missing hostname")
	}

	// Must be a remote/WAN destination, never bind/loopback.
	// Allow loopback for local testing/development
	if host == "0.0.0.0" {
		return errors.New("host must not be 0.0.0.0")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() {
			return errors.New("host must not be unspecified")
		}
	}

	// If a port is specified, validate it’s numeric 1..65535.
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return errors.New("invalid port")
		}
	}

	return nil
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	// Start from defaults so missing JSON fields remain initialized.
	cfg := Default()
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	return util.WriteJSONFile(path, cfg)
}

// Ensure loads config if it exists; otherwise creates a default config file.
// Returns (cfg, createdNew, err).
func Ensure(path string) (Config, bool, error) {
	if _, err := os.Stat(path); err == nil {
		cfg, err := Load(path)
		return cfg, false, err
	} else if !os.IsNotExist(err) {
		return Config{}, false, err
	}

	cfg := Default()
	if err := Save(path, cfg); err != nil {
		return Config{}, false, fmt.Errorf("create default config: %w", err)
	}
	return cfg, true, nil
}
