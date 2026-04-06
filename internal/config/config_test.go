package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault_ValidatesCleanly(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Default() config should validate: %v", err)
	}
}

func TestDefault_KeyValues(t *testing.T) {
	cfg := Default()

	if cfg.Identity.KeyFile != "data/identity.key" {
		t.Errorf("KeyFile = %q", cfg.Identity.KeyFile)
	}
	if cfg.Paths.SiteRoot != "site" {
		t.Errorf("SiteRoot = %q", cfg.Paths.SiteRoot)
	}
	if cfg.P2P.MdnsTag != "goop-mdns" {
		t.Errorf("MdnsTag = %q", cfg.P2P.MdnsTag)
	}
	if cfg.Presence.TTLSec != 20 {
		t.Errorf("TTLSec = %d", cfg.Presence.TTLSec)
	}
	if cfg.Presence.HeartbeatSec != 5 {
		t.Errorf("HeartbeatSec = %d", cfg.Presence.HeartbeatSec)
	}
	if cfg.Presence.RendezvousPort != 8787 {
		t.Errorf("RendezvousPort = %d", cfg.Presence.RendezvousPort)
	}
	if cfg.Presence.RendezvousBind != "127.0.0.1" {
		t.Errorf("RendezvousBind = %q", cfg.Presence.RendezvousBind)
	}
	if cfg.Viewer.Theme != "dark" {
		t.Errorf("Theme = %q", cfg.Viewer.Theme)
	}
	if cfg.Viewer.PeerOfflineGraceMin != 15 {
		t.Errorf("PeerOfflineGraceMin = %d", cfg.Viewer.PeerOfflineGraceMin)
	}
	if cfg.Lua.TimeoutSeconds != 5 {
		t.Errorf("Lua.TimeoutSeconds = %d", cfg.Lua.TimeoutSeconds)
	}
	if cfg.Lua.MaxMemoryMB != 10 {
		t.Errorf("Lua.MaxMemoryMB = %d", cfg.Lua.MaxMemoryMB)
	}
}

func validConfig() Config {
	return Default()
}

func TestValidate_Identity(t *testing.T) {
	cfg := validConfig()
	cfg.Identity.KeyFile = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty key_file")
	}
}

func TestValidate_Paths(t *testing.T) {
	t.Run("EmptySiteRoot", func(t *testing.T) {
		cfg := validConfig()
		cfg.Paths.SiteRoot = ""
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("EmptySiteSource", func(t *testing.T) {
		cfg := validConfig()
		cfg.Paths.SiteSource = ""
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("EmptySiteStage", func(t *testing.T) {
		cfg := validConfig()
		cfg.Paths.SiteStage = ""
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("SourceEqualsStage", func(t *testing.T) {
		cfg := validConfig()
		cfg.Paths.SiteSource = "same"
		cfg.Paths.SiteStage = "same"
		if err := cfg.Validate(); err == nil {
			t.Error("expected error when source == stage")
		}
	})
}

func TestValidate_P2P(t *testing.T) {
	t.Run("PortTooLow", func(t *testing.T) {
		cfg := validConfig()
		cfg.P2P.ListenPort = -1
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("PortTooHigh", func(t *testing.T) {
		cfg := validConfig()
		cfg.P2P.ListenPort = 65536
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("EmptyMdnsTag", func(t *testing.T) {
		cfg := validConfig()
		cfg.P2P.MdnsTag = ""
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
}

func TestValidate_Presence(t *testing.T) {
	t.Run("EmptyTopic", func(t *testing.T) {
		cfg := validConfig()
		cfg.Presence.Topic = ""
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("TTLZero", func(t *testing.T) {
		cfg := validConfig()
		cfg.Presence.TTLSec = 0
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("HeartbeatZero", func(t *testing.T) {
		cfg := validConfig()
		cfg.Presence.HeartbeatSec = 0
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("HeartbeatEqualsOrExceedsTTL", func(t *testing.T) {
		cfg := validConfig()
		cfg.Presence.HeartbeatSec = cfg.Presence.TTLSec
		if err := cfg.Validate(); err == nil {
			t.Error("expected error when heartbeat >= TTL")
		}
	})
	t.Run("RendezvousOnlyWithoutHost", func(t *testing.T) {
		cfg := validConfig()
		cfg.Presence.RendezvousOnly = true
		cfg.Presence.RendezvousHost = false
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
}

func TestValidate_Rendezvous(t *testing.T) {
	t.Run("HostEnabled_InvalidPort", func(t *testing.T) {
		cfg := validConfig()
		cfg.Presence.RendezvousHost = true
		cfg.Presence.RendezvousPort = 0
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for port 0")
		}
	})
	t.Run("HostEnabled_ValidPort", func(t *testing.T) {
		cfg := validConfig()
		cfg.Presence.RendezvousHost = true
		cfg.Presence.RendezvousPort = 8787
		if err := cfg.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("InvalidBind", func(t *testing.T) {
		cfg := validConfig()
		cfg.Presence.RendezvousHost = true
		cfg.Presence.RendezvousBind = "not-an-ip"
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for invalid bind address")
		}
	})
}

func TestValidate_Relay(t *testing.T) {
	relayConfig := func() Config {
		cfg := validConfig()
		cfg.Presence.RendezvousHost = true
		cfg.Presence.RelayPort = 4001
		return cfg
	}

	t.Run("RelayWithoutHost", func(t *testing.T) {
		cfg := validConfig()
		cfg.Presence.RelayPort = 4001
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("RelayPortTooHigh", func(t *testing.T) {
		cfg := relayConfig()
		cfg.Presence.RelayPort = 70000
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("RelayWSPortTooHigh", func(t *testing.T) {
		cfg := relayConfig()
		cfg.Presence.RelayWSPort = 70000
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("NegativeTimings", func(t *testing.T) {
		fields := []struct {
			name string
			set  func(*Config)
		}{
			{"CleanupDelay", func(c *Config) { c.Presence.RelayCleanupDelaySec = -1 }},
			{"PollDeadline", func(c *Config) { c.Presence.RelayPollDeadlineSec = -1 }},
			{"ConnectTimeout", func(c *Config) { c.Presence.RelayConnectTimeoutSec = -1 }},
			{"RefreshInterval", func(c *Config) { c.Presence.RelayRefreshIntervalSec = -1 }},
			{"RecoveryGrace", func(c *Config) { c.Presence.RelayRecoveryGraceSec = -1 }},
		}
		for _, f := range fields {
			t.Run(f.name, func(t *testing.T) {
				cfg := relayConfig()
				f.set(&cfg)
				if err := cfg.Validate(); err == nil {
					t.Errorf("expected error for negative %s", f.name)
				}
			})
		}
	})
}

func TestValidate_Lua(t *testing.T) {
	luaConfig := func() Config {
		cfg := validConfig()
		cfg.Lua.Enabled = true
		return cfg
	}

	t.Run("EmptyScriptDir", func(t *testing.T) {
		cfg := luaConfig()
		cfg.Lua.ScriptDir = ""
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("TimeoutTooLow", func(t *testing.T) {
		cfg := luaConfig()
		cfg.Lua.TimeoutSeconds = 0
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("TimeoutTooHigh", func(t *testing.T) {
		cfg := luaConfig()
		cfg.Lua.TimeoutSeconds = 61
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("RateLimitPerPeerZero", func(t *testing.T) {
		cfg := luaConfig()
		cfg.Lua.RateLimitPerPeer = 0
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("RateLimitGlobalZero", func(t *testing.T) {
		cfg := luaConfig()
		cfg.Lua.RateLimitGlobal = 0
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("MaxMemoryTooLow", func(t *testing.T) {
		cfg := luaConfig()
		cfg.Lua.MaxMemoryMB = 0
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("MaxMemoryTooHigh", func(t *testing.T) {
		cfg := luaConfig()
		cfg.Lua.MaxMemoryMB = 1025
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("DisabledSkipsValidation", func(t *testing.T) {
		cfg := validConfig()
		cfg.Lua.Enabled = false
		cfg.Lua.ScriptDir = ""
		cfg.Lua.TimeoutSeconds = 0
		if err := cfg.Validate(); err != nil {
			t.Errorf("disabled lua should skip validation: %v", err)
		}
	})
}

func TestValidateWANRendezvous(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"ValidHTTPS", "https://goop2.com", false},
		{"ValidHTTP", "http://1.2.3.4:8787", false},
		{"ValidHTTPSWithPort", "https://rv.example.org:443", false},
		{"Localhost", "http://127.0.0.1:8787", false},
		{"InvalidScheme", "ftp://goop2.com", true},
		{"NoScheme", "goop2.com", true},
		{"EmptyHost", "http://", true},
		{"BindAddress", "http://0.0.0.0:8787", true},
		{"InvalidPort", "http://goop2.com:99999", true},
		{"NonNumericPort", "http://goop2.com:abc", true},
		{"InvalidURL", "://bad", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWANRendezvous(tc.url)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateWANRendezvous(%q) error=%v, wantErr=%v", tc.url, err, tc.wantErr)
			}
		})
	}
}

func TestValidate_WANRendezvousIntegration(t *testing.T) {
	cfg := validConfig()
	cfg.Presence.RendezvousWAN = "ftp://bad"
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for invalid WAN URL")
	}
}

func TestStripBOM(t *testing.T) {
	t.Run("WithBOM", func(t *testing.T) {
		input := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"identity":{}}`)...)
		got := stripBOM(input)
		if string(got) != `{"identity":{}}` {
			t.Errorf("BOM not stripped: %q", got)
		}
	})
	t.Run("WithoutBOM", func(t *testing.T) {
		input := []byte(`{"identity":{}}`)
		got := stripBOM(input)
		if string(got) != `{"identity":{}}` {
			t.Errorf("unexpected change: %q", got)
		}
	})
	t.Run("ShortInput", func(t *testing.T) {
		got := stripBOM([]byte{0xEF, 0xBB})
		if len(got) != 2 {
			t.Errorf("short input should pass through")
		}
	})
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{
		"identity": {"key_file": "test.key"},
		"profile": {"label": "test-peer"}
	}`), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Identity.KeyFile != "test.key" {
		t.Errorf("KeyFile = %q", cfg.Identity.KeyFile)
	}
	if cfg.Profile.Label != "test-peer" {
		t.Errorf("Label = %q", cfg.Profile.Label)
	}
	if cfg.Paths.SiteRoot != "site" {
		t.Errorf("defaults not applied: SiteRoot = %q", cfg.Paths.SiteRoot)
	}
}

func TestLoad_WithBOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"profile":{"label":"bom-test"}}`)...)
	os.WriteFile(path, content, 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load with BOM: %v", err)
	}
	if cfg.Profile.Label != "bom-test" {
		t.Errorf("Label = %q", cfg.Profile.Label)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{not json}`), 0644)

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoad_ValidationFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"identity":{"key_file":""}}`), 0644)

	_, err := Load(path)
	if err == nil {
		t.Error("expected validation error")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadPartial_SkipsValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"identity":{"key_file":""}}`), 0644)

	cfg, err := LoadPartial(path)
	if err != nil {
		t.Fatalf("LoadPartial should not validate: %v", err)
	}
	if cfg.Identity.KeyFile != "" {
		t.Errorf("expected empty key_file from partial load")
	}
}

func TestSave_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := Default()
	cfg.Profile.Label = "saved-peer"
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if loaded.Profile.Label != "saved-peer" {
		t.Errorf("Label = %q", loaded.Profile.Label)
	}
}

func TestSave_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := Default()
	cfg.Identity.KeyFile = ""
	if err := Save(path, cfg); err == nil {
		t.Error("expected validation error on save")
	}
}

func TestEnsure_CreatesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, created, err := Ensure(path)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}
	if cfg.Profile.Label != "hello" {
		t.Errorf("Label = %q", cfg.Profile.Label)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should exist: %v", err)
	}
}

func TestEnsure_LoadsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"profile":{"label":"existing"}}`), 0644)

	cfg, created, err := Ensure(path)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if created {
		t.Error("expected created=false")
	}
	if cfg.Profile.Label != "existing" {
		t.Errorf("Label = %q", cfg.Profile.Label)
	}
}
