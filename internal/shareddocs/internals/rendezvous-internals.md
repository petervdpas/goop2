# Rendezvous Server Internals

## Overview

The rendezvous server (`internal/rendezvous/`) runs inside the goop2 binary when `presence.rendezvous_host = true`. It serves as WAN peer discovery, presence relay, circuit relay, and gateway to microservices.

## Server struct

`internal/rendezvous/server.go`

Key state:

- `peers map[string]peerRow` — in-memory peer presence view
- `clients map[chan []byte]struct{}` — SSE client connections
- `wsClients map[string]*wsClient` — WebSocket clients keyed by peer ID
- `peerDB *peerDB` — optional SQLite persistence for peer state
- `relayHost host.Host` — circuit relay v2 libp2p host (nil when disabled)
- `relayInfo *RelayInfo` — relay multiaddresses for clients

## Microservice providers

Each remote service has a provider struct that proxies HTTP calls:

| Provider | Package file | Service |
| -- | -- | -- |
| `RemoteCreditProvider` | `remote_credits.go` | Credits (:8800) |
| `RemoteRegistrationProvider` | `remote_registration.go` | Registrations (:8801) |
| `RemoteEmailProvider` | `remote_email.go` | Email (:8802) |
| `RemoteTemplatesProvider` | `remote_templates.go` | Templates (:8803) |
| `RemoteBridgeProvider` | `remote_bridge.go` | Bridge (:8804) |
| `RemoteEncryptionProvider` | `remote_encryption.go` | Encryption (:8805) |
| `LocalTemplateStore` | `local_templates.go` | Local template bundles (fallback when templates_url empty) |

Providers are nil when the corresponding service URL is not configured.

## Presence relay

### HTTP presence (legacy)

- `POST /publish` — peer publishes presence message (rate-limited per IP)
- `GET /events` — SSE stream of presence events (limit: 1024 global, 10 per IP)

### WebSocket presence

`server_ws.go` — preferred transport:

- `GET /ws` — WebSocket upgrade, authenticated by peer ID
- Per-peer channel: `wsClients map[string]*wsClient`
- Limit: 100 WebSocket connections per IP
- Client sends JSON `PresenceMsg`, server relays to all other connected peers
- Server validates verification tokens and sets `verified` flag

### Peer state

`server_peers.go` — manages the in-memory peer map:

- `peerRow` struct: PeerID, Type, Content, Email, AvatarHash, ActiveTemplate, PublicKey, etc.
- Peers are tracked on online/update, removed on offline
- Optional persistence via `peerDB` (SQLite, configured by `presence.peer_db_path`)

## Circuit relay v2

`relay.go` — when `presence.relay_port > 0`:

- Starts a separate libp2p host as a circuit relay v2 server
- Provides `RelayInfo` (peer ID + multiaddresses) to connecting peers via `GET /relay`
- Timing config: cleanup delay, poll deadline, connect timeout, refresh interval, recovery grace

## Client

`internal/rendezvous/client.go`

Used by peers to connect to the rendezvous server:

- `Publish(ctx, PresenceMsg)` — HTTP POST to `/publish`
- `PublishWS(PresenceMsg)` — send via WebSocket (preferred)
- `ConnectWebSocket()` — persistent WS with state machine: WS connect → on 425 retry after backoff → on unsupported fall back to SSE + periodic WS probe → on probe success cancel SSE and switch to WS → on normal disconnect reconnect with exponential backoff (capped at 500ms)
- `SubscribeEvents(ctx)` — SSE subscription to `/events` with auto-reconnect and exponential backoff (capped at 500ms)
- `probeWS()` — lightweight WS dial + immediate close to check server support
- DNS caching for server hostname resolution

## Rate limiting

- `/publish` endpoint: per-IP rate limiter with fixed-size ring buffer (60 entries)
- Punch hint cooldowns: prevents spamming hole-punch attempts for the same peer pair

## Web UI

The rendezvous server serves its own web UI:

- Peer list page (embedded HTML templates)
- Admin panel (HTTP Basic Auth, password from config)
- Registration page (proxied to registrations service)
- Docs site (`docs.go` — serves shareddocs as HTML)
- Template store page
- Swagger API docs

## Shared types

`template_meta.go` — `StoreMeta` and `TablePolicy` structs, kept in sync with `goop2-services/templates/template_meta.go`.
