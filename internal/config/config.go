package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/petervdpas/goop2/internal/util"
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
	Lua      Lua      `json:"lua"`
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

	// If true, run a local rendezvous service on RendezvousBind:RendezvousPort.
	RendezvousHost bool `json:"rendezvous_host"`

	// Local rendezvous service port (used only when RendezvousHost=true).
	RendezvousPort int `json:"rendezvous_port"`

	// Bind address for the rendezvous server. Default "127.0.0.1" (localhost only).
	// Set to "0.0.0.0" to accept connections from other machines on the network.
	RendezvousBind string `json:"rendezvous_bind"`

	// Optional WAN rendezvous address to join WAN presence mesh (LAN + WAN).
	// Example: https://rv.example.org  or  http://1.2.3.4:8787
	RendezvousWAN string `json:"rendezvous_wan"`

	// If true: run ONLY rendezvous server; do NOT start libp2p peer node.
	// This implies RendezvousHost=true and requires a valid RendezvousPort.
	RendezvousOnly bool `json:"rendezvous_only"`

	// Password for the /admin monitoring panel (HTTP Basic Auth, user: "admin").
	// Empty means admin panel is disabled (returns 403).
	AdminPassword string `json:"admin_password"`

	// Optional path to a SQLite database for persisting peer state across
	// rendezvous server restarts and sharing state between multiple instances.
	// Relative to the peer directory. Empty means in-memory only (default).
	PeerDBPath string `json:"peer_db_path"`

	// Public URL for the rendezvous server (e.g., "https://goop2.com").
	// When set, this URL is shown to users instead of auto-discovered LAN IPs.
	// Required for servers behind NAT or reverse proxies.
	ExternalURL string `json:"external_url"`

	// Circuit relay v2 port. When > 0, a relay libp2p host is started on this
	// TCP port alongside the rendezvous HTTP server. Requires RendezvousHost=true.
	RelayPort int `json:"relay_port"`

	// Path to the relay identity key file. Default "data/relay.key".
	RelayKeyFile string `json:"relay_key_file"`

	// Relay timing (seconds). Pushed to clients via /relay.
	// 0 = use default. Only validated when RelayPort > 0.
	RelayCleanupDelaySec    int `json:"relay_cleanup_delay_sec"`
	RelayPollDeadlineSec    int `json:"relay_poll_deadline_sec"`
	RelayConnectTimeoutSec  int `json:"relay_connect_timeout_sec"`
	RelayRefreshIntervalSec int `json:"relay_refresh_interval_sec"`
	RelayRecoveryGraceSec   int `json:"relay_recovery_grace_sec"`

	// When true, external microservices (credits, registration, email, templates)
	// are wired up using the URLs below. When false, services are disabled even
	// if URLs are set — useful for running a LAN-only server without microservices.
	UseServices bool `json:"use_services"`

	// External service URLs. When set, goop2 proxies requests to these
	// standalone services. Empty = disabled (use built-in/no-op defaults).
	CreditsURL      string `json:"credits_url"`      // e.g., "http://localhost:8800"
	RegistrationURL string `json:"registration_url"` // e.g., "http://localhost:8801"
	EmailURL        string `json:"email_url"`        // e.g., "http://localhost:8802"
	TemplatesURL    string `json:"templates_url"`    // e.g., "http://localhost:8803"

	// Local template directory for the store (used when templates_url is empty).
	// Each subdirectory needs a manifest.json. Relative to peer dir.
	TemplatesDir string `json:"templates_dir"`

	// Admin tokens for accessing admin-only endpoints on external services.
	// Used when fetching data panels in the admin dashboard.
	CreditsAdminToken      string `json:"credits_admin_token"`
	RegistrationAdminToken string `json:"registration_admin_token"`
	TemplatesAdminToken    string `json:"templates_admin_token"`

}

type Profile struct {
	Label string `json:"label"`
	Email string `json:"email"`
}

type Viewer struct {
	HTTPAddr       string `json:"http_addr"`
	Debug          bool   `json:"debug"`
	Theme          string `json:"theme"`
	PreferredCam   string `json:"preferred_cam"`
	PreferredMic   string `json:"preferred_mic"`
	VideoDisabled  bool   `json:"video_disabled"`  // Disable video/audio calls (e.g., Linux WebKitGTK limitation)
	HideUnverified bool   `json:"hide_unverified"` // Hide unverified peers from the peer list
	ActiveTemplate string `json:"active_template"` // dir name of currently applied template
}

type Lua struct {
	Enabled          bool   `json:"enabled"`
	ScriptDir        string `json:"script_dir"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	MaxMemoryMB      int    `json:"max_memory_mb"`
	RateLimitPerPeer int    `json:"rate_limit_per_peer"`
	RateLimitGlobal  int    `json:"rate_limit_global"`
	HTTPEnabled      bool   `json:"http_enabled"`
	KVEnabled        bool   `json:"kv_enabled"`
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
			Topic:               "goop.presence.v1",
			TTLSec:              20,
			HeartbeatSec:        5,
			RendezvousHost:      false,
			RendezvousPort:      8787,
			RendezvousBind:      "127.0.0.1",
			RendezvousWAN:       "",
			RendezvousOnly:      false,
			RelayPort:               0,
			RelayKeyFile:            "data/relay.key",
			RelayCleanupDelaySec:    3,
			RelayPollDeadlineSec:    25,
			RelayConnectTimeoutSec:  15,
			RelayRefreshIntervalSec: 300,
			RelayRecoveryGraceSec:   5,
		},
		Profile: Profile{
			Label: "hello",
		},
		Viewer: Viewer{
			HTTPAddr: "",
			Debug:    false,
			Theme:    "dark",
		},
		Lua: Lua{
			Enabled:          false,
			ScriptDir:        "site/lua",
			TimeoutSeconds:   5,
			MaxMemoryMB:      10,
			RateLimitPerPeer: 30,
			RateLimitGlobal:  120,
			HTTPEnabled:      true,
			KVEnabled:        true,
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
		if b := c.Presence.RendezvousBind; b != "" {
			if net.ParseIP(b) == nil {
				return errors.New("presence.rendezvous_bind must be a valid IP address")
			}
		}
	}

	// Relay
	if c.Presence.RelayPort > 0 {
		if !c.Presence.RendezvousHost {
			return errors.New("presence.relay_port requires presence.rendezvous_host=true")
		}
		if c.Presence.RelayPort > 65535 {
			return errors.New("presence.relay_port must be 1..65535")
		}
		if c.Presence.RelayCleanupDelaySec < 0 {
			return errors.New("presence.relay_cleanup_delay_sec must be >= 0")
		}
		if c.Presence.RelayPollDeadlineSec < 0 {
			return errors.New("presence.relay_poll_deadline_sec must be >= 0")
		}
		if c.Presence.RelayConnectTimeoutSec < 0 {
			return errors.New("presence.relay_connect_timeout_sec must be >= 0")
		}
		if c.Presence.RelayRefreshIntervalSec < 0 {
			return errors.New("presence.relay_refresh_interval_sec must be >= 0")
		}
		if c.Presence.RelayRecoveryGraceSec < 0 {
			return errors.New("presence.relay_recovery_grace_sec must be >= 0")
		}
	}

	// Rendezvous (WAN mesh join)
	rw := strings.TrimSpace(c.Presence.RendezvousWAN)
	if rw != "" {
		if err := validateWANRendezvous(rw); err != nil {
			return fmt.Errorf("presence.rendezvous_wan: %w", err)
		}
	}

	// Lua
	if c.Lua.Enabled {
		if strings.TrimSpace(c.Lua.ScriptDir) == "" {
			return errors.New("lua.script_dir is required when lua is enabled")
		}
		if c.Lua.TimeoutSeconds < 1 || c.Lua.TimeoutSeconds > 60 {
			return errors.New("lua.timeout_seconds must be 1..60")
		}
		if c.Lua.RateLimitPerPeer <= 0 {
			return errors.New("lua.rate_limit_per_peer must be > 0")
		}
		if c.Lua.RateLimitGlobal <= 0 {
			return errors.New("lua.rate_limit_global must be > 0")
		}
		if c.Lua.MaxMemoryMB < 1 || c.Lua.MaxMemoryMB > 1024 {
			return errors.New("lua.max_memory_mb must be 1..1024")
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

	// Strip UTF-8 BOM if present (common when editing JSON on Windows).
	b = stripBOM(b)

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

// LoadPartial reads a config file without validation. Useful for reading
// individual fields (like rendezvous_only) when full validation may fail.
func LoadPartial(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	b = stripBOM(b)

	cfg := Default()
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// stripBOM removes a UTF-8 byte order mark if present.
func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
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
