# Configuration

All configuration lives in a single `goop.json` file in your peer directory. There are no environment variables or CLI flags for settings.

## Full reference

```json
{
  "identity": {
    "key_file": "data/identity.key"
  },
  "paths": {
    "site_root": "site",
    "site_source": "site/src",
    "site_stage": "site/stage"
  },
  "p2p": {
    "listen_port": 0,
    "mdns_tag": "goop-mdns",
    "bridge_mode": false,
    "nacl_public_key": "",
    "nacl_private_key": ""
  },
  "presence": {
    "topic": "goop.presence.v1",
    "ttl_seconds": 20,
    "heartbeat_seconds": 5,
    "rendezvous_host": false,
    "rendezvous_port": 8787,
    "rendezvous_bind": "127.0.0.1",
    "rendezvous_wan": "",
    "rendezvous_only": false,
    "admin_password": "",
    "external_url": "",
    "peer_db_path": "",
    "relay_port": 0,
    "relay_ws_port": 0,
    "relay_key_file": "data/relay.key",
    "relay_cleanup_delay_sec": 3,
    "relay_poll_deadline_sec": 10,
    "relay_connect_timeout_sec": 5,
    "relay_refresh_interval_sec": 90,
    "relay_recovery_grace_sec": 5,
    "use_services": false,
    "credits_url": "",
    "registration_url": "",
    "email_url": "",
    "templates_url": "",
    "bridge_url": "",
    "encryption_url": "",
    "templates_dir": "templates",
    "credits_admin_token": "",
    "registration_admin_token": "",
    "templates_admin_token": "",
    "bridge_admin_token": "",
    "encryption_admin_token": ""
  },
  "profile": {
    "label": "hello",
    "email": "",
    "verification_token": "",
    "bridge_token": ""
  },
  "viewer": {
    "http_addr": "127.0.0.1:8080",
    "debug": false,
    "theme": "dark",
    "preferred_cam": "",
    "preferred_mic": "",
    "video_disabled": false,
    "hide_unverified": false,
    "active_template": "",
    "open_sites_external": false,
    "splash": "goop2-splash2.png",
    "peer_offline_grace_min": 15,
    "cluster_binary_path": "",
    "cluster_binary_mode": ""
  },
  "lua": {
    "enabled": false,
    "script_dir": "site/lua",
    "timeout_seconds": 5,
    "max_memory_mb": 10,
    "rate_limit_per_peer": 30,
    "rate_limit_global": 120,
    "http_enabled": true,
    "kv_enabled": true
  }
}
```

## Section details

### identity

| Field | Default | Description |
|-------|---------|-------------|
| `key_file` | `data/identity.key` | Path to the peer's persistent cryptographic identity. Created automatically on first run. |

### paths

| Field | Default | Description |
|-------|---------|-------------|
| `site_root` | `site` | Directory served to visitors. |
| `site_source` | `site/src` | Source directory for the site editor (optional). |
| `site_stage` | `site/stage` | Staging directory for previewing changes (optional). Must differ from `site_source`. |

### p2p

| Field | Default | Description |
|-------|---------|-------------|
| `listen_port` | `0` | libp2p listen port. `0` picks an available port automatically. Set a fixed port if you need to configure firewall rules. |
| `mdns_tag` | `goop-mdns` | mDNS service tag. Peers with the same tag discover each other on the local network. |
| `bridge_mode` | `false` | When true, connect through a bridge service over WebSocket instead of libp2p. Useful for thin clients that cannot run a full P2P node. |
| `nacl_public_key` | `""` | NaCl public key for peer-to-peer encryption. Generated automatically on first use. |
| `nacl_private_key` | `""` | NaCl private key for peer-to-peer encryption. Generated automatically on first use. |

### presence

| Field | Default | Description |
|-------|---------|-------------|
| `topic` | `goop.presence.v1` | PubSub topic for presence messages. |
| `ttl_seconds` | `20` | Seconds before an unresponsive peer is considered offline. |
| `heartbeat_seconds` | `5` | Seconds between presence announcements. Must be less than `ttl_seconds`. |
| `rendezvous_host` | `false` | Enable the built-in rendezvous server. |
| `rendezvous_port` | `8787` | Port for the rendezvous server (when hosting). |
| `rendezvous_bind` | `127.0.0.1` | Bind address for the rendezvous server. Set to `0.0.0.0` to accept connections from other machines. |
| `rendezvous_wan` | `""` | URL of a remote rendezvous server to publish presence to. |
| `rendezvous_only` | `false` | Run only the rendezvous server with no P2P node. |
| `admin_password` | `""` | Password for the rendezvous admin panel. Leave empty to disable admin. |
| `peer_db_path` | `""` | SQLite path for persisting peer state across restarts. Required for registration and multi-instance setups. |
| `external_url` | `""` | Public URL for the server (e.g. `https://goop2.com`). Required behind a reverse proxy so peers see the correct address. |
| `relay_port` | `0` | Circuit relay v2 port. When > 0, a relay host runs alongside the rendezvous server for NAT traversal. |
| `relay_ws_port` | `0` | WebSocket relay port. When > 0, a WebSocket relay endpoint runs alongside the circuit relay. |
| `relay_key_file` | `data/relay.key` | Path to the relay identity key file. |
| `relay_cleanup_delay_sec` | `3` | Seconds before cleaning up stale relay connections. |
| `relay_poll_deadline_sec` | `10` | Seconds before a relay poll request times out. |
| `relay_connect_timeout_sec` | `5` | Seconds before a relay connect attempt times out. |
| `relay_refresh_interval_sec` | `90` | Seconds between relay reservation refreshes. |
| `relay_recovery_grace_sec` | `5` | Seconds to wait before retrying after a relay failure. |
| `use_services` | `false` | Master switch for external microservices. When false, services are disabled even if URLs are set. |
| `credits_url` | `""` | URL of the credits service (e.g. `http://localhost:8800`). Enables template pricing and credit purchases. |
| `registration_url` | `""` | URL of the registration service (e.g. `http://localhost:8801`). Handles email verification and peer registration. |
| `email_url` | `""` | URL of the email service (e.g. `http://localhost:8802`). Centralizes SMTP sending and email templates. |
| `templates_url` | `""` | URL of the templates service (e.g. `http://localhost:8803`). Provides store templates, bundling, and pricing. |
| `bridge_url` | `""` | URL of the bridge service (e.g. `http://localhost:8804`). Enables thin-client peer connections over WebSocket. |
| `encryption_url` | `""` | URL of the encryption service (e.g. `http://localhost:8805`). Manages peer key exchange and broadcast key distribution. |
| `templates_dir` | `templates` | Local template directory for the store (fallback when `templates_url` is empty). Each subdirectory needs a `manifest.json`. |
| `credits_admin_token` | `""` | Bearer token for admin endpoints on the credits service. |
| `registration_admin_token` | `""` | Bearer token for admin endpoints on the registration service. |
| `templates_admin_token` | `""` | Bearer token for admin endpoints on the templates service. |
| `bridge_admin_token` | `""` | Bearer token for admin endpoints on the bridge service. |
| `encryption_admin_token` | `""` | Bearer token for admin endpoints on the encryption service. |

### profile

| Field | Default | Description |
|-------|---------|-------------|
| `label` | `hello` | Display name shown to other peers. |
| `email` | `""` | Email address for identity and avatar. |
| `verification_token` | `""` | Email verification token returned by the registration service. Set automatically after verification. |
| `bridge_token` | `""` | Token for bridge-mode authentication. Set automatically when requesting a bridge connection. |

### viewer

| Field | Default | Description |
|-------|---------|-------------|
| `http_addr` | `""` | Bind address for the local viewer. Use `127.0.0.1:8080` to restrict access to your machine. Empty means auto-assigned. |
| `debug` | `false` | Enable debug mode in the viewer. |
| `theme` | `dark` | Default theme: `dark` or `light`. |
| `preferred_cam` | `""` | Preferred camera device label for video calls. |
| `preferred_mic` | `""` | Preferred microphone device label for video calls. |
| `video_disabled` | `false` | Disable video and audio calls entirely. |
| `hide_unverified` | `false` | Hide unverified peers from the peer list. |
| `active_template` | `""` | Directory name of the currently applied template. Set automatically when applying a template. |
| `open_sites_external` | `false` | Open peer sites in the system browser instead of embedded tabs. |
| `splash` | `goop2-splash2.png` | Splash image filename displayed on the peers page. |
| `peer_offline_grace_min` | `15` | Minutes before an offline non-favorite peer is pruned from the peer list (1--60). |
| `cluster_binary_path` | `""` | Path to the executor binary for cluster compute jobs. |
| `cluster_binary_mode` | `""` | Executor binary mode: `oneshot` (default) or `daemon`. |

### lua

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Enable Lua scripting. |
| `script_dir` | `site/lua` | Directory containing Lua scripts. |
| `timeout_seconds` | `5` | Maximum execution time per script invocation (1--60). |
| `max_memory_mb` | `10` | Maximum memory per Lua VM (1--1024). |
| `rate_limit_per_peer` | `30` | Maximum Lua calls per minute per remote peer. |
| `rate_limit_global` | `120` | Maximum total Lua calls per minute. |
| `http_enabled` | `true` | Allow Lua scripts to make HTTP requests. |
| `kv_enabled` | `true` | Allow Lua scripts to use the key-value store. |

## Validation rules

- `site_source` and `site_stage` must be different paths.
- `heartbeat_seconds` must be less than `ttl_seconds`.
- `listen_port` must be `0` or between `1` and `65535`.
- `relay_port`, when set, must be between `1` and `65535`.
- `rendezvous_only` requires `rendezvous_host` to be true.
- `relay_port` requires `rendezvous_host` to be true.
- Relay timing values must be >= 0 (only validated when `relay_port` > 0).
- `lua.timeout_seconds` must be 1--60 when Lua is enabled.
- `lua.max_memory_mb` must be 1--1024 when Lua is enabled.

## External services

Goop2 can connect to six standalone microservices that add functionality to the rendezvous server. These services are separate binaries from the [goop2-services](https://github.com/petervdpas/goop2-services) repository and must be installed and run independently.

All services are optional. Set `use_services` to `true` to activate them; when `false`, service URLs are ignored even if set.

| Service | Default port | What it does |
|---------|-------------|--------------|
| **Registration** | `:8801` | Handles email verification and peer registration. Owns the `registration_required` toggle and the registrations database. Without it, Goop2 uses a built-in registration fallback. |
| **Credits** | `:8800` | Manages credit balances, template pricing, and purchases. Credits are tied to registered email accounts. Without it, all templates are free. |
| **Email** | `:8802` | Sends transactional emails (verification, welcome, purchase receipts) via SMTP. Provides HTML email templates. The registration service uses it for sending verification emails. |
| **Templates** | `:8803` | Stores and serves store templates, handles bundling and pricing. Templates are loaded from disk, not embedded. The credits service proxies price lookups to it. |
| **Bridge** | `:8804` | Enables thin-client peers to connect over WebSocket instead of libp2p. Useful for environments where running a full P2P node is not possible. |
| **Encryption** | `:8805` | Manages NaCl key exchange between peers and distributes broadcast encryption keys. Handles key rotation. |

Each service has its own `config.json` where you configure service-specific settings like SMTP credentials (email service), `registration_required` toggle (registration service), or `dummy_mode` for safe testing. Goop2 communicates with these services purely over HTTP.

To connect services, set `use_services` to `true` and provide the service URLs and admin tokens:

```json
"use_services": true,
"registration_url": "http://localhost:8801",
"registration_admin_token": "your-shared-token"
```

The admin token must match the `admin_token` in the service's own config. It is required for the admin dashboard to read registration and account data.

The service dependency chain is: **registration** depends on **credits**, which depends on **templates**. The registration service calls the email service for sending verification emails. The credits service proxies price lookups to the templates service. The bridge service and encryption service are independent.

## Example configurations

### Simple peer

```json
{
  "profile": { "label": "My Site" },
  "viewer": { "http_addr": "127.0.0.1:8080" }
}
```

### Peer with WAN rendezvous

```json
{
  "profile": { "label": "My Site" },
  "presence": {
    "rendezvous_wan": "https://goop2.com"
  },
  "p2p": { "listen_port": 4001 }
}
```

### Dedicated rendezvous server

```json
{
  "presence": {
    "rendezvous_only": true,
    "rendezvous_host": true,
    "rendezvous_port": 8787,
    "rendezvous_bind": "0.0.0.0",
    "admin_password": "secure-password",
    "external_url": "https://goop2.com"
  }
}
```

### Rendezvous with relay and all services

```json
{
  "presence": {
    "rendezvous_only": true,
    "rendezvous_host": true,
    "rendezvous_port": 8787,
    "rendezvous_bind": "0.0.0.0",
    "admin_password": "secure-password",
    "external_url": "https://goop2.com",
    "relay_port": 4001,
    "peer_db_path": "data/peers.db",
    "use_services": true,
    "registration_url": "http://localhost:8801",
    "registration_admin_token": "shared-secret",
    "credits_url": "http://localhost:8800",
    "credits_admin_token": "shared-secret",
    "email_url": "http://localhost:8802",
    "templates_url": "http://localhost:8803",
    "templates_admin_token": "shared-secret",
    "bridge_url": "http://localhost:8804",
    "bridge_admin_token": "shared-secret",
    "encryption_url": "http://localhost:8805",
    "encryption_admin_token": "shared-secret"
  }
}
```

### Thin-client peer (bridge mode)

```json
{
  "profile": {
    "label": "My Thin Client",
    "email": "me@example.com"
  },
  "p2p": {
    "bridge_mode": true
  },
  "presence": {
    "bridge_url": "http://localhost:8804"
  }
}
```

### Peer with cluster compute

```json
{
  "profile": { "label": "Worker Node" },
  "presence": {
    "rendezvous_wan": "https://goop2.com"
  },
  "viewer": {
    "cluster_binary_path": "/usr/local/bin/my-executor",
    "cluster_binary_mode": "daemon"
  }
}
```

See the [External services](#external-services) section above for details on these microservices.
