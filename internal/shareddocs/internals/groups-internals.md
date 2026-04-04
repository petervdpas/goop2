# Group System Internals

## Manager

`internal/group/manager.go`

In-memory state:

- `groups map[string]*hostedGroup` — groups this peer hosts
- `activeConns map[string]*clientConn` — outbound connections to remote group hosts
- `pendingJoins map[string]chan joinResult` — pending join handshakes
- `handlers map[string]TypeHandler` — registered type-specific lifecycle handlers

DB persistence: `_groups`, `_group_members`, `_group_subscriptions` tables.

MQ subscriptions on startup:

- `group:` prefix — group protocol messages
- `group.invite` — group invitation delivery
- `peer:announce` — triggers `reconnectSubscriptions` for previously joined groups

## TypeHandler interface

`internal/group/typehandler.go`

```
Flags() GroupTypeFlags
  HostCanJoin bool  — whether the host can join their own group as a member
  Volatile    bool  — ephemeral: no member persistence, excluded from group cap

OnCreate(groupID, name string, maxMembers int) error
OnJoin(groupID, peerID string, isHost bool)
OnLeave(groupID, peerID string, isHost bool)
OnClose(groupID string)
OnEvent(evt *Event)
```

All hooks are called on the HOST side only. Hooks are called outside the manager's locks so handlers can safely call read methods back on Manager.

Registered via `manager.RegisterType(groupType, handler)`. If no handler is registered for a type, `GroupTypeFlagsForGroup` returns defaults (HostCanJoin: true).

## Group type implementations

`internal/group_types/`:

| Type | Package | Handler | Volatile | HostCanJoin |
| -- | -- | -- | -- | -- |
| `chat` | `chat/` | Chat room lifecycle, message history, member broadcast | No | Yes |
| `template` | `template/` | Schema analysis, template apply/close, co-author access | No | Yes |
| `files` | `files/` | Shared file storage (50MB limit, sha256 hash) | No | Yes |
| `listen` | `listen/` | Audio streaming: host streams, members listen | No | Yes |
| `cluster` | `cluster/` | Job queue with priority, worker dispatch, progress tracking | Yes | No |
| `datafed` | `datafed/` | Data federation: schema offer/withdraw/sync, peer contributions | No | Yes |

## Message types

`internal/group/message.go`:

| Type | Direction | Purpose |
| -- | -- | -- |
| `join` | client → host | Request to join a group |
| `welcome` | host → client | Join accepted, includes group name, type, member list |
| `error` | host → client | Join rejected (e.g., group full) |
| `members` | host → all | Updated member list broadcast |
| `msg` | any → host | Application message (relayed by host) |
| `state` | any → host | State update (relayed by host) |
| `leave` | client → host | Member leaving |
| `close` | host → all | Group closed by host |
| `ping` | host → member | Keepalive probe |
| `pong` | member → host | Keepalive response |
| `meta` | host → all | Group metadata update (name, maxMembers, roles) |

## Message routing

`internal/group/routing.go`

Host-relayed model:

- All messages go through the host — no direct member-to-member communication
- `broadcastToGroup` sends to all members except the sender
- Group topic format: `group:{groupID}:{type}`

Host message flow (`handleHostMessage`):

1. `join` → add member, send `welcome`, broadcast `members`, call `OnJoin`
2. `leave` → remove member, broadcast `members`, call `OnLeave`
3. `ping` → respond with `pong`
4. `msg`/`state` → relay to all other members, notify local listeners

Client message flow (`handleMemberMessage`):

1. `welcome` → deliver to pending join channel
2. `error` → deliver error to pending join channel
3. `members` → update `clientConn.members`
4. `close` → remove subscription, call `OnClose`
5. `meta` → update subscription metadata

## Member management

Members stored in `hostedGroup.members map[string]*memberMeta`:

- `memberMeta`: peerID, role, joinedAt
- Default role on join comes from `hostedGroup.info.DefaultRole` (default: "viewer")
- Roles persisted in `_group_members` table
- `SetMemberRole`: updates in-memory + DB + broadcasts updated member list

## Client-side (joining remote groups)

`internal/group/client.go`

- `activeConns`: outbound connections keyed by groupID
- `clientConn`: holds hostPeerID, groupID, groupType, and last known members list
- `JoinRemoteGroup`: sends `TypeJoin`, waits for `TypeWelcome` (timeout: 10s), creates subscription in DB
- `reconnectSubscriptions`: runs on startup to rejoin previously connected groups
- `ClientGroupMembers(groupID)`: returns last known member list from `clientConn.members`
- Subscriptions persisted in `_group_subscriptions` for reconnection across restarts

## Groups are never auto-deleted

Only the owner removes groups:

- `CloseGroup` removes from memory + DB + broadcasts `TypeClose` to all members
- Template apply closes groups where `group_type == template AND group_context == old template name`
- No startup cleanup or purge logic exists
