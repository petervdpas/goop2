# Peer Identity

## The single rule

**Identity travels over the WebSocket presence channel. Everything else resolves from the PeerTable.**

There is exactly ONE path for identity data:

```
Peer A → WebSocket → Rendezvous Server → WebSocket → Peer B → PeerTable.Upsert()
```

On LAN, the same data also flows via GossipSub:

```
Peer A → GossipSub (goop.presence.v1) → Peer B → PeerTable.Upsert()
```

Both paths deliver `PresenceMsg` which carries the full identity: Name (Content), Email, AvatarHash, GoopClientVersion, PublicKey, EncryptionSupported, ActiveTemplate, VideoDisabled, Verified.

## Communication layers

Goop2 has multiple communication layers. Understanding which carries what is critical:

| Layer | Transport | Carries | Identity? |
| -- | -- | -- | -- |
| Presence (WAN) | WebSocket to rendezvous server | `PresenceMsg` — full peer identity | **YES — this is THE source** |
| Presence (LAN) | GossipSub `goop.presence.v1` | `PresenceMsg` — same struct | **YES — same source, different transport** |
| MQ | libp2p stream `/goop/mq/1.0.0` | All peer-to-peer messages (chat, groups, calls) | No — `from` is a raw peer ID from `stream.Conn().RemotePeer()` |
| Content probe | libp2p stream `/goop/content/1.0.0` | Single line: peer's display name | Name only — used for reachability probes, NOT for identity |
| Data | libp2p stream `/goop/data/1.0.0` | ORM queries/responses | No |
| Site | libp2p stream `/goop/site/1.0.0` | File content | No |
| HTTP | Viewer HTTP server | Browser ↔ Go | No — browser gets identity via MQ SSE `peer:announce` events |

## PeerTable is THE identity cache

`internal/state/peers.go` — `PeerTable` is the single in-memory cache for all peer identity data.

**Writers** (things that put identity INTO the PeerTable):

- Rendezvous WebSocket `onMsg` callback → `peers.Upsert()`
- GossipSub presence handler → `peers.Upsert()`
- DB cache seed on startup → `peers.Seed()`

**Readers** (things that get identity FROM the PeerTable):

- `resolvePeer()` — the canonical resolver in `peer.go`
- MQ presence bridge → `PublishPeerAnnounce()` → browser SSE
- Probe/heartbeat loop → checks `peers.Get()` for reachability

## The canonical resolver

`internal/app/modes/peer.go` defines ONE `resolvePeer` function. It is created once and passed to every subsystem:

```
resolvePeer(peerID) → PeerIdentity
  1. Self?       → selfContent() + selfEmail()     [instant]
  2. PeerTable?  → FromSeenPeer(sp)                 [instant, in-memory]
  3. DB cache?   → from SQLite _peer_cache table     [instant, local disk]
  4. Unknown     → returns empty PeerIdentity{}
```

The resolver returns `state.PeerIdentity` — a struct with Name, Email, AvatarHash, Reachable, Verified, GoopClientVersion, PublicKey, EncryptionSupported, ActiveTemplate, VideoDisabled, Known.

### Consumers

Every subsystem receives the same `resolvePeer` instance:

| Consumer | Uses |
| -- | -- |
| `group.Manager` | Member name resolution in `resolveMemberNames()` |
| `chatType.Manager` | Message `FromName`, member list names |
| `routes.RegisterGroups` | Member names, host reachability in subscription list |
| `routes.RegisterListen` | Listener names |
| `routes.RegisterChatRooms` | Passed for consistency (chat manager resolves internally) |
| `routes.Deps` | Used by docs route for peer file labels |
| `viewer.Viewer` | Holds reference, passes to all route registrations |

### On-demand identity via MQ

When a peer sends an MQ message but isn't in the PeerTable yet (timing race — MQ connection before WebSocket presence arrives), the resolver:

1. Returns `PeerIdentity{Known: false}` immediately (non-blocking)
2. Fires a background MQ `Send` on topic `identity` to the unknown peer
3. The remote peer's `identity` subscriber responds with `identity.response` carrying `IdentityPayload`
4. The `identity.response` handler upserts the full identity into the PeerTable
5. Next lookup returns the full identity

This uses the MQ bus — the same transport as everything else. No separate stream protocol, no parallel identity channel.

### MQ topics

| Topic | Direction | Purpose |
| -- | -- | -- |
| `identity` | Requester → Unknown peer | "Who are you?" — fire-and-forget request |
| `identity.response` | Responder → Requester | Full `IdentityPayload` — name, email, avatar, version, etc. |

Both topics are **suppressed from SSE** — they're internal plumbing, not browser events.

The frontend handles the brief window with a `getPeerName()` fallback from the `peer:announce` MQ cache, and a last-resort truncated peer ID display.

## Frontend identity

The browser maintains its own peer identity cache in `mq/peers.js`:

```
peer:announce event (SSE) → _peerMeta[peerID] = payload
peer:gone event (SSE)     → delete _peerMeta[peerID]
```

Lookup: `Goop.mq.getPeerName(peerID)` → `_peerMeta[peerID].content`

This is the JS equivalent of `resolvePeer(id).Name`. Templates use it as a fallback when server-provided names are empty (timing race).

## What NOT to do

- **Do NOT create new libp2p stream protocols for identity.** Identity comes from presence (WebSocket/gossipsub) with MQ fallback. All P2P communication goes over MQ.
- **Do NOT resolve names differently in different places.** Use `resolvePeer`. One function, one instance.
- **Do NOT pass `func(string) string` for name-only lookups.** Use `func(string) state.PeerIdentity` — consumers pick the fields they need.
- **Do NOT use `db.GetPeerName()` directly** outside the resolver's fallback path. All identity lookups go through `resolvePeer`.
- **Do NOT use `Peers.Snapshot()` for single-peer lookups.** Use `peers.Get(id)` — it's O(1) with a mutex, not O(n) copy.
