# goop2-services Architecture

## Overview

Separate repo: `~/Projects/goop2-services/`. Monorepo with independent services, each a standalone Go binary with its own HTTP server and SQLite database. Zero imports between repos — goop2 gateway proxies HTTP calls.

All services share a common startup pattern: parse `-config` flag, load JSON config, init DB, setup logbuf (200-line ring buffer), register routes, `http.ListenAndServe`. Every service exposes `GET /healthz`, `GET /api/logs`, and `GET /api/{service}/topology`.

## Services

| Service | Port | Binary | Purpose |
| -- | -- | -- | -- |
| `credits/` | `:8800` | `goop2-service-credits` | Credit balances, template pricing, purchases |
| `registrations/` | `:8801` | `goop2-service-registration` | User registration, email verification |
| `email/` | `:8802` | `goop2-service-email` | SMTP sending, HTML email templates |
| `templates/` | `:8803` | `goop2-service-templates` | Template store, bundling, pricing |
| `bridge/` | `:8804` | `goop2-service-bridge` | WebSocket bridge for browser-only peers |
| `encryption/` | `:8805` | `goop2-service-encryption` | NaCl key exchange, broadcast key distribution |

## Credits service (:8800)

**Endpoints:**

- `GET /api/credits/balance?email=X` — get balance
- `POST /api/credits/grant` — grant credits
- `POST /api/credits/spend` — deduct credits + grant ownership
- `GET /api/credits/prices` — list template prices (proxied to templates service)
- `POST /api/credits/prices` — set template price (admin)
- `GET /api/credits/access?email=X&template_dir=Y` — check download access
- `GET /api/credits/store-data?email=X` — full store status (balance, owned_templates, verification)
- `GET /api/credits/template-info?email=X&template_dir=Y` — price + ownership for one template
- `GET /api/credits/accounts` — list all accounts (admin)

**Calls:** registrations (email+token validation), templates (price lookups)

## Registrations service (:8801)

**Endpoints:**

- `GET /api/reg/status` — registration requirements, dummy_mode, grant_amount
- `GET /api/reg/verified?email=X` — check if email is verified
- `POST /api/reg/register` — create registration + send verification email
- `GET /api/reg/verify?token=X` — verify token, mark verified, trigger credit grant
- `POST /api/reg/validate` — check email+token pair validity
- `GET /api/reg/registrations` — list all (admin)
- `DELETE /api/reg/registrations?email=X` — delete registration (admin)

**Calls:** email (send verification), credits (grant registration bonus, check existing balance)

## Email service (:8802)

**Endpoints:**

- `POST /api/email/send` — send email using template (`{"to", "template", "subject", "data"}`)
- `GET /api/email/templates` — list available templates
- `GET /api/email/status` — service status, SMTP config

**No inter-service dependencies.** SMTP integration or dummy mode (log to console).

## Templates service (:8803)

**Endpoints:**

- `GET /api/templates` — list all templates with prices
- `GET /api/templates/prices` — all template prices
- `GET /api/templates/price?template_dir=X` — single price
- `POST /api/templates/prices` — set price (admin)
- `GET /api/templates/{dir}/manifest` — manifest.json for template
- `GET /api/templates/{dir}/bundle` — download .tar.gz (validates email+token, calls credits/spend)

**Calls:** registrations (validate email+token on download), credits (spend on download)

Templates loaded from disk on startup (primary `templates_dir` + overlays from `extra_dirs`). All files cached in-memory.

## Bridge service (:8804)

WebSocket bridge for browser-only peers (no libp2p). Creates `VirtualPeer` structs in memory.

**Endpoints:**

- `POST /api/bridge/token` — issue bridge token (admin, emails token to peer)
- `GET /api/bridge/peers` — list virtual peers
- `POST /api/bridge/peers` — register/reconnect virtual peer (bridge token required)
- `GET /api/bridge/peers/{peerID}` — get single peer
- `DELETE /api/bridge/peers/{peerID}` — disconnect peer
- `POST /api/bridge/peers/{peerID}/ping` — keepalive
- `GET /api/bridge/ws/{peerID}` — WebSocket upgrade (`X-Goop-Email` + `X-Bridge-Token`)
- `GET /api/bridge/status` — virtual peer count, ws_url

**WebSocket protocol:** JSON messages with `{"type": "ping"|"mq", "data": {...}}`

**Calls:** rendezvous (publish presence online/offline), registrations (verify peer), email (send bridge tokens)

Cleanup loop removes stale peers after 1 min idle.

## Encryption service (:8805)

NaCl encryption: X25519 key exchange + XSalsa20-Poly1305 (via `nacl/box`).

**Endpoints:**

- `POST /api/encryption/keys` — upload peer public key
- `GET /api/encryption/keys/{peerID}` — get peer's public key
- `GET /api/encryption/broadcast-key?peer_id=X` — broadcast key sealed for peer
- `POST /api/encryption/rotate` — force broadcast key rotation (admin)
- `GET /api/encryption/status` — key age, peer count, rotation interval, server public key

Auto-rotates broadcast key (default: 60 min). Broadcast keys are NaCl box-sealed per peer.

## Dependency chain

```
registrations → email        (verification emails)
registrations → credits      (grant registration bonus)
credits       → registrations (validate email+token)
credits       → templates    (price lookups)
templates     → registrations (validate email+token on download)
templates     → credits      (spend on download)
bridge        → rendezvous   (publish virtual peer presence)
bridge        → registrations (verify peer)
bridge        → email        (send bridge tokens)
email         → (none)
encryption    → (none)
```

## Gateway proxy

Config fields in `goop.json` under `presence`:

| Field | Default | Purpose |
| -- | -- | -- |
| `credits_url` | `http://localhost:8800` | Credits service |
| `registration_url` | `http://localhost:8801` | Registration service |
| `email_url` | `http://localhost:8802` | Email service |
| `templates_url` | `http://localhost:8803` | Template store |
| `bridge_url` | `http://localhost:8804` | WebSocket bridge |

`RemoteCreditProvider` in `internal/rendezvous/` proxies credit operations from the rendezvous server to the credits service.

When `templates_url` is empty, uses `local_template_dir` for local template bundles.

## Shared types

`StoreMeta` / `TablePolicy` defined in both repos:

- `goop2/internal/rendezvous/template_meta.go`
- `goop2-services/templates/template_meta.go`

Same JSON shape — must be kept in sync manually.

`StoreMeta` fields: `Name`, `Description`, `Category`, `Icon`, `Dir`, `Source`, `Schemas` (ORM table names), `RequireEmail`, `DefaultRole`, `Tables` (legacy `map[string]TablePolicy`)

## Shared packages

**`logbuf/`** — thread-safe ring buffer (200 lines). Implements `io.Writer` for `log.SetOutput`. Provides `Handler()` for `GET /api/logs`.

**`data/`** — runtime directory for SQLite database files (`.db` + WAL files). Not a Go package.

## Common config fields

All services share:

- `addr` — listen address
- `app_name` — branding (default: "Goop2")
- `dummy_mode` — disables real operations, logs to console
- `admin_token` — Bearer token for admin endpoints (empty = no auth)

## Auth patterns

- **Email identity**: `X-Goop-Email` header or `?email=` query param
- **Verification token**: `X-Verification-Token` header (validated against registrations service)
- **Admin**: Bearer token in Authorization header
- **Bridge token**: `X-Bridge-Token` header (email + token pair stored in bridge DB)
