# P2P Networking

## Protocols

All protocol IDs are defined in `internal/proto/proto.go`.

### Two categories

**MQ (messaging)** — all peer-to-peer messaging goes through a single protocol:

| Protocol ID | Purpose |
| -- | -- |
| `/goop/mq/1.0.0` | THE message bus — chat, groups, calls, identity, all messaging |

Everything that is a message (text, signals, state updates, identity requests) goes over MQ. See `shareddocs/internals/mq-internals.md` for topics.

**Stream (binary/large data)** — separate protocols for bulk transfers:

| Protocol ID | Purpose |
| -- | -- |
| `/goop/content/1.0.0` | Reachability probe — single line, unencrypted, NOT for identity |
| `/goop/site/1.0.0` | Fetch files from a peer's site folder |
| `/goop/data/1.0.0` | Remote ORM queries/responses (request-response, large payloads) |
| `/goop/avatar/1.0.0` | Peer avatar binary fetch |
| `/goop/docs/1.0.0` | Shared document listing and file transfer |
| `/goop/listen/1.0.0` | Audio streaming (continuous binary) |

Stream protocols exist because their payloads are binary or too large for the MQ JSON transport. If it's a message, it goes over MQ. If it's a file or stream, it gets its own protocol.

## Presence

Peers announce themselves via GossipSub on topic `goop.presence.v1`. The `PresenceMsg` struct carries:

- `type`: `online`, `update`, `offline`, `punch`
- `peerId`, `content`, `email`, `avatarHash`, `videoDisabled`, `activeTemplate`
- `addrs`: multiaddresses for WAN connectivity
- `publicKey`, `encryptionSupported`: NaCl E2E encryption
- `verificationToken`: set by client, validated by rendezvous server
- `verified`: set by rendezvous server after email verification
- `goopClientVersion`: build version of the sending peer
- `target`: punch hint (peer ID this message is addressed to)

## Data proxy

Remote ORM operations route through the `/goop/data/1.0.0` stream protocol (`internal/p2p/data.go`).

### Wire format

Request: single newline-delimited JSON line (`DataRequest`). Response: single newline-delimited JSON line (`DataResponse`).

When encryption is enabled, lines are prefixed with `ENC:` followed by base64-encoded NaCl ciphertext.

### DataRequest fields

`Op`, `Table`, `Name`, `Data`, `ID`, `Where`, `Args`, `Columns`, `ColumnDefs`, `Column`, `Limit`, `Offset`, `OldName`, `NewName`, `Function`, `Params`, `Order`, `Fields`, `Expr`, `GroupBy`, `KeyCol`

### Operations

| Op | Remote | Local-only | Notes |
| -- | -- | -- | -- |
| `tables` | yes | yes | List all tables |
| `orm-schema` | yes | yes | List all ORM schemas |
| `describe` | yes | yes | Table column info |
| `query` | yes | yes | Remote: access-checked; local: unrestricted |
| `insert` | yes | yes | Access-checked |
| `update` | yes | yes | Remote: access-checked; local: unrestricted |
| `delete` | yes | yes | Remote: access-checked; local: unrestricted |
| `query-one` | no | yes | Local only |
| `exists` | no | yes | Local only |
| `count` | no | yes | Local only |
| `pluck` | no | yes | Local only |
| `distinct` | no | yes | Local only |
| `aggregate` | no | yes | Local only |
| `update-where` | no | yes | Local only |
| `delete-where` | no | yes | Local only |
| `upsert` | no | yes | Local only |
| `role` | yes | yes | Returns caller's role and permissions for a table |
| `lua-call` | yes | yes | Invoke a Lua data function |
| `lua-list` | yes | yes | List available Lua data functions |
| `create-table`, `add-column`, `drop-column`, `rename-table`, `delete-table` | no | yes | Schema ops: local only |

### Access enforcement

Access is checked in `data.go`, NOT in the storage layer. The `checkGroupAccess` function:

1. Looks up the caller's role via `groupChecker.TemplateMemberRole(callerID)`
2. Loads the table's ORM schema (`db.GetSchema(table)`)
3. Checks `schema.RoleCanDo(roles, role, op)` against the schema's Roles map
4. Owner (callerID == selfID) always has full access

Access levels per table operation: `local`, `owner`, `group`, `open` — defined in the schema JSON.

## Peer discovery

- **LAN**: mDNS with tag `goop-mdns` — starts immediately in `p2p.New()` (not deferred)
- **WAN**: Rendezvous server — peers publish presence via HTTP POST or WebSocket; server relays to all connected peers

## NAT traversal

- **Circuit relay v2**: Auto-relay with static relay peer (from rendezvous server config)
- **DCUtR hole-punching**: Enabled when relay is available
- **ForceReachabilityPrivate**: All peers assume they're behind NAT
- `SetReachable(true)` is called only on first successful discovery, not on every heartbeat
- Failure dedup: peer is only marked unreachable after 2 distinct failure events >4s apart
