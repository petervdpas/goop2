# Chat Internals

## Two chat systems

Goop2 has two separate chat systems that share the `chat` word but are architecturally distinct:

| System | Package | Transport | Persistence | Purpose |
| -- | -- | -- | -- | -- |
| Direct/broadcast chat | `internal/directchat/` | MQ topics `chat` and `chat.broadcast` | `_chat_messages` table | 1:1 messages and broadcast to all peers |
| Group chat rooms | `internal/group_types/chat/` | MQ topics `chat.room:{groupID}:{sub}` | In-memory ring buffer per room | Bounded group chat within a hosted group |

## Direct chat (`internal/directchat/`)

### Manager

`directchat.Manager` owns direct message persistence and Lua command dispatch.

- `selfID`: local peer ID
- `store`: `Store` interface (backed by `_chat_messages` table)
- `mq`: local `MQ` interface (Send + SubscribeTopic — a subset of `mq.Transport`, defined in `store.go`)
- `lua`: optional `LuaDispatcher` for `!` command handling

### Message flow

**Inbound** (remote peer → local):

1. MQ delivers message on topic `chat`
2. `handleDirect` extracts content from payload
3. Persists to `_chat_messages` (from=remote, peer_id=remote)
4. If content starts with `!`, dispatches to Lua engine as a command

**Outbound** (local → remote peer):

1. Viewer UI calls MQ Send with topic `chat`
2. `PersistOutbound` stores message (from=self, peer_id=remote)

**Broadcast** (topic `chat.broadcast`):

- Fire-and-forget, not persisted
- Published via MQ to all peers

### Lua commands

Messages starting with `!` trigger Lua dispatch:

1. `LuaDispatcher.DispatchCommand(ctx, fromPeerID, content, sendFn)`
2. Lua can respond by calling the sendFn callback
3. Response is sent back via MQ topic `chat`

## Group chat rooms (`internal/group_types/chat/`)

### Architecture

Group chat rooms are a TypeHandler implementation on top of the group protocol. The chat manager creates a hosted group per room, and uses MQ for message delivery.

### Key types

- `Manager` — owns rooms map, group manager reference, MQ reference
- `roomState` — per-room: `info` (Room metadata) + `history` (RingBuffer of messages)
- `Room` — ID, Name, Description, Members
- `Message` — ID, From, FromName, Text, Timestamp

### Room lifecycle

1. **Create**: `CreateRoom(name, desc, context, max)` → `grp.CreateGroup()` + `grp.JoinOwnGroup()` → `OnCreate` callback creates `roomState`
2. **Join**: Joiner calls `JoinRoom(ctx, hostPeerID, groupID)` → `grp.JoinRemoteGroup()` (group protocol join) → creates local `roomState` with name from subscription
3. **Send**: `SendMessage(groupID, fromPeerID, text)` → stores in ring buffer → `broadcastToRoom`
4. **Close**: `CloseRoom(groupID)` → `grp.CloseGroup()` → `OnClose` removes from rooms map
5. **Context close**: `CloseByContext(context)` — closes all rooms matching a template context (on template switch)

### Host vs joiner asymmetry

The TypeHandler callbacks (`OnCreate`, `OnJoin`, `OnLeave`, `OnClose`) only fire on the **host** side. The joiner's `rooms` entry is created by `JoinRoom` after the group join succeeds, using the group name from the stored subscription.

`resolveMembers(groupID)` checks `HostedGroupMembers` first (host path), then falls back to `ClientGroupMembers` (joiner path).

`broadcastToRoom(groupID, sub, msg, excludePeer)` uses the same fallback for discovering recipients.

### MQ topics

| Topic | Subtopic | Purpose |
| -- | -- | -- |
| `chat.room:{groupID}:msg` | `msg` | Chat message delivery |
| `chat.room:{groupID}:history` | `history` | Message history sent to new joiners |
| `chat.room:{groupID}:members` | `members` | Updated member list broadcast |

### Message handling

`handleIncoming` processes inbound `chat.room:*` messages:

- `msg`: stores in room history ring buffer, rebroadcasts to other members
- `history` and `members`: delivered to browser via local MQ publish (SSE)
