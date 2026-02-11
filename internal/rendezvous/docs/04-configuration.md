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
    "mdns_tag": "goop-mdns"
  },
  "presence": {
    "topic": "goop.presence.v1",
    "ttl_seconds": 20,
    "heartbeat_seconds": 5,
    "rendezvous_host": false,
    "rendezvous_port": 8787,
    "rendezvous_wan": "",
    "rendezvous_only": false,
    "admin_password": "",
    "external_url": "",
    "templates_dir": "templates",
    "peer_db_path": "",
    "relay_port": 0,
    "relay_key_file": "data/relay.key",
    "registration_url": "",
    "registration_admin_token": "",
    "credits_url": "",
    "credits_admin_token": "",
    "email_url": "",
    "templates_url": ""
  },
  "profile": {
    "label": "hello",
    "email": ""
  },
  "viewer": {
    "http_addr": "127.0.0.1:8080",
    "debug": false,
    "theme": "dark"
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

### presence

| Field | Default | Description |
|-------|---------|-------------|
| `topic` | `goop.presence.v1` | PubSub topic for presence messages. |
| `ttl_seconds` | `20` | Seconds before an unresponsive peer is considered offline. |
| `heartbeat_seconds` | `5` | Seconds between presence announcements. Must be less than `ttl_seconds`. |
| `rendezvous_host` | `false` | Enable the built-in rendezvous server. |
| `rendezvous_port` | `8787` | Port for the rendezvous server (when hosting). |
| `rendezvous_wan` | `""` | URL of a remote rendezvous server to publish presence to. |
| `rendezvous_only` | `false` | Run only the rendezvous server with no P2P node. |
| `admin_password` | `""` | Password for the rendezvous admin panel. Leave empty to disable admin. |
| `external_url` | `""` | Public URL for the server (e.g. `https://goop2.com`). Required behind a reverse proxy so peers see the correct address. |
| `templates_dir` | `templates` | Directory containing template store templates (relative to peer dir). |
| `peer_db_path` | `""` | SQLite path for persisting peer state across restarts. Required for registration and multi-instance setups. |
| `relay_port` | `0` | Circuit relay v2 port. When > 0, a relay host runs alongside the rendezvous server for NAT traversal. |
| `relay_key_file` | `data/relay.key` | Path to the relay identity key file. |
| `registration_url` | `""` | URL of the registration service (e.g. `http://localhost:8801`). Leave empty to use built-in registration. |
| `registration_admin_token` | `""` | Bearer token for admin endpoints on the registration service. Must match `admin_token` in the service's config. |
| `credits_url` | `""` | URL of the credits service (e.g. `http://localhost:8800`). Enables template pricing and credit purchases. |
| `credits_admin_token` | `""` | Bearer token for admin endpoints on the credits service. Must match `admin_token` in the service's config. |
| `email_url` | `""` | URL of the email service (e.g. `http://localhost:8802`). Centralizes SMTP sending and email templates. |
| `templates_url` | `""` | URL of the templates service (e.g. `http://localhost:8803`). Provides store templates, bundling, and pricing. Without it, only built-in templates are available. |

### profile

| Field | Default | Description |
|-------|---------|-------------|
| `label` | `hello` | Display name shown to other peers. |
| `email` | `""` | Email address for identity and avatar. |

### viewer

| Field | Default | Description |
|-------|---------|-------------|
| `http_addr` | `127.0.0.1:8080` | Bind address for the local viewer. Use `127.0.0.1` to restrict access to your machine. |
| `debug` | `false` | Enable debug mode in the viewer. |
| `theme` | `dark` | Default theme: `dark` or `light`. |

### lua

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Enable Lua scripting. |
| `script_dir` | `site/lua` | Directory containing Lua scripts. |
| `timeout_seconds` | `5` | Maximum execution time per script invocation (1--60). |
| `max_memory_mb` | `10` | Maximum memory per Lua VM. |
| `rate_limit_per_peer` | `30` | Maximum Lua calls per minute per remote peer. |
| `rate_limit_global` | `120` | Maximum total Lua calls per minute. |
| `http_enabled` | `true` | Allow Lua scripts to make HTTP requests. |
| `kv_enabled` | `true` | Allow Lua scripts to use the key-value store. |

## Validation rules

- `site_source` and `site_stage` must be different paths.
- `heartbeat_seconds` must be less than `ttl_seconds`.
- `listen_port` must be `0` or between `1` and `65535`.
- `relay_port`, when set, must be between `1` and `65535`.

## External services

Goop2 can optionally connect to three standalone microservices that add functionality to the rendezvous server. These services are **not included** with Goop2 -- they are separate binaries from the [goop2-services](https://github.com/petervdpas/goop2-services) repository and must be installed and run independently.

All services are optional. When a service URL is left empty, that feature is either disabled or falls back to built-in behavior.

| Service | Default port | What it does |
|---------|-------------|--------------|
| **Registration** | `:8801` | Handles email verification and peer registration. Owns the `registration_required` toggle and the registrations database. Without it, Goop2 uses a built-in registration fallback. |
| **Credits** | `:8800` | Manages credit balances, template pricing, and purchases. Credits are tied to registered email accounts. Without it, all templates are free. |
| **Email** | `:8802` | Sends transactional emails (verification, welcome, purchase receipts) via SMTP. Provides HTML email templates. The registration service uses it for sending verification emails. |
| **Templates** | `:8803` | Stores and serves store templates, handles bundling and pricing. Templates are loaded from disk, not embedded. The credits service proxies price lookups to it. |

Each service has its own `config.json` where you configure service-specific settings like SMTP credentials (email service), `registration_required` toggle (registration service), or `dummy_mode` for safe testing. Goop2 communicates with these services purely over HTTP.

To connect a service, set its URL and (where applicable) a shared admin token in the `presence` section of your `goop.json`:

```json
"registration_url": "http://localhost:8801",
"registration_admin_token": "your-shared-token"
```

The admin token must match the `admin_token` in the service's own config. It is required for the admin dashboard to read registration and account data.

The service dependency chain is: **registration** depends on **credits**, which depends on **templates**. The registration service calls the email service for sending verification emails. The credits service proxies price lookups to the templates service.

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
    "admin_password": "secure-password",
    "external_url": "https://goop2.com"
  }
}
```

### Rendezvous with relay and services

```json
{
  "presence": {
    "rendezvous_only": true,
    "rendezvous_host": true,
    "rendezvous_port": 8787,
    "admin_password": "secure-password",
    "external_url": "https://goop2.com",
    "relay_port": 4001,
    "peer_db_path": "data/peers.db",
    "registration_url": "http://localhost:8801",
    "registration_admin_token": "shared-secret",
    "credits_url": "http://localhost:8800",
    "credits_admin_token": "shared-secret",
    "email_url": "http://localhost:8802",
    "templates_url": "http://localhost:8803"
  }
}
```

See the [External services](#external-services) section above for details on these microservices.
