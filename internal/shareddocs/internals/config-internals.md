# Configuration Internals

## Config struct

`internal/config/config.go`

Top-level struct loaded from `goop.json`:

```
Config
├── Identity    — key_file
├── Paths       — site_root, site_source, site_stage
├── P2P         — listen_port, mdns_tag, bridge_mode, nacl keys
├── Presence    — rendezvous, relay, microservice URLs, admin
├── Profile     — label, email, verification_token, bridge_token
├── Viewer      — http_addr, theme, debug, video, template, cluster
└── Lua         — enabled, script_dir, timeouts, rate limits
```

## Sections

### Identity

| Field | Default | Purpose |
| -- | -- | -- |
| `key_file` | `data/identity.key` | Ed25519 identity key for libp2p peer ID |

### Paths

| Field | Default | Purpose |
| -- | -- | -- |
| `site_root` | `site` | Root directory for site content |
| `site_source` | `site/src` | Source files for the editor |
| `site_stage` | `site/stage` | Staging directory |

### P2P

| Field | Default | Purpose |
| -- | -- | -- |
| `listen_port` | `0` (random) | TCP port for libp2p host |
| `mdns_tag` | `goop-mdns` | mDNS discovery group tag |
| `bridge_mode` | `false` | Use WebSocket bridge instead of libp2p |
| `nacl_public_key` | (generated) | NaCl X25519 public key (base64) |
| `nacl_private_key` | (generated) | NaCl X25519 private key (base64) |

### Presence

| Field | Default | Purpose |
| -- | -- | -- |
| `topic` | `goop.presence.v1` | GossipSub presence topic |
| `ttl_seconds` | `20` | Presence TTL before peer is considered stale |
| `heartbeat_seconds` | `5` | Heartbeat interval |
| `rendezvous_host` | `false` | Run local rendezvous server |
| `rendezvous_port` | `8787` | Rendezvous server port |
| `rendezvous_bind` | `127.0.0.1` | Bind address (`0.0.0.0` for network access) |
| `rendezvous_wan` | (empty) | WAN rendezvous URL to join |
| `rendezvous_only` | `false` | Run ONLY rendezvous server, no P2P node |
| `admin_password` | (empty) | Admin panel password (empty = disabled) |
| `peer_db_path` | (empty) | SQLite path for persistent peer state |
| `external_url` | (empty) | Public URL for servers behind NAT/proxy |
| `relay_port` | `0` | Circuit relay v2 port (0 = disabled) |
| `relay_ws_port` | `0` | Relay WebSocket port |
| `relay_key_file` | `data/relay.key` | Relay identity key file |
| `relay_cleanup_delay_sec` | `3` | Relay timing |
| `relay_poll_deadline_sec` | `10` | Relay timing |
| `relay_connect_timeout_sec` | `5` | Relay timing |
| `relay_refresh_interval_sec` | `90` | Relay timing |
| `relay_recovery_grace_sec` | `5` | Relay timing |
| `use_services` | `false` | Enable microservice integration |
| `credits_url` | (empty) | Credits service URL |
| `registration_url` | (empty) | Registration service URL |
| `email_url` | (empty) | Email service URL |
| `templates_url` | (empty) | Templates service URL |
| `bridge_url` | (empty) | Bridge service URL |
| `encryption_url` | (empty) | Encryption service URL |
| `templates_dir` | (empty) | Local template directory (fallback) |
| `*_admin_token` | (empty) | Admin tokens for service dashboards |

### Profile

| Field | Default | Purpose |
| -- | -- | -- |
| `label` | `hello` | Peer display name |
| `email` | (empty) | Peer email address |
| `verification_token` | (empty) | Email verification token |
| `bridge_token` | (empty) | Bridge service token |

### Viewer

| Field | Default | Purpose |
| -- | -- | -- |
| `http_addr` | (empty, auto-assigned) | Viewer HTTP listen address |
| `debug` | `false` | Enable debug mode |
| `theme` | `dark` | UI theme (dark/light) |
| `preferred_cam` | (empty) | Preferred camera device |
| `preferred_mic` | (empty) | Preferred microphone device |
| `video_disabled` | `false` | Disable video/audio calls |
| `hide_unverified` | `false` | Hide unverified peers |
| `active_template` | (empty) | Currently applied template dir name |
| `open_sites_external` | `false` | Open peer sites in system browser |
| `splash` | `goop2-splash2.png` | Splash image filename |
| `peer_offline_grace_min` | `15` | Minutes before offline non-favorite is pruned (1–60) |
| `cluster_binary_path` | (empty) | Path to cluster worker binary |
| `cluster_binary_mode` | (empty) | Cluster binary execution mode |

### Lua

| Field | Default | Purpose |
| -- | -- | -- |
| `enabled` | `false` | Enable Lua engine |
| `script_dir` | (empty) | Override script directory |
| `timeout_seconds` | (config default) | Script execution timeout |
| `max_memory_mb` | (config default) | Script memory limit |
| `rate_limit_per_peer` | (config default) | Requests/min/peer/function |
| `rate_limit_global` | (config default) | Global requests/min |
| `http_enabled` | `false` | Enable Lua HTTP client |
| `kv_enabled` | `false` | Enable Lua key-value store |

## Loading

`config.Ensure(cfgPath)` loads the config from `goop.json`, applies defaults via `Default()`, validates via `Validate()`, and writes back if the file was newly created.
