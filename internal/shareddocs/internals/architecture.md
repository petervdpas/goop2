# Architecture

## System overview

Two repos: `goop2` (gateway, `:8787`) and `goop2-services` (microservices). Zero imports between them — communication is purely HTTP.

### Peer lifecycle

1. **Startup**: `main.go` parses flags, loads config from `goop.json`, calls `app.Run()`
2. **Mode selection**: `app/run.go` → `app/modes/peer.go` (normal) or `app/modes/rendezvous.go` (server)
3. **P2P node**: Creates libp2p host, joins GossipSub presence topic, starts mDNS
4. **Storage**: Opens SQLite database at `<peerDir>/data.db`
5. **Services**: Wires MQ manager, group manager, chat manager, template handler, Lua engine
6. **Viewer**: Starts HTTP server on configured address, registers all route handlers
7. **Presence**: Publishes `online` presence message, begins heartbeat loop
8. **Discovery**: mDNS (LAN) + rendezvous server (WAN) discover peers → PeerTable.Upsert
9. **Shutdown**: Context cancellation propagates to all subsystems

### Package structure

| Package | Purpose |
| -- | -- |
| `internal/p2p` | libp2p node, stream protocols, peer discovery, NAT traversal |
| `internal/mq` | Message queue protocol, SSE delivery, topic routing |
| `internal/group` | Group manager, TypeHandler interface, host/client message routing |
| `internal/group_types/` | TypeHandler implementations: chat, template, files, listen, cluster, datafed |
| `internal/storage` | SQLite database, system tables, ORM table management |
| `internal/orm` | Schema validation, access enforcement, query building |
| `internal/lua` | Lua sandbox engine, `goop.*` API surface |
| `internal/viewer` | HTTP server, route registration, UI templates |
| `internal/ui` | Go templates, CSS, JavaScript for the admin viewer |
| `internal/sdk` | JavaScript SDK files served at `/sdk/` for template viewers |
| `internal/sitetemplates` | Built-in embedded templates (blog, clubhouse, tictactoe, todo, enquete) |
| `internal/rendezvous` | Rendezvous/relay server, WebSocket presence, credit proxy |
| `internal/app/modes` | Peer and rendezvous startup orchestration |
| `internal/app/shared` | Shared options and helpers across modes |
| `internal/proto` | Wire types: PresenceMsg, protocol IDs, constants |
| `internal/state` | In-memory PeerTable with event subscriptions |
| `internal/config` | Config struct, defaults, validation, `goop.json` loading |
| `internal/content` | Site content store (file listing, serving) |
