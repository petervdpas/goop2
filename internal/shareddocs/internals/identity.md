# Peer Identity and Presence

## The single rule

**Identity travels over the WebSocket presence channel (WAN) and GossipSub (LAN). Everything else resolves from the PeerTable.**

## Data structures

### PresenceMsg (wire format)

`internal/proto/proto.go` — the message published on GossipSub and sent over WebSocket/SSE to the rendezvous server:

```
PresenceMsg {
    Type                string   json:"type"                    // online|update|offline|punch
    PeerID              string   json:"peerId"
    Content             string   json:"content,omitempty"       // display name / label
    Email               string   json:"email,omitempty"
    AvatarHash          string   json:"avatarHash,omitempty"
    VideoDisabled       bool     json:"videoDisabled,omitempty"
    ActiveTemplate      string   json:"activeTemplate,omitempty"
    Target              string   json:"target,omitempty"        // punch hint: addressed peer ID
    Addrs               []string json:"addrs,omitempty"         // multiaddrs for WAN connectivity
    VerificationToken   string   json:"verificationToken,omitempty"
    PublicKey           string   json:"publicKey,omitempty"     // NaCl public key for E2E
    EncryptionSupported bool     json:"encryptionSupported,omitempty"
    GoopClientVersion   string   json:"goopClientVersion,omitempty"
    TS                  int64    json:"ts"                      // Unix milliseconds
    Verified            bool     json:"verified,omitempty"      // set by rendezvous server
}
```

Type constants: `TypeOnline = "online"`, `TypeUpdate = "update"`, `TypeOffline = "offline"`, `TypePunch = "punch"`

### SeenPeer (PeerTable entry)

`internal/state/peers.go` — the in-memory representation inside PeerTable:

```
SeenPeer {
    Content             string
    Email               string
    AvatarHash          string
    VideoDisabled       bool
    ActiveTemplate      string
    PublicKey           string
    EncryptionSupported bool
    Verified            bool
    GoopClientVersion   string
    Reachable           bool          // marked true after successful content probe
    LastSeen            time.Time
    OfflineSince        time.Time     // zero = online, non-zero = offline
    Favorite            bool
    failStreak          int           // (unexported) consecutive probe failures
    lastFailAt          time.Time     // (unexported) last failure timestamp
}
```

### PeerIdentityPayload (canonical identity)

`internal/state/identity.go` — THE single identity struct used everywhere: MQ wire format, resolver return type, route handler data:

```
PeerIdentityPayload {
    PeerID              string    json:"peerID"
    Content             string    json:"content"
    Email               string    json:"email,omitempty"
    AvatarHash          string    json:"avatarHash,omitempty"
    VideoDisabled       bool      json:"videoDisabled,omitempty"
    ActiveTemplate      string    json:"activeTemplate,omitempty"
    PublicKey           string    json:"publicKey,omitempty"
    EncryptionSupported bool      json:"encryptionSupported,omitempty"
    Verified            bool      json:"verified,omitempty"
    GoopClientVersion   string    json:"goopClientVersion,omitempty"
    Reachable           bool      json:"reachable"
    Offline             bool      json:"offline,omitempty"
    LastSeen            int64     json:"lastSeen,omitempty"          // Unix millis
    Favorite            bool      json:"favorite,omitempty"
    Known               bool      json:"-"                           // resolver flag, not on wire
    LastSeenTime        time.Time json:"-"                           // internal only
}
```

- `Name()` method returns `Content` field
- `FromSeenPeer(sp)` converts SeenPeer → PeerIdentityPayload, sets `Known=true`, computes `Offline` from `OfflineSince`

### PeerEvent (PeerTable change notification)

```
PeerEvent {
    Type   string              json:"type"     // "update" or "remove"
    PeerID string              json:"peer_id"
    Peer   *SeenPeer           json:"peer"     // non-nil for "update"
    Peers  map[string]SeenPeer json:"peers"    // snapshot (optional)
}
```

## Communication layers

| Layer | Transport | Carries | Identity? |
| -- | -- | -- | -- |
| Presence (WAN) | WebSocket to rendezvous (`/ws?peer_id=`) | `PresenceMsg` | **YES — primary source** |
| Presence (WAN fallback) | SSE from rendezvous (`/events`) | `PresenceMsg` | **YES — same data, SSE transport** |
| Presence (LAN) | GossipSub `goop.presence.v1` | `PresenceMsg` | **YES — same struct, pubsub transport** |
| MQ | libp2p stream `/goop/mq/1.0.0` | All peer-to-peer messages | No — `from` is raw peer ID from `stream.Conn().RemotePeer()` |
| Content probe | libp2p stream `/goop/content/1.0.0` | Single line: display name | Name only — reachability check, NOT identity |
| Data | libp2p stream `/goop/data/1.0.0` | ORM queries/responses | No |
| Site | libp2p stream `/goop/site/1.0.0` | File content | No |
| HTTP | Viewer HTTP server | Browser ↔ Go | No — browser gets identity via MQ SSE `peer:announce` |

## PeerTable — THE identity cache

`internal/state/peers.go` — single in-memory cache for all peer identity data.

```
PeerTable {
    mu        sync.Mutex
    peers     map[string]SeenPeer       // peer ID → identity
    listeners []chan PeerEvent           // change subscribers (buffered, cap 16)
}
```

### Writers (things that put identity INTO the PeerTable)

| Source | Method called | When |
| -- | -- | -- |
| Rendezvous WebSocket `rvOnMsg` callback | `peers.Upsert()` | TypeOnline/TypeUpdate from WAN |
| GossipSub presence handler (RunPresenceLoop) | `peers.Upsert()` | TypeOnline/TypeUpdate from LAN |
| MQ `identity.response` handler | `peers.Upsert()` | Response to on-demand identity request |
| DB cache seed on startup | `peers.Seed()` | Boot: restore known peers from `_peer_cache` |
| Content probe result | `peers.SetReachable()` | Successful/failed reachability check |
| Favorite toggle | `peers.SetFavorite()` | User action |
| Encryption key update | `peers.SetPublicKey()` | Key fetched from rendezvous |

### Readers (things that get identity FROM the PeerTable)

| Consumer | Access pattern |
| -- | -- |
| `resolvePeer()` | `peers.Get(id)` — O(1) mutex-guarded lookup |
| MQ presence bridge | `peers.Subscribe()` → chan PeerEvent → `PublishPeerAnnounce()` |
| Prune loop | `peers.PruneStale()` — scans all, removes stale |
| Peer list API (`/api/peers`) | `peers.Snapshot()` — full copy |
| Heartbeat/probe | `peers.Get()` for reachability check |

### Key PeerTable methods

**Upsert** — `func (t *PeerTable) Upsert(id, content, email, avatarHash string, videoDisabled bool, activeTemplate, publicKey string, encryptionSupported, verified bool, goopClientVersion string)`
- Preserves local state across updates: `Reachable`, `Favorite`, `failStreak`, `lastFailAt`
- Preserves `PublicKey` and `EncryptionSupported` if incoming update doesn't include them (zero values)
- Updates `LastSeen` to now, clears `OfflineSince`
- Broadcasts `PeerEvent{Type: "update"}` to all listeners

**Seed** — inserts only if not already present, marks `OfflineSince` to now (initially offline), broadcasts update

**SetReachable** — on success: resets failStreak, marks `Reachable=true`. On failure: increments failStreak only if >2s since last failure (dedup window: `PeerFailureDedupWindow`), marks `Reachable=false` only after `failStreak >= 2`

**PruneStale** — moves online peers past TTL to offline, removes offline peers past grace period, broadcasts events

**Subscribe/Unsubscribe** — creates/removes buffered channel (cap 16) for PeerEvent notifications

## The canonical resolver

`internal/app/modes/peer.go` defines ONE `resolvePeer` closure. Created once, passed to every subsystem via `Deps.ResolvePeer`.

```
resolvePeer(peerID) → PeerIdentityPayload
  1. Self?       → PeerIdentityPayload{PeerID, Content: selfContent(), Email: selfEmail(), Known: true}
  2. PeerTable?  → peers.Get(id) → FromSeenPeer(sp) [Known=true, instant, in-memory]
  3. DB cache?   → db.GetCachedPeer(id) → PeerIdentityPayload{..., Reachable: has_addrs, Known: true}
  4. Unknown     → fire-and-forget goroutine: mqMgr.Send(id, "identity", nil) with 2s timeout
                   returns empty PeerIdentityPayload{} [Known=false]
```

### Consumers

Every subsystem receives the same `resolvePeer` instance:

| Consumer | Uses |
| -- | -- |
| `group.Manager` | Member name resolution in `resolveMemberNames()` |
| `chat.Manager` (group rooms) | Message `FromName`, member list names |
| `routes.RegisterGroups` | Member names, host reachability in subscription list |
| `routes.RegisterListen` | Listener names |
| `routes.RegisterChatRooms` | Passed to chat room manager |
| `routes.Deps` | Used by docs route, peer routes |
| `viewer.Viewer` | Holds reference, passes to all route registrations |

## On-demand identity via MQ

When a peer sends an MQ message but isn't in the PeerTable yet (timing race — MQ stream opens before presence arrives):

### Flow

```
Peer A (unknown)                    Peer B (local)
     │                                    │
     │  MQ message arrives                │
     │ ──────────────────────────────────> │
     │                              resolvePeer(A) → Known=false
     │                              goroutine: MQ Send "identity" to A
     │  <──────────────────────────────── │
     │  MQ "identity" request             │
     │                                    │
     │  MQ "identity.response"            │
     │ ──────────────────────────────────> │
     │                              peers.Upsert(A, content, email, ...)
     │                              next resolvePeer(A) → Known=true
```

### MQ topic handlers

**Identity request** (`peer.go` lines 259-278):
```
Subscribe to "identity" → respond with:
  PeerAnnouncePayload{PeerID, Content, Email, AvatarHash, GoopClientVersion,
                       PublicKey, EncryptionSupported, ActiveTemplate, VideoDisabled,
                       Reachable: true}
  → sent back on "identity.response"
```

**Identity response** (`peer.go` lines 281-301):
```
Subscribe to "identity.response" → extract fields from map[string]any →
  peers.Upsert(from, content, email, avatarHash, videoDisabled, activeTemplate,
               publicKey, encryptionSupported, false, goopClientVersion)
```

Both topics are **suppressed from SSE** — internal plumbing, not browser events.

## Presence publishing

`peer.go` defines a `publish(ctx, typ)` function that broadcasts to ALL channels simultaneously:

```
publish(ctx, type)
├── node.Publish(ctx, type)                          // GossipSub → LAN peers
└── for each rendezvous client:
    ├── cc.PublishWS(pm) → true?                     // WebSocket (preferred, non-blocking)
    └── cc.Publish(ctx, pm)                          // HTTP POST fallback (with ShortTimeout)
```

The `PresenceMsg` payload includes: Content, Email, AvatarHash, VideoDisabled, ActiveTemplate, PublicKey, EncryptionSupported, VerificationToken, GoopClientVersion, Addrs (WAN multiaddrs), TS.

### When publish is called

| Trigger | Type | Purpose |
| -- | -- | -- |
| Startup (step 10) | `TypeOnline` | Initial announcement |
| Heartbeat ticker | `TypeUpdate` | Periodic refresh (every `HeartbeatSec`) |
| Address change (relay) | `TypeUpdate` | New relay address acquired |
| Shutdown | `TypeOffline` | Graceful departure |

## Presence receiving

### From GossipSub (LAN)

`node.RunPresenceLoop(ctx, callback)`:
- Reads from GossipSub subscription on `goop.presence.v1`
- Filters own messages (by peer ID)
- Calls `onEvent(pm)` for each `PresenceMsg`

The callback in `peer.go` handles each type:
- **TypeOnline**: `peers.Upsert()`, add addresses to peerstore, probe content, warm avatar
- **TypeUpdate**: `peers.Upsert()`, update addresses, probe if not already probed
- **TypeOffline**: `peers.MarkOffline(id)`

### From rendezvous WebSocket (WAN)

`rvOnMsg` callback in `peer.go`:
- **TypeOnline/TypeUpdate**: `peers.Upsert()`, add addresses, probe, warm avatar
- **TypePunch**: NAT hole-punch hint — attempt direct connection to target
- **TypeOffline**: `peers.MarkOffline(id)`

### WebSocket connection lifecycle

**Client side** (`rendezvous/client.go`):
- `ConnectWebSocket(ctx, peerID, onMsg)` → `ws://host/ws?peer_id=<id>` or `wss://`
- Auto-reconnect with exponential backoff
- Falls back to SSE (`SubscribeEvents` on `/events`) if WebSocket unavailable (404/403/501)
- Probes periodically to detect WebSocket upgrade availability
- Write pump: sends from buffered `sendCh` (cap 64)
- Read pump: receives TextMessage → unmarshal PresenceMsg → `onMsg(pm)`

**Server side** (`rendezvous/server_ws.go`):
- Requires peer to have published via `/publish` first (425 Too Early otherwise)
- Per-IP WebSocket limit (`maxWSClientsPerIP`)
- Validates email + verificationToken via registration service → sets `Verified` flag
- `upsertPeer(pm)` stores in server peer map
- `emitPunchHints(pm)` sends TypePunch to existing peers (with per-pair cooldown)
- On disconnect: broadcasts TypeOffline

## MQ presence bridge (PeerTable → browser)

`peer.go` subscribes to `peers.Subscribe()` and bridges changes to the browser SSE stream:

```
PeerTable change event
├── "update" → mqMgr.PublishPeerAnnounce(PeerAnnouncePayload{...})
│              → PublishLocal("peer:announce", "", payload)
│              → delivered to all SSE listeners → browser EventSource
└── "remove" → mqMgr.PublishPeerGone(peerID)
               → PublishLocal("peer:gone", "", {peerID})
               → go db.DeleteCachedPeer(peerID)
```

`PeerAnnouncePayload` carries: PeerID, Content, Email, AvatarHash, VideoDisabled, ActiveTemplate, PublicKey, EncryptionSupported, Verified, GoopClientVersion, Reachable, Offline, LastSeen, Favorite.

## Heartbeat and pruning

### Heartbeat loop

```
Ticker: every cfg.Presence.HeartbeatSec seconds
→ publish(ctx, proto.TypeUpdate)
→ GossipSub + all rendezvous clients
```

### Prune loop

```
Ticker: every PruneCheckInterval
├── Re-read PeerOfflineGraceMin from config every ConfigRereadInterval cycles
├── ttlCutoff = now - TTLSec
├── graceCutoff = now - PeerOfflineGraceMin (1-60 min, default 15)
└── peers.PruneStale(ttlCutoff, graceCutoff)
    ├── Online peers with LastSeen < ttlCutoff → mark offline (OfflineSince = now)
    ├── Offline peers with OfflineSince < graceCutoff → Remove (unless Favorite)
    └── Broadcast PeerEvent for each state change
```

### Timing constants

| Constant | Source | Purpose |
| -- | -- | -- |
| `HeartbeatSec` | `cfg.Presence.HeartbeatSec` | How often to publish TypeUpdate |
| `TTLSec` | `cfg.Presence.TTLSec` | How long before an online peer is considered stale |
| `PeerOfflineGraceMin` | `cfg.Viewer.PeerOfflineGraceMin` | Minutes to keep offline peers visible (1-60, default 15) |
| `PeerFailureDedupWindow` | `state/timings.go` (2s) | Minimum gap between counting probe failures |
| `PruneCheckInterval` | `peer.go` constant | How often to run the prune loop |
| `ConfigRereadInterval` | `peer.go` constant | How often to re-read grace period from disk |

## Frontend identity

### Admin viewer: MQ peer cache

`internal/ui/assets/js/mq/peers.js` maintains the browser-side peer identity cache:

```javascript
var _peerMeta = {};  // peerID → PeerAnnouncePayload

// Auto-subscribed at load:
mq.onPeerAnnounce(function(from, topic, payload, ack) {
    if (payload && payload.peerID) _peerMeta[payload.peerID] = payload;
    ack();
});
mq.onPeerGone(function(from, topic, payload, ack) {
    if (payload && payload.peerID) delete _peerMeta[payload.peerID];
    ack();
});

// Lookup:
Goop.mq.getPeer(peerID)     → _peerMeta[peerID] || null
Goop.mq.getPeerName(peerID) → (p && p.content) || null
```

This is the JS equivalent of `resolvePeer(id).Name()`.

### Admin viewer: topic registry

`internal/ui/assets/js/mq/topics.js` defines typed subscribe/send helpers:

```javascript
TOPICS = {
    PEER_ANNOUNCE: "peer:announce",
    PEER_GONE: "peer:gone",
    CALL_PREFIX: "call:",
    CALL_LOOPBACK_PREFIX: "call:loopback:",
    GROUP_PREFIX: "group:",
    GROUP_INVITE: "group.invite",
    LISTEN_PREFIX: "listen:",
    CHAT: "chat",
    CHAT_BROADCAST: "chat.broadcast",
    CHATROOM_PREFIX: "chat.room:",
    IDENTITY: "identity",
    IDENTITY_RESPONSE: "identity.response",
    LOG_MQ: "log:mq",
    LOG_CALL: "log:call",
    RELAY_STATUS: "relay:status"
}

// Typed subscribers: mq.onPeerAnnounce(fn), mq.onCall(fn), mq.onChat(fn), ...
// Typed senders: mq.sendCallRequest(peerId, channelId, constraints), mq.sendChat(peerId, payload), ...
```

### SDK: identity resolution

`internal/sdk/goop-identity.js` — used by template pages:

```javascript
Goop.identity.get()     → Promise → {id, label, email}  (via GET /api/self, cached)
Goop.identity.id()      → peer ID (from cache)
Goop.identity.label()   → display name (from cache)
Goop.identity.email()   → email (from cache)
Goop.identity.refresh() → clear cache, force re-fetch

Goop.identity.resolveName(peerId, serverName)
  → Priority: self → MQ cache (getPeerName) → serverName → truncated ID (last 6 chars)
```

### SDK: peer list

`internal/sdk/goop-peers.js` — used by template pages:

```javascript
Goop.peers.list()       → Promise → [peer objects]
Goop.peers.subscribe(callbacks, pollIntervalMs)
  // Polls /api/peers every N ms (default 5000)
  // callbacks: onSnapshot(peers), onUpdate(id, peer), onRemove(id), onError()

// Peer object shape (from /api/peers):
{
    ID, Content, Email, AvatarHash, VideoDisabled, ActiveTemplate,
    Verified, Reachable, Offline, LastSeen
}
```

## Rendezvous server presence management

`internal/rendezvous/server_peers.go` and `server_ws.go`:

### Server-side peer storage

```
peerRow {
    PeerID, Type, Content, Email, AvatarHash, ActiveTemplate,
    PublicKey, EncryptionSupported, Addrs, TS, LastSeen,
    BytesSent, BytesReceived, Verified, verificationToken, WSConnected
}
```

### Server-side operations

- **upsertPeer**: stores/updates peer, detects address changes (gates punch hints), validates verification token
- **emitPunchHints**: sends TypePunch to existing peers about new/changed peer (per-pair cooldown)
- **cleanupStalePeers**: timer-driven, removes peers not seen in 30s, broadcasts TypeOffline
- **broadcast**: sends to ALL connected HTTP SSE + WebSocket clients, increments byte counters
- **snapshotPeers**: returns sorted list (online first), caches until dirty

## Complete presence flow diagram

```
                    ┌─────────────────┐
                    │  Peer A startup │
                    └────────┬────────┘
                             │
                    publish(TypeOnline)
                    ┌────────┴────────┐
                    │                 │
              node.Publish()    cc.PublishWS(pm)
              (GossipSub)       (WebSocket to RV)
                    │                 │
                    ▼                 ▼
            ┌───────────┐    ┌──────────────┐
            │  Peer B   │    │  Rendezvous  │
            │ (LAN)     │    │  Server      │
            │           │    │              │
            │ RunPres.  │    │ upsertPeer() │
            │ Loop()    │    │ broadcast()  │
            │           │    │ punchHints() │
            └─────┬─────┘    └──────┬───────┘
                  │                 │
          peers.Upsert(A)    WebSocket/SSE
          peers.SetReachable  to Peer C (WAN)
                  │                 │
                  ▼                 ▼
            ┌──────────┐    ┌───────────┐
            │PeerTable │    │  Peer C   │
            │ (Peer B) │    │  (WAN)    │
            └────┬─────┘    │           │
                 │          │ rvOnMsg() │
          PeerEvent         │           │
          "update"          └─────┬─────┘
                 │                │
                 ▼          peers.Upsert(A)
          mqMgr.Publish           │
          PeerAnnounce            ▼
                 │          ┌──────────┐
                 ▼          │PeerTable │
          ┌──────────┐     │ (Peer C) │
          │ Browser  │     └────┬─────┘
          │ (Peer B) │          │
          │          │    PeerEvent → browser
          │ SSE:     │
          │ peer:    │
          │ announce │
          └──────────┘
```

## What NOT to do

- **Do NOT create new libp2p stream protocols for identity.** Identity comes from presence (WebSocket/GossipSub) with MQ fallback.
- **Do NOT resolve names differently in different places.** Use `resolvePeer`. One function, one instance.
- **Do NOT pass `func(string) string` for name-only lookups.** Use `func(string) state.PeerIdentityPayload` — consumers pick the fields they need.
- **Do NOT use `db.GetPeerName()` directly** outside the resolver's fallback path. All identity lookups go through `resolvePeer`.
- **Do NOT use `Peers.Snapshot()` for single-peer lookups.** Use `peers.Get(id)` — it's O(1) with a mutex, not O(n) copy.
- **Do NOT add identity fields to `SeenPeer` without also adding them to `PeerIdentityPayload`, `PresenceMsg`, and `PeerAnnouncePayload`.** All four structs must stay in sync.
- **Do NOT bypass the MQ bridge for browser updates.** PeerTable changes reach the browser via `PublishPeerAnnounce` → SSE. No polling needed.
