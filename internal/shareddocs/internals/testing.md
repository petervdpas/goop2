# Test Infrastructure and Peer Emulation

## mq.Transport interface

`internal/mq/transport.go` — the abstraction that decouples managers from libp2p:

```
Transport interface {
    Send(ctx, peerID, topic, payload) (string, error)
    SubscribeTopic(prefix, fn) func()      // returns unsubscribe
    PublishLocal(topic, from, payload)      // local-only SSE delivery
}
```

- `*mq.Manager` satisfies Transport (production, libp2p streams)
- `testpeer.MQAdapter` satisfies Transport (testing, in-process bus)
- `mq.NopTransport` satisfies Transport (unit tests that don't need messaging)

### Naming convention

| Context | Field/param name | Type | Reason |
| -- | -- | -- | -- |
| Manager fields (group, chat, listen, cluster, files, datafed, signaler) | `mq` | `mq.Transport` | Short field name, type tells the story |
| Constructor params | `transport` | `mq.Transport` | Declares what it is |
| Viewer routes (need `Subscribe()`, `NotifyDelivered()` etc.) | `mqMgr` | `*mq.Manager` | Concrete manager, not interface |
| peer.go (wiring code) | `mqMgr` | `*mq.Manager` | Creates the real manager |

## testpeer package

`internal/testpeer/` — complete peer emulation for multi-peer tests without libp2p.

### Files

| File | Type | Purpose |
| -- | -- | -- |
| `bus.go` | `TestBus` | In-process message router. Routes messages between adapters, records message log |
| `adapter.go` | `MQAdapter` | Implements `mq.Transport`. Each peer gets one. Routes Send() through the bus, dispatches to topic subscribers |
| `peer.go` | `TestPeer` | Complete peer: PeerTable + DB + MQ + GroupManager + resolvePeer + identity handlers |

### TestBus

```go
bus := testpeer.NewTestBus()

// Inspection:
bus.Messages(filter)           // all messages matching predicate
bus.MessagesForTopic("group:") // topic prefix filter
bus.MessageCount()             // total routed messages
```

Messages are delivered **synchronously** — when peer A calls `Send(peerB, topic, payload)`, peer B's topic subscribers execute before Send returns. This makes tests deterministic.

### TestPeer

```go
alice := testpeer.NewTestPeer(t, bus, testpeer.PeerConfig{Content: "Alice"})
// Fields: ID, Content, Email, Bus, MQ, Peers, DB, Groups, ResolvePeer
// Cleanup via t.Cleanup — no manual Close needed
```

What it wires up:
1. `storage.Open(t.TempDir())` → SQLite in temp dir
2. `state.NewPeerTable()` → in-memory peer cache
3. `NewMQAdapter(bus, id)` → implements mq.Transport, registered on bus
4. `group.NewTestManager(db, id, opts)` → with MQ adapter and resolvePeer
5. Identity MQ handlers (`identity` / `identity.response`) → mirrors production peer.go
6. `resolvePeer` closure → self → PeerTable → MQ fallback (same chain as production)

### TestPeer methods

- `AnnounceOnline()` — broadcasts `peer:announce` to all bus peers
- `AnnounceOffline()` — broadcasts `peer:gone` to all bus peers

### Existing test helpers (still valid)

| Package | Helper | What it provides |
| -- | -- | -- |
| `group` | `NewTestManager(db, selfID, opts?)` | Manager with optional MQ + resolvePeer via `TestManagerOpts` |
| `group` | `SimulateInvite/Join/Leave/HostClose` | Inject events without real transport |
| `group` | `SetActiveConn/SetActiveConnMembers` | Fake client connections |
| `group_types/chat` | `NewTestManager(grpMgr, selfID, resolvePeer)` | Chat room manager with NopTransport |
| `group_types/chat` | `RegisterJoinedRoom(groupID, name)` | Fake room entry for joiner tests |
| `group_types/datafed` | `NewTestManager(grpMgr, selfID, schemas)` | DataFed manager without MQ |
| `group_types/datafed` | `AddTestContribution(groupID, peerID, tableName)` | Inject fake federation data |
| `group_types/listen` | `NewTestManager(store)` | Listen manager with StateStore, no host/MQ |
| `group_types/listen` | `SetTestGroup(id)` | Set active listen group for queue tests |
| `group_types/listen` | `SetTestQueue(paths, idx)` | Set playlist queue and current index |

Both `group.NewTestManager` and `chat.NewTestManager` default to `mq.NopTransport{}` when no MQ is provided, preventing nil interface panics.

## Test coverage by package

| Package | Tests | What they cover | Gaps |
| -- | -- | -- | -- |
| `testpeer` | 10 | Bus routing, topic prefix, identity MQ fallback, presence broadcast, group lifecycle, 3-peer chain | Full MQ join (not just SimulateJoin) |
| `group` | 15 | Host create/close/join, remote join/leave/kick, subscriptions, volatile wipe, invite | Client-side JoinRemoteGroup (needs libp2p for Connect) |
| `group_types/chat` | 6+ | Room create/close, message delivery, history | Joiner-side message receipt |
| `group_types/cluster` | 5+ | Job dispatch, worker assignment | Multi-peer compute |
| `group_types/datafed` | 8+ | Contributions, sync, peer sources | Cross-peer federation |
| `group_types/files` | 4+ | Store operations, handler lifecycle | File transfer |
| `group_types/listen` | 4+ | Queue, events, room state | Audio streaming |
| `mq` | 9 | Topic prefix match, subscribe/unsubscribe, inbox replay, PublishLocal, delivered events | Send (needs libp2p) |
| `state` | 5 | FromSeenPeer, PeerIdentityPayload, Name() | PeerTable Upsert/Prune |
| `p2p` | 3 | Libp2p options, WSS dial, context flags | Node creation (slow, needs network) |
| `rendezvous` | 10+ | Server API, WebSocket, relay, credits, docs, templates | Client reconnect logic |
| `orm` | 10+ | Schema validation, query building, merge, roles | - |
| `lua` | 10+ | ORM API, blog, groups, templates | - |
| `viewer/routes` | BDD | Template store install/uninstall | Most routes untested |

## Code quality notes (for future cleanup)

### Things that still don't read well

- `internal/viewer/viewer.go`: the `Viewer` struct passed to `Start()` has ~20 fields. Could benefit from grouping into sub-structs but this is cosmetic

### Reviewed and accepted

- `group.New(host.Host, ...)` vs `NewTestManager(db, selfID, opts...)` — intentional split. Production needs host for `h.ID()` and `h.Connect()`. TestPeer never passes nil to `New`; it uses `NewTestManager` exclusively.
- `TestManagerOpts` migration — complete. All callers (unit tests, BDD tests, testpeer) use the opts pattern correctly.
- Route file sizes — groups.go (461), data.go (664), call.go (452). Moderate, not worth splitting.
- JSON decode pattern — only 4 occurrences of `json.NewDecoder(r.Body).Decode` in routes. Helpers (`writeJSON`, `http.Error`) already handle most response encoding. No abstraction needed.

### What NOT to change

- `internal/group/` core files (client.go, host.go, routing.go) read well — clear function names, consistent patterns
- `internal/mq/manager.go` is clean — the Send/handleIncoming flow is linear and well-commented
- `internal/state/peers.go` and `identity.go` are tight and correct
- `internal/proto/proto.go` is a clean wire type definition
