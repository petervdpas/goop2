# Architecture

## System overview

Two repos: `goop2` (gateway, `:8787`) and `goop2-services` (microservices). Zero imports between them — communication is purely HTTP.

```
┌──────────────────────────────────────────────────────────────────┐
│  goop2 peer                                                      │
│  ┌──────────┐  ┌─────┐  ┌───────┐  ┌─────────┐  ┌───────────┐  │
│  │ P2P Node │──│ MQ  │──│Groups │──│ Storage │──│  Viewer   │  │
│  │ (libp2p) │  │     │  │       │  │ (SQLite)│  │  (HTTP)   │  │
│  └────┬─────┘  └──┬──┘  └───┬───┘  └────┬────┘  └─────┬─────┘  │
│       │           │         │            │             │         │
│  GossipSub    Streams    TypeHandlers   ORM        /api/* + SSE │
│  mDNS         /goop/mq   listen,chat   Lua engine  /sdk/*      │
│  Relay        /1.0.0     cluster,files              /assets/*   │
│  DCUtR                   template,datafed           /p/{peer}/  │
└──────────────────────────────────────────────────────────────────┘
         │                                                 │
    libp2p streams                                    Browser (JS)
    + GossipSub                                       EventSource
    + WebSocket                                       fetch/POST
         │                                                 │
┌────────┴──────────────────────────────────────────────────┴─────┐
│  Rendezvous server  (goop2.com / Pi)                            │
│  WebSocket presence, SSE fallback, relay info, punch hints      │
│  Credit proxy → goop2-services (credits, registration, email,   │
│                                  templates, bridge, encryption) │
└─────────────────────────────────────────────────────────────────┘
```

## Entry point

`main.go` parses flags and selects a mode:

| Command | Mode | Handler |
| -- | -- | -- |
| `goop2` (no args) | Desktop | `runDesktopApp()` → Wails UI |
| `goop2 peer <dir>` | CLI peer | `app.Run()` → `modes.RunPeer()` |
| `goop2 rendezvous <dir>` | Rendezvous server | `app.Run()` → `modes.RunRendezvous()` |

Config is loaded via `config.Load(cfgPath)` from `goop.json`. If missing, `config.Ensure(cfgPath)` creates defaults. Signal handling (SIGTERM/SIGINT) triggers graceful shutdown via context cancellation.

## Peer startup sequence

`internal/app/modes/peer.go` — `RunPeer()` initializes everything in strict dependency order:

### Step 1 — Rendezvous clients

- Create `rendezvous.Client` for local rendezvous (if `RendezvousHost` set)
- Create `rendezvous.Client` for WAN rendezvous (if `RendezvousWAN` URL set)
- `WarmDNS()` on all clients, filter unreachable ones
- Fetch `RelayInfo` from WAN rendezvous (parallel goroutines) for circuit relay

### Step 2 — Peer table

- `state.NewPeerTable()` — in-memory peer registry, the single source of truth for peer identity

### Step 3 — P2P node

- `p2p.New(ctx, listenPort, keyFile, peers, selfContent, selfEmail, ..., relayInfo, presenceTTL)`
- Loads/generates Ed25519 identity key from `keyFile`
- Creates libp2p host with: TCP + QUIC + WebSocket + WSS transports, Yamux muxer, circuit relay v2 (if relay available), hole-punching + AutoRelay, mDNS discovery
- Creates GossipSub pubsub, joins `goop.presence.v1` topic
- Registers stream handlers: `/goop/content/1.0.0` (probe), `/goop/diag/1.0.0` (diagnostics), `/goop/relay-refresh/1.0.0` (relay pulse)
- Subscribes to connection events for immediate peer discovery

### Step 4 — Message queue

- `mq.New(node.Host)` — registers `/goop/mq/1.0.0` stream handler
- Sets E2E encryptor (NaCl box) if configured: `sealKeyFor` (encrypt outbound if peer supports), `openKeyFor` (decrypt inbound)

### Step 5 — Avatar and storage

- `avatar.NewStore(peerDir)` + `avatar.NewCache(peerDir)` → `node.EnableAvatar()`
- `storage.Open(peerDir)` → SQLite at `<peerDir>/data.db`
- System tables created (see Storage section below)
- Load cached peers from DB → `peers.Seed()` + add addresses to libp2p peerstore

### Step 6 — Canonical resolver and identity handlers

- `resolvePeer(peerID)` function created — the ONE identity resolver (see identity.md)
- MQ subscribe to `identity` topic → respond with full identity
- MQ subscribe to `identity.response` → upsert into PeerTable

### Step 7 — Rendezvous WebSocket presence

- For each rendezvous client: `cc.ConnectWebSocket(ctx, nodeID, rvOnMsg)`
- `rvOnMsg` callback handles `TypeOnline`/`TypeUpdate`/`TypePunch`/`TypeOffline`
- Updates PeerTable, adds peer addresses, probes reachability, warms avatar cache

### Step 8 — MQ presence bridge

- Subscribes to `peers.Subscribe()` (PeerTable change events)
- Publishes `peer:announce` and `peer:gone` via `mqMgr.PublishLocal()` → delivered to browser SSE

### Step 9 — Service managers (dependency order)

| Order | Manager | Constructor | Group type |
| -- | -- | -- | -- |
| 1 | Direct chat | `directchat.New(nodeID, dbStore, mqMgr)` | — |
| 2 | Lua engine | `luapkg.NewEngine(...)` (if enabled) | — |
| 3 | Group manager | `group.New(host, db, mqMgr, resolvePeer)` | — |
| 4 | Call manager | `call.New(sigAdapter, nodeID, ...)` (Linux only) | — |
| 5 | Listen room | `listen.New(host, grpMgr, mqMgr, nodeID, peerDir)` | `"listen"` |
| 6 | Chat rooms | `chat.New(grpMgr, transport, nodeID, resolvePeer)` | `"chat"` |
| 7 | Cluster compute | `clusterType.New(mqMgr, grpMgr, nodeID)` | `"cluster"` |
| 8 | File sharing | `filesType.New(mqMgr, grpMgr, docStore)` | `"files"` |
| 9 | Data federation | `datafed.New(mqMgr, grpMgr, nodeID, gqlEngine)` | `"datafed"` |
| 10 | Template handler | `templateType.New(grpMgr)` | `"template"` |

### Step 10 — First presence publish

- `publish(ctx, proto.TypeOnline)` → GossipSub + all rendezvous clients (WebSocket preferred, HTTP POST fallback)

### Step 11 — HTTP viewer server

- `viewer.Start(addr, Viewer{...})` with all managers and `resolvePeer` in `Deps`
- `content.NewStore(peerDir, siteRoot)` for static site files
- Route registration (see HTTP routes section below)

### Step 12 — Background loops

- **Presence loop**: `node.RunPresenceLoop(ctx, callback)` — reads GossipSub messages, upserts peers
- **Heartbeat loop**: publishes `TypeUpdate` every `cfg.Presence.HeartbeatSec` seconds
- **Prune loop**: `peers.PruneStale(ttlCutoff, graceCutoff)` at regular intervals
- **Relay refresh**: periodic relay circuit refresh (if relay available)

### Step 13 — Shutdown

- `publish(context.Background(), proto.TypeOffline)` — tells all peers we're leaving
- Clear avatar cache, close all managers

## Rendezvous-only mode

`modes.RunRendezvous()` — starts only the rendezvous server with optional minimal viewer (no P2P endpoints). The rendezvous server handles:

- WebSocket presence hub (broadcast peer:announce/offline to all connected peers)
- SSE fallback for peers that can't use WebSocket
- Punch hints for NAT traversal (per-pair cooldown)
- Relay info distribution
- Credit proxy to goop2-services microservices
- Peer verification (email token validation via registration service)

## Package structure

| Package | Purpose |
| -- | -- |
| `internal/p2p` | libp2p node, stream protocols, peer discovery, NAT traversal, relay |
| `internal/mq` | Message queue protocol (`/goop/mq/1.0.0`), SSE delivery, topic routing, E2E encryption |
| `internal/group` | Group manager, `TypeHandler` interface, host/client message routing, MQ subscriptions |
| `internal/group_types/listen` | Audio room: CRDT state, WebSocket audio relay, playlist |
| `internal/group_types/cluster` | Compute: job queue, worker dispatch, result aggregation |
| `internal/group_types/files` | Document sharing: file store, `/goop/docs/1.0.0` protocol |
| `internal/group_types/chat` | Group-bounded chat rooms with ring buffer history |
| `internal/group_types/template` | Template group lifecycle, schema cleanup |
| `internal/group_types/datafed` | GraphQL federation over P2P, peer data sources |
| `internal/storage` | SQLite database, system tables, ORM table management |
| `internal/orm` | Schema validation, access enforcement, query building |
| `internal/orm/schema` | ORM schema types, merge logic, role resolution |
| `internal/orm/gql` | GraphQL engine over ORM-managed tables |
| `internal/orm/transformation` | Data transformations (ETL) |
| `internal/lua` | Lua sandbox engine, `goop.*` API surface |
| `internal/luaprefabs` | Prefab Lua libraries for templates |
| `internal/viewer` | HTTP server creation, static asset serving, peer site proxy |
| `internal/viewer/routes` | All HTTP route handlers, organized by domain |
| `internal/ui` | Go HTML templates, CSS, JavaScript for admin viewer |
| `internal/sdk` | JavaScript SDK files served at `/sdk/` for template viewers |
| `internal/sitetemplates` | Built-in embedded templates (blog, clubhouse, tictactoe, todo, enquete) |
| `internal/rendezvous` | Rendezvous/relay server + client, WebSocket presence, SSE, credit proxy |
| `internal/state` | In-memory `PeerTable` with event subscriptions, `PeerIdentityPayload` |
| `internal/proto` | Wire types: `PresenceMsg`, protocol IDs, constants |
| `internal/config` | Config struct, defaults, validation, `goop.json` loading |
| `internal/content` | Site content store (file listing, serving) |
| `internal/avatar` | Avatar image store and in-memory cache |
| `internal/directchat` | Direct P2P chat manager, DB-backed history, Lua command dispatch |
| `internal/call` | Native WebRTC (Go/Pion) for Linux desktop |
| `internal/crypto` | NaCl box E2E encryption, key management |
| `internal/bridge` | WebSocket bridge for Wails desktop |
| `internal/app` | Application bootstrap |
| `internal/app/modes` | Peer and rendezvous startup orchestration |
| `internal/app/shared` | Shared options struct across modes |
| `internal/util` | DNS cache, timeouts, helpers |

## Protocol layers

### P2P stream protocols (libp2p)

| Protocol ID | Purpose | Payload |
| -- | -- | -- |
| `/goop/mq/1.0.0` | Message queue | Newline-delimited JSON (`MQMsg` → `MQAck`) |
| `/goop/content/1.0.0` | Reachability probe | Single line: peer's display name |
| `/goop/diag/1.0.0` | Relay diagnostics | Diagnostic snapshot JSON |
| `/goop/relay-refresh/1.0.0` | Relay pulse | Rendezvous triggers relay circuit refresh |
| `/goop/docs/1.0.0` | Document transfer | File content |

### GossipSub topic

| Topic | Purpose |
| -- | -- |
| `goop.presence.v1` | Peer presence broadcast (LAN + relay). Carries `PresenceMsg` |

### MQ topics (application layer, over `/goop/mq/1.0.0`)

| Topic | Direction | Purpose |
| -- | -- | -- |
| `peer:announce` | local → browser (SSE) | PeerTable change → browser peer cache update |
| `peer:gone` | local → browser (SSE) | Peer removed from PeerTable |
| `identity` | requester → unknown peer | "Who are you?" fire-and-forget |
| `identity.response` | responder → requester | Full `PeerIdentityPayload` |
| `chat` | peer ↔ peer | Direct P2P chat messages |
| `chat.broadcast` | peer → all | Broadcast chat (ephemeral) |
| `chat.room:{groupID}:{type}` | group members | Group-bounded chat |
| `call:{channelID}` | peer ↔ peer | WebRTC signaling (offer, answer, ICE, hangup) |
| `call:loopback:{channelID}` | local Go → browser | Native Pion LocalPC → browser ICE |
| `group:{groupID}:{type}` | host ↔ members | Group protocol messages (join, leave, event, ping) |
| `group.invite` | host → invitee | Group invitation |
| `listen:{groupID}:state` | room host → members | Audio room state updates |
| `log:mq` | local only | MQ event logs for debug UI |
| `log:call` | local only | Call event logs |
| `relay:status` | local only | Relay connection status |
| `mq.ack` | peer → peer | Application-level delivery ACK |

**Suppressed from SSE**: `identity`, `identity.response`, `log:*`, `mq.ack` — internal plumbing only.

### SSE endpoints (browser EventSource)

| Endpoint | Purpose | Event types |
| -- | -- | -- |
| `GET /api/mq/events` | MQ bus → browser | `connected`, `message` (all non-suppressed MQ topics), `delivered` |
| `GET /api/logs/stream` | Log tail → browser | `message` (log entries: level, source, timestamp, text) |
| `GET /api/groups/events` | Group lifecycle → browser | `welcome`, `members`, `msg`, `state`, `leave`, `close`, `error`, `invite` |

## Config structure

`internal/config/config.go`:

```
Config
├── Identity    { KeyFile }
├── Paths       { SiteRoot, SiteSource, SiteStage }
├── P2P         { ListenPort, MdnsTag, BridgeMode, NaClPublicKey, NaClPrivateKey }
├── Presence    { Topic, TTLSec, HeartbeatSec,
│                 RendezvousHost, RendezvousPort, RendezvousBind,
│                 RendezvousWAN, RendezvousOnly,
│                 RelayPort, RelayWSPort,
│                 UseServices, CreditsURL, RegistrationURL, EmailURL,
│                 TemplatesURL, BridgeURL, EncryptionURL }
├── Profile     { Label, Email, VerificationToken, BridgeToken }
├── Viewer      { HTTPAddr, Theme, ClusterBinaryPath, PeerOfflineGraceMin }
└── Lua         { Enabled, ScriptDir, Timeouts, RateLimits }
```

Strict validation: paths, ports, TTL > Heartbeat ordering, rendezvous config consistency.

## Storage (SQLite)

`internal/storage/db.go` — `Open(configDir)` creates `<configDir>/data.db` with WAL mode, foreign keys ON.

### System tables

| Table | Purpose |
| -- | -- |
| `_meta` | Key-value metadata (template_tables, etc.) |
| `_tables` | Table registry (name, schema, insert_policy) |
| `_orm_schemas` | ORM schema definitions (table_name, schema_json) |
| `_groups` | Hosted groups (id, name, owner, type, context, max_members, volatile) |
| `_group_subscriptions` | Joined remote groups (host_peer_id, group_id) |
| `_group_members` | Group membership (group_id, peer_id, role) |
| `_cluster_jobs` | Cluster compute jobs (id, group_id, type, mode, payload, status, result) |
| `_peer_cache` | Cached peer identity (peer_id, content, email, avatar_hash, addrs, protocols, last_seen) |
| `_chat_messages` | Chat history (id, peer_id, from_id, content, ts) |
| `_favorites` | Favorited peers (peer_id, content, email, avatar_hash) |

## Group manager

`internal/group/manager.go` — single `Manager` instance per peer.

### TypeHandler interface

```
TypeHandler
├── Flags() GroupTypeFlags     // HostCanJoin, Volatile
├── OnCreate(groupID, name, maxMembers) error
├── OnJoin(groupID, peerID, isHost)
├── OnLeave(groupID, peerID, isHost)
├── OnClose(groupID)
└── OnEvent(evt *Event)
```

### Group MQ message types

| Type | Direction | Purpose |
| -- | -- | -- |
| `join` | client → host | Request to join group |
| `leave` | client → host | Leave group |
| `close` | host → members | Group closed by host |
| `members` | host → members | Broadcast full member list |
| `event` | bidirectional | Generic payload event (type-specific) |
| `ping` | host → members | Keepalive |

## HTTP viewer server

`internal/viewer/viewer.go` — `Start(addr, Viewer{...})` creates `http.ServeMux` and registers all routes.

### Deps struct

All route handlers receive `routes.Deps` — a flat struct containing every subsystem:

```
Deps {
    Node, SelfLabel, SelfEmail, Peers, CfgPath, Cfg, BaseURL, PeerDir,
    DB, AvatarStore, AvatarCache, RVClients, ResolvePeer, DocsStore,
    GroupManager, TemplateHandler, EnsureLua, LuaCall,
    Content, Logs, BridgeURL, TopologyFunc, RendezvousOnly, RendezvousURL
}
```

### Route registration order

```
viewer.Start()
├── render.InitTemplates()           // Go HTML template compilation
├── Static: /assets/, /sdk/          // Embedded CSS/JS
├── Proxy: /p/{peerID}/              // Remote peer site proxy
├── routes.Register(mux, deps)       // Main route group:
│   ├── Home, Self, Editor, Peers, Database, Groups pages
│   ├── /api/logs/*, /api/peer/*, /api/settings/*, /api/avatar/*
│   ├── /api/site/*, /api/docs/*, /api/fs/*
│   └── /api/data/lua/*, template routes, export routes
├── routes.RegisterMQ(mux, mq)       // /api/mq/send, /api/mq/ack, /api/mq/events
├── routes.RegisterChat(mux, chat)   // /api/chat/history
├── routes.RegisterData(mux, db)     // /api/data/* (ORM, tables, schemas)
├── routes.RegisterGraphQL(mux, gql) // /api/graphql
├── routes.RegisterGroups(mux, grp)  // /api/groups/*
├── routes.RegisterCluster(mux, cl)  // /api/cluster/*
├── routes.RegisterCall(mux, call)   // /api/call/* (nil on non-Linux)
├── routes.RegisterListen(mux, lis)  // /api/listen/*
├── routes.RegisterChatRooms(mux, cr)// /api/chat/rooms/*
├── routes.RegisterDataProxy(mux, n) // P2P data proxy
├── routes.RegisterDataFed(mux, df)  // /api/datafed/*
└── http.ListenAndServe(addr, mux)
```

### Complete API route map

**MQ** (`/api/mq/`)
| Method | Path | Purpose |
| -- | -- | -- |
| POST | `/api/mq/send` | Send P2P message `{peer_id, topic, payload}` → `{msg_id, status}` |
| POST | `/api/mq/ack` | App-level ACK `{msg_id, from_peer_id}` |
| GET | `/api/mq/events` | SSE stream (EventSource) |

**Peers** (`/api/peers/`, `/api/peer/`, `/api/self`)
| Method | Path | Purpose |
| -- | -- | -- |
| GET | `/api/peers` | List all peers (from PeerTable) |
| GET | `/api/self` | Current peer identity `{id, label, email}` |
| GET | `/api/peer/content?id=` | Fetch remote peer content via P2P probe |
| POST | `/api/peers/favorite` | Toggle peer favorite |
| POST | `/api/peers/probe` | Ping all peers |

**Data** (`/api/data/`)
| Method | Path | Purpose |
| -- | -- | -- |
| GET | `/api/data/tables` | List tables |
| POST | `/api/data/tables/create` | Create table |
| POST | `/api/data/tables/delete` | Drop table |
| POST | `/api/data/tables/describe` | Get schema |
| POST | `/api/data/tables/rename` | Rename table |
| POST | `/api/data/tables/add-column` | Add column |
| POST | `/api/data/tables/drop-column` | Drop column |
| POST | `/api/data/tables/set-policy` | Set access policy |
| POST | `/api/data/query` | Raw SQL query |
| POST | `/api/data/insert`, `/update`, `/delete` | CRUD |
| POST | `/api/data/find`, `/find-one`, `/get-by` | ORM queries |
| POST | `/api/data/pluck`, `/distinct`, `/count`, `/aggregate` | ORM aggregation |
| POST | `/api/data/upsert`, `/update-where`, `/delete-where` | ORM bulk ops |
| GET | `/api/data/schemas` | List ORM schemas |
| POST | `/api/data/schemas/get`, `/save`, `/delete`, `/ddl`, `/apply` | Schema management |
| GET | `/api/data/orm-schema` | ORM schema for templates |
| GET | `/api/data/lua/list` | List Lua functions |
| POST | `/api/data/lua/call` | Call Lua function |

**Groups** (`/api/groups/`)
| Method | Path | Purpose |
| -- | -- | -- |
| GET | `/api/groups` | List hosted groups |
| POST | `/api/groups` | Create group |
| POST | `/api/groups/join` | Join remote group |
| POST | `/api/groups/join-own` | Host joins own group |
| POST | `/api/groups/leave` | Leave group |
| POST | `/api/groups/leave-own` | Host leaves own group |
| POST | `/api/groups/close` | Delete group |
| POST | `/api/groups/send` | Send message to group |
| POST | `/api/groups/invite` | Invite peer |
| POST | `/api/groups/kick` | Remove member |
| POST | `/api/groups/max-members` | Set group size limit |
| POST | `/api/groups/meta` | Update group metadata |
| POST | `/api/groups/rejoin` | Rejoin after disconnect |
| POST | `/api/groups/set-role` | Set member role |
| POST | `/api/groups/set-default-role` | Default role for new members |
| POST | `/api/groups/set-roles` | Define role permissions |
| GET | `/api/groups/subscriptions` | List joined groups |
| POST | `/api/groups/remove-subscription` | Remove subscription |
| GET | `/api/groups/events` | SSE stream |

**Call** (`/api/call/`)
| Method | Path | Purpose |
| -- | -- | -- |
| GET | `/api/call/mode` | `{mode: "native"|"browser", platform}` |
| GET | `/api/call/active` | Active call sessions |
| GET | `/api/call/debug` | Debug info with RTP stats |
| POST | `/api/call/start` | Initiate call `{channel_id, remote_peer}` |
| POST | `/api/call/accept` | Accept incoming call |
| POST | `/api/call/hangup` | End call |
| POST | `/api/call/toggle-audio` | Mute/unmute |
| POST | `/api/call/toggle-video` | Camera toggle |
| POST | `/api/call/loopback/{ch}/offer` | Loopback SDP offer |
| POST | `/api/call/loopback/{ch}/ice` | Loopback ICE candidate |
| WS | `/api/call/media/{ch}` | WebSocket: live WebM stream |
| WS | `/api/call/self/{ch}` | WebSocket: self-view VP8 |
| HTTP | `/api/call/video/{ch}` | HTTP chunked: video stream |
| HTTP | `/api/call/selfvideo/{ch}` | HTTP chunked: self-view |

**Listen** (`/api/listen/`)
| Method | Path | Purpose |
| -- | -- | -- |
| GET | `/api/listen/state` | Current room state |
| POST | `/api/listen/create` | Create room |
| POST | `/api/listen/close` | Close room |
| POST | `/api/listen/load` | Load playlist |
| POST | `/api/listen/queue/add` | Add to queue |
| POST | `/api/listen/control` | Play/pause/seek |
| POST | `/api/listen/join` | Join room |
| POST | `/api/listen/leave` | Leave room |
| HTTP | `/api/listen/stream` | Audio stream URL |

**Chat** (`/api/chat/`)
| Method | Path | Purpose |
| -- | -- | -- |
| GET | `/api/chat/history?peer_id=` | Chat history with peer |
| DELETE | `/api/chat/history?peer_id=` | Clear history |

**Avatar** (`/api/avatar/`)
| Method | Path | Purpose |
| -- | -- | -- |
| GET | `/api/avatar` | Current peer's avatar |
| GET | `/api/avatar/peer/{id}` | Another peer's avatar |
| POST | `/api/avatar/upload` | Upload avatar (FormData) |
| DELETE | `/api/avatar/delete` | Delete avatar |

**Site** (`/api/site/`)
| Method | Path | Purpose |
| -- | -- | -- |
| GET | `/api/site/files` | List site files |
| GET | `/api/site/content?path=` | Read file |
| POST | `/api/site/upload` | Upload file (FormData) |
| POST | `/api/site/upload-local` | Copy from filesystem |
| GET | `/api/site/export` | Download site as .zip |
| POST | `/api/site/import` | Import site from .zip |

**Docs** (`/api/docs/`)
| Method | Path | Purpose |
| -- | -- | -- |
| GET | `/api/docs/my?group_id=` | List docs in group |
| GET | `/api/docs/browse?group_id=` | Browse shared docs |
| POST | `/api/docs/delete` | Delete doc |
| GET | `/api/docs/download` | Download doc (with peer_id, inline flag) |
| POST | `/api/docs/upload` | Upload doc (FormData) |
| POST | `/api/docs/upload-local` | Upload from filesystem |

**Other**
| Method | Path | Purpose |
| -- | -- | -- |
| POST | `/api/graphql` | Execute GraphQL query |
| POST | `/api/graphql/rebuild` | Rebuild schema |
| GET | `/api/graphql/status` | Schema status |
| GET | `/api/settings/quick/get` | Quick settings |
| POST | `/api/settings/quick` | Save quick settings |
| GET | `/api/services/health` | Check all services |
| GET | `/api/services/check?url=&type=` | Check single service |
| GET | `/api/fs/browse?dir=` | Browse filesystem |
| GET | `/api/topology` | Peer topology graph |
| POST | `/api/logs/client` | Client-side log entry |
| GET/POST | `/api/logs/verbose` | Get/set verbose flag |
| GET | `/api/cluster/*` | Cluster management (status, jobs, workers, stats) |
| POST | `/api/cluster/*` | Cluster ops (create, join, leave, submit, cancel, pause, resume) |
| GET | `/api/datafed/*` | Data federation (groups, contributions) |
| POST | `/api/datafed/*` | Federation ops (offer, withdraw) |

## Two UI layers

### Admin viewer (Go templates + viewer JS)

The local admin interface at `http://localhost:8787`. Loaded from `internal/ui/`:

- **Templates**: `internal/ui/templates/*.html` — Go `html/template` with layout, partials
- **JavaScript**: `internal/ui/assets/js/` — page-specific JS, NOT SDK
  - `core.js` — DOM utilities, `api()` fetch wrapper, validation
  - `api.js` — typed HTTP client mirroring all API endpoints (`Goop.api.mq.send()`, `Goop.api.groups.list()`, etc.)
  - `mq/topics.js` — topic constants + typed subscribe/send helpers (`mq.onPeerAnnounce()`, `mq.sendCallRequest()`)
  - `mq/peers.js` — `_peerMeta` cache, `getPeer()`, `getPeerName()` (auto-subscribed to `peer:announce`/`peer:gone`)
  - `pages/peers.js` — peer list with search, broadcast chat, call buttons
  - `pages/peer.js` — single peer chat (session-only ring buffer), emoji
  - `pages/self.js` — settings, avatar upload, service health checks
  - `pages/groups.js`, `database.js`, `logs.js`, `call.js`, `editor.js`, etc.
- **Data attributes on `<body>`**: `data-self-id`, `data-bridge-url`, `data-split-prefs`
- **Template variables**: `.SelfID`, `.SelfName`, `.SelfEmail`, `.BaseURL`, `.Peers`, `.Groups`, `.CSRF`, `.Theme`, `.Debug`

### Template viewer (SDK JS)

Remote peer content served at `/p/{peerID}/`. Templates load SDK files via `<script src="/sdk/goop-*.js">`.

**SDK files** (`internal/sdk/`):

| File | Namespace | Purpose |
| -- | -- | -- |
| `goop-mq.js` | `Goop.mq` | Subscribe/send MQ, SSE via `/api/mq/events`, auto-reconnect |
| `goop-identity.js` | `Goop.identity` | `get()`, `id()`, `label()`, `email()`, `resolveName()` |
| `goop-peers.js` | `Goop.peers` | `list()`, `subscribe(callbacks, pollMs)` — polls `/api/peers` |
| `goop-data.js` | `Goop.data` | `orm("table")` → handle with find/insert/update/delete/etc. Also `tables()`, `schemas()`, `call(fn, params)` for Lua |
| `goop-group.js` | `Goop.group` | `join()`, `send()`, `leave()`, `subscribe()` via SSE |
| `goop-realtime.js` | `Goop.realtime` | `connect(peerId)` → virtual MQ channel, `accept()`, `onIncoming()` |
| `goop-router.js` | `Goop.router` | `param()`, `page()`, `go(target)`, `home()` |
| `goop-api.js` | `Goop.api` | CRUD convenience over `Goop.data.call("api", ...)` |

**SDK utilities** (on `Goop` global):
- `Goop.esc(str)` — HTML escape
- `Goop.date(ts, opts)` — format timestamp
- `Goop.peer()` → `{myId, hostId, isOwner, isGroup, label}`
- `Goop.list(el, rows, renderFn, opts)` — list renderer with action handlers
- `Goop.overlay(id)` — overlay dialog helper
- `Goop.store(initial)` → reactive store: `{set, get, watch, update}`

## JavaScript ↔ Go interaction patterns

### Pattern 1: JSON fetch (most API calls)

```
fetch(url, {method: body ? "POST" : "GET", body: JSON.stringify(body)})
  .then(r => r.ok ? r.json() : r.text().then(t => throw Error(t)))
```

### Pattern 2: EventSource (SSE streaming)

```
new EventSource("/api/mq/events")
  .addEventListener("message", e => handle(JSON.parse(e.data)))
  // Auto-reconnect after 3s on error
```

### Pattern 3: WebSocket (media streaming)

```
new WebSocket(wsUrl("/api/call/media/" + channelId))
  ws.binaryType = "arraybuffer"
  // Binary WebM clusters → MediaSource.appendBuffer()
```

### Pattern 4: FormData (file upload)

```
new XMLHttpRequest().send(formData)  // avatar, site import, docs
```

## Dependency wiring (DAG)

```
Config
├── Identity key (Ed25519)
├── Rendezvous clients (WAN/LAN)
│   └── Relay discovery
├── PeerTable (in-memory)
├── P2P Node (libp2p host + GossipSub)
│   ├── Uses: identity key, relay info, PeerTable
│   └── Provides: Host for MQ/groups/streams
├── MQ Manager
│   ├── Uses: P2P Host
│   └── Provides: Send/Subscribe/PublishLocal for all managers
├── Database (SQLite)
│   ├── Provides: persistence for all managers
│   └── Seeds: PeerTable from _peer_cache
├── resolvePeer (closure)
│   └── Uses: PeerTable, DB, MQ (fallback)
├── Chat → Uses: DB, MQ
├── Lua → Uses: Config (lazy), all managers (lazy)
├── Group Manager → Uses: DB, MQ, resolvePeer
│   └── TypeHandlers registered after creation
├── Listen → Uses: Group, MQ, P2P Host
├── ChatRooms → Uses: Group, MQ, resolvePeer
├── Cluster → Uses: Group, MQ, DB
├── Files → Uses: Group, MQ, docs store
├── DataFed → Uses: Group, MQ, GraphQL engine
├── Templates → Uses: Group
└── Viewer (HTTP)
    └── Uses: ALL of the above via Deps struct
```

## What NOT to do

- **Do NOT create circular imports.** The DAG above is strict — downstream never imports upstream.
- **Do NOT bypass MQ for peer messaging.** All peer-to-peer communication goes through `/goop/mq/1.0.0`. The only exception is presence (GossipSub) and binary streams (content probe, docs, audio).
- **Do NOT create new SSE endpoints.** Use MQ `PublishLocal()` to deliver events to the browser via the existing `/api/mq/events` stream.
- **Do NOT pass individual manager references to routes.** Use the `Deps` struct.
- **Do NOT create separate identity caches.** `PeerTable` is THE cache. `resolvePeer` is THE resolver.
