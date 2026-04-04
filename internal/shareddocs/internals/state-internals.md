# State Internals

## PeerTable

`internal/state/peers.go`

Thread-safe in-memory peer state with event subscriptions. This is the single source of truth for which peers are online and their metadata.

### SeenPeer struct

```go
type SeenPeer struct {
    Content, Email, AvatarHash     string
    VideoDisabled                  bool
    ActiveTemplate                 string
    PublicKey                      string
    EncryptionSupported            bool
    Verified                       bool
    GoopClientVersion              string
    Reachable                      bool
    LastSeen                       time.Time
    OfflineSince                   time.Time
    Favorite                       bool
    failStreak                     int       // unexported
    lastFailAt                     time.Time // unexported
}
```

### Operations

| Method | Purpose |
| -- | -- |
| `Upsert(id, content, email, ...)` | Add or update peer. Preserves Reachable, Favorite, failStreak, PublicKey, EncryptionSupported, GoopClientVersion across updates if the incoming value is empty. |
| `Seed(id, content, ...)` | Insert only if peer doesn't exist (for DB cache restore on startup) |
| `Touch(id)` | Update LastSeen timestamp |
| `Get(id)` | Return single peer |
| `IDs()` | Return all peer IDs |
| `Snapshot()` | Return copy of entire map |
| `Remove(id)` | Delete peer, emit "remove" event |
| `MarkOffline(id)` | Set Reachable=false, set OfflineSince, reset failure tracking |
| `SetReachable(id, bool)` | Success: reset failStreak, mark reachable. Failure: increment failStreak, only mark unreachable after 2 distinct failures. |
| `SetFavorite(id, bool)` | Toggle favorite status |
| `SetPublicKey(id, key)` | Update NaCl public key |
| `PruneStale(ttlCutoff, graceCutoff)` | Online peers past TTL → offline. Offline peers past grace → removed. |

### Failure dedup

A peer is only marked unreachable after `failStreak >= 2` distinct failure events that are more than 4 seconds apart (`PeerFailureDedupWindow`). This prevents a single transient probe timeout from flashing the UI.

### Event system

```go
type PeerEvent struct {
    Type   string              // "update" or "remove"
    PeerID string
    Peer   *SeenPeer           // set on "update"
    Peers  map[string]SeenPeer // unused (for bulk events)
}
```

- `Subscribe()` returns a buffered channel (cap: 16) that receives PeerEvents
- `Unsubscribe(ch)` removes and closes the channel
- Events emitted by: Upsert, Seed, Remove, MarkOffline, SetReachable, SetFavorite, PruneStale

### Integration

The PeerTable is subscribed to by `app/modes/peer.go` which:

1. Listens on the event channel
2. On "update": publishes `PeerAnnouncePayload` to local MQ (→ browser SSE)
3. On "remove": publishes `PeerGonePayload` to local MQ, deletes from `_peer_cache`

Peers enter the table from:

- P2P GossipSub presence messages (via `node.go` gossip handler)
- Rendezvous server WebSocket/SSE events (via `peer.go` presence handler)
- DB cache restore on startup (`Seed`)
