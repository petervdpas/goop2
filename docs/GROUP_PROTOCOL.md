# Group Protocol Design

## The Gap

Goop2 currently has three communication patterns:

| Pattern | Protocol | Scope |
|---------|----------|-------|
| Broadcast | GossipSub (presence) | Everyone |
| 1:1 | `/goop/chat/1.0.0` | Two peers |
| Request/response | `/goop/data/1.0.0` | Two peers |

Groups are the missing middle -- N peers, scoped to a subset of the network. A chess game between two players shouldn't be broadcast to the entire community. A quiz with 30 students needs a private channel that isn't 1:1. A study group needs ongoing multi-peer communication.

---

## Options Considered

### A. Fan-out Streams

Sender opens a `/goop/group/1.0.0` stream to each member individually. Each message is sent N times.

- Simple to implement
- O(N) connections per message
- No infrastructure beyond what exists
- Doesn't scale, but groups are small (2-30 peers)
- No single point of failure beyond the sender

### B. Per-group Pubsub Topic

Each group subscribes to a GossipSub topic like `goop.group.<id>`. Messages are broadcast via the existing pubsub infrastructure.

- Efficient routing (GossipSub handles fan-out)
- Scales to larger groups
- But: pubsub routing may leak group membership to non-members (GossipSub peers see topic subscriptions)
- Topic management overhead (create/destroy per group)
- Less control over delivery guarantees

### C. Host-relayed (Recommended)

The group creator (host) acts as the hub. Each member opens a long-lived bidirectional stream to the host. The host relays messages between members.

- Natural fit: the host already serves the site and stores data
- Single source of truth for group state
- Host's database records game moves, quiz answers, chat history via the existing data protocol
- Low member counts (2-30) mean fan-out cost is negligible
- Private by design -- only the host knows the full member list
- Clean lifecycle: group exists as long as the host is online

---

## Recommended Architecture: Host-relayed Groups

### Why Host-relayed

The host peer is already the center of gravity in Goop2's model:

- The host **serves the site** (UI, templates) via `/goop/site/1.0.0`
- The host **stores the data** (game state, form responses) via `/goop/data/1.0.0`
- The host **knows the visitors** (peer table, presence)

Making the host the group relay keeps everything in one place. A chess game is:

1. Host serves the board UI (site protocol)
2. Host stores the moves (data protocol)
3. Host relays moves between players (group protocol)

No additional infrastructure. No pubsub topic management. No membership leaks.

### Protocol: `/goop/group/1.0.0`

A long-lived bidirectional stream between each member and the host. JSON messages flow in both directions, newline-delimited.

```
Member A  <──stream──>  Host  <──stream──>  Member B
                         │
                    (fan-out relay)
```

### Wire Format

Each message is a single JSON line:

```json
{"type":"join","group":"chess-42","payload":{}}
{"type":"msg","group":"chess-42","from":"<peerID>","payload":{"move":"e2e4"}}
{"type":"leave","group":"chess-42"}
```

**Fields:**

| Field | Description |
|-------|-------------|
| `type` | Message type: `join`, `leave`, `msg`, `state`, `error`, `members` |
| `group` | Group identifier (unique per host) |
| `from` | Sender's peer ID (set server-side for relayed messages) |
| `payload` | Arbitrary JSON payload (game moves, chat text, state sync) |

### Message Types

| Type | Direction | Purpose |
|------|-----------|---------|
| `join` | Member -> Host | Request to join a group |
| `welcome` | Host -> Member | Confirmation with current member list and state |
| `members` | Host -> Members | Updated member list (on join/leave) |
| `msg` | Bidirectional | Application message (chat, game move, etc.) |
| `state` | Host -> Member | Full state sync (reconnection, late join) |
| `leave` | Member -> Host | Member leaving |
| `close` | Host -> Members | Group is closed |
| `error` | Host -> Member | Error response |

### Lifecycle

```
1. Host creates a group (local operation, stored in DB)
2. Host shares group ID (via site UI, direct chat, or presence)
3. Member opens stream to host on /goop/group/1.0.0
4. Member sends: {"type":"join","group":"chess-42"}
5. Host validates, adds to member list
6. Host sends: {"type":"welcome","group":"chess-42","payload":{"members":[...],"state":{...}}}
7. Host broadcasts to other members: {"type":"members","group":"chess-42","payload":{"members":[...]}}
8. Messages flow bidirectionally:
   - Member sends msg -> Host relays to all other members (with "from" set server-side)
   - Host can inject system messages (timer, game events)
9. On leave: member sends leave, host broadcasts updated member list
10. On close: host sends close to all members, tears down streams
```

### Identity

Same principle as the data protocol: the `from` field on relayed messages is set by the host from `s.Conn().RemotePeer()`, not from the message payload. Cryptographically authenticated, unforgeable.

### Group Storage

Groups are stored in the host's SQLite database using the existing data protocol infrastructure:

```sql
-- Active groups on this host
CREATE TABLE _groups (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    app_type    TEXT,              -- 'chess', 'quiz', 'chat', etc.
    max_members INTEGER DEFAULT 0, -- 0 = unlimited
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

Membership is tracked in-memory (only online peers matter). Group messages can optionally be persisted using regular data tables (e.g. a `chess_moves` table written via the data protocol).

### Interaction with Existing Protocols

| Task | Protocol |
|------|----------|
| Serve game UI | `/goop/site/1.0.0` |
| Store game state, scores, history | `/goop/data/1.0.0` |
| Real-time moves, chat, sync | `/goop/group/1.0.0` |
| Discover available groups | Site UI + `/goop/data/1.0.0` (query `_groups` table) |

Templates use all three protocols transparently. A chess template's `app.js` would:

1. `Goop.data.query("_groups")` -- list open games
2. `Goop.group.join("chess-42")` -- join via group protocol
3. `Goop.group.send({move: "e2e4"})` -- send moves in real-time
4. `Goop.data.query("chess_moves")` -- review move history

### JavaScript API (goop-group.js)

```js
Goop.group = {
    // Join a group on the current (or remote) host
    join(groupId, onMessage) { ... },

    // Send a message to the group
    send(payload) { ... },

    // Leave the group
    leave() { ... },

    // List available groups (uses data protocol)
    list() { return Goop.data.query("_groups"); },
};
```

The JS client would use a WebSocket or SSE connection to the local viewer, which maintains the libp2p stream to the host. This keeps the browser -> local server -> P2P layering consistent with the data protocol.

---

## Data Flow Example: Chess Game

```
PeerA (host) creates chess game, shares link
PeerB visits /p/<peerA>/chess.html
  -> Site fetched via /goop/site/1.0.0
  -> JS calls Goop.group.join("chess-42")
  -> Browser opens WebSocket to local viewer
  -> Local viewer opens /goop/group/1.0.0 stream to PeerA
  -> PeerA adds PeerB to group, sends welcome with board state

PeerB makes a move:
  -> JS calls Goop.group.send({move: "e2e4"})
  -> WebSocket -> local viewer -> libp2p stream -> PeerA
  -> PeerA validates move, stores in DB via data protocol
  -> PeerA relays to all other group members
  -> PeerA sends updated board state

PeerC (spectator) joins:
  -> Same flow, receives full state sync on welcome
  -> Sees moves in real-time, read-only
```

---

## Open Questions

1. **Reconnection** -- if a member's stream drops, should they auto-rejoin? The host could hold state for a grace period.
2. **Persistence** -- should group chat history be stored? Game state likely yes (via data protocol), chat maybe optional.
3. **Permissions** -- should the host be able to define roles (player vs spectator)? The `welcome` message could include a role field.
4. **Discovery** -- how do peers find groups? Currently via the site UI. Could also be announced via presence metadata.
5. **WebSocket vs SSE** -- the browser needs a persistent connection to the local viewer. WebSocket is bidirectional (natural fit), SSE is simpler but requires a separate POST endpoint for sending.

---

*See also: [TEMPLATES_AND_GROUPS.md](TEMPLATES_AND_GROUPS.md) for the application-level view of groups and templates.*
