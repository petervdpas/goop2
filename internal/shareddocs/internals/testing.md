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

## BDD feature tests (godog)

All under `tests/` — each directory has a `.feature` file + `*_test.go` step definitions.

| Suite | File | Scenarios | What it covers |
| -- | -- | -- | -- |
| `tests/mq/` | `mq.feature` | 49 | Topic routing, SSE delivery/suppression, inbox buffering, payload fidelity, identity propagation, cross-subsystem isolation |
| `tests/orm/` | `orm.feature` | 49 | ORM data API through HTTP routes: schema CRUD, insert/find/find-one/get-by, exists/count, pluck/distinct, aggregate+GROUP BY, update/delete by id, update-where/delete-where, upsert, access policies, table DDL (rename), 11 validation error cases |
| `tests/groups/` | `subscription.feature` | — | Group subscription lifecycle |
| `tests/chatrooms/` | `chatroom.feature`, `chatroom_joiner.feature`, `clubhouse_wiring.feature` | — | Chat room host/joiner flows, clubhouse template wiring |
| `tests/grouptypes/` | `grouptypes.feature` | — | Group type registry |

The ORM suite tests through the real HTTP handler stack (`RegisterData` → `storage.DB` → SQLite in-memory), not mocks.

## Test coverage by package (2026-04-06 baseline)

Run: `go test ./... -coverprofile=coverage.out -covermode=atomic`

| Package | Coverage | Tests | What they cover | Gaps |
| -- | -- | -- | -- | -- |
| `app/shared` | 100% | 5 | NormalizeLocalViewer | - |
| `ui/viewmodels` | 100% | 5 | BuildPeerRow, BuildPeerRows | - |
| `avatar` | 96.2% | 21 | Store CRUD + hash, extractInitials, deterministicColor, InitialsSVG, Cache | - |
| `config` | 94.9% | 49 | Default(), Validate() all sections, validateWANRendezvous, stripBOM, Load/Save/Ensure | - |
| `orm/transformation` | 90.2% | — | — | - |
| `orm/schema` | 86.6% | — | — | - |
| `state` | 83.2% | 5+ | FromSeenPeer, PeerIdentityPayload, PeerTable Upsert/Prune | - |
| `testpeer` | 82.8% | 10 | Bus routing, topic prefix, identity MQ, presence, group lifecycle | - |
| `bridge` | 82.5% | 6 | New, Register, connectOnce, Connect reconnect | - |
| `orm` | 82.3% | 10+ | Schema validation, query building, merge, roles | - |
| `template` | 82.1% | — | Template handler lifecycle | - |
| `sitetemplates` | 81.1% | — | Manifest parsing, embed | - |
| `crypto` | 77.3% | — | NaCl encrypt/decrypt | - |
| `content` | 74.9% | 34 | Store CRUD, etag, path traversal, tree listing | - |
| `files` | 74.1% | 4+ | Store operations, handler lifecycle | File transfer |
| `mq` | 73.8% | 9 | Topic match, subscribe, inbox replay, PublishLocal | - |
| `directchat` | 72.4% | — | Manager, message delivery | - |
| `lua` | 63.8% | 10+ | ORM API, blog, groups, templates | - |
| `gql` | 64.2% | — | GraphQL engine | - |
| `cluster` | 56.9% | 5+ | Job dispatch, worker assignment | Multi-peer compute |
| `datafed` | 53.1% | 8+ | Contributions, sync, peer sources | Cross-peer federation |
| `viewer` | 48.1% | 18 | contentType, LogBuffer, noCache, proxyPeerSite | Remote peer proxy |
| `chat` | 44.9% | 6+ | Room create/close, message delivery | Joiner-side, events |
| `util` | 37.1% | — | DNS cache, ring buffer, helpers | Pure functions untested |
| `rendezvous` | 30.0% | 25+ | Server API, WS races, relay, credits, client WS state machine | Server handlers, punch hints |
| `ui/render` | 29.6% | 10 | Highlight, InitTemplates, RenderStandalone | Template funcs via execution |
| `group` | 25.3% | 15 | Host create/close/join, remote join/leave/kick | Client-side, routing |
| `call` | 23.3% | — | Session basics | SDP/ICE, track mgmt |
| `storage` | 17.0% | — | SQLite basics | Table CRUD, meta, ORM schema |
| `viewer/routes` | 11.1% | 80 | Helpers, home, data API, site API, export | Peer, settings, editor, groups |
| `app` | 10.8% | 4 | WaitTCP, setupMicroService | runPeer logic branches |
| `p2p` | 6.4% | 3 | Libp2p options, WSS dial | Data handlers, topology |
| `listen` | 2.9% | 4+ | Queue basics | Events, room state |
| `app/modes` | 0% | 0 | — | Pure orchestration |

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
