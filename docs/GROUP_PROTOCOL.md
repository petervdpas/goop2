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

## Frontend / UI Design

The protocol sections above cover the backend plumbing. This section describes where groups surface in the viewer UI.

### Navigation Changes

The current top-level navigation is:

```
Peers | Me | Create | Database | Logs
```

Two changes:

1. **Create tab gets sub-tabs**: `[Editor] [Templates] [Groups]` -- group creation lives here alongside site editing and template selection, since groups are part of what a host "publishes."
2. **New top-level Groups tab**: shows all groups the peer is subscribed to.

Updated navigation:

```
Peers | Me | Create | Groups | Database | Logs
                       ▲
                       new
```

Inside the Create tab:

```
[Editor]  [Templates]  [Groups]
```

### Create > Groups (Host-side)

This is where a host creates and manages the groups they offer. The UI provides:

- **Create Group** form: name, app type (chess, quiz, chat, ...), max members
- **My Hosted Groups** list: all groups in the local `_groups` table, with member count and status
- Actions per group: edit settings, close group, delete group

When a host creates a group they are automatically subscribed to it as the group admin (see below).

Templates may also auto-create groups. For example, applying the chess template could insert a default group into `_groups`. The Create > Groups sub-tab shows these alongside manually created groups.

### Groups Tab (Top-level, Subscriber-side)

This is the peer's single view of **all groups they are subscribed to**, regardless of which host owns them. The list is divided into two blocks:

#### My Groups (Admin)

Groups hosted by this peer. These stand out visually (different background, admin badge, or separate block at the top). The host is always subscribed to their own groups so they appear here automatically.

For each group:
- Group name, app type, member count
- **Admin** badge
- Click to open the group's app UI

#### Joined Groups

Groups on other peers that this peer has previously joined. **Only groups whose host is currently online are shown** -- if the host is offline, the group is unreachable (host-relayed model) and is hidden or shown greyed-out with an "offline" indicator.

For each group:
- Group name, app type, host peer name
- Online/offline status of the host
- Click to open (rejoins via the group protocol)

### Subscriptions

Because there is no persistent server-side subscription (groups only exist while the host is online), subscriptions are tracked **locally** on the subscriber's side:

```sql
-- Local table, NOT synced to any host
CREATE TABLE _group_subscriptions (
    host_peer_id  TEXT NOT NULL,
    group_id      TEXT NOT NULL,
    group_name    TEXT,
    app_type      TEXT,
    role          TEXT DEFAULT 'member',  -- 'admin' for own groups
    subscribed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (host_peer_id, group_id)
);
```

- When a peer joins a group, a subscription record is created locally.
- When a peer leaves a group, the record is removed.
- When the host creates a group, a subscription with `role = 'admin'` is created for themselves.
- The Groups tab queries this table and cross-references with the online peer list to determine which groups are reachable.

### Discovery

How does a peer find groups to join?

1. **Via the host's site UI** -- the host's site pages (served via `/goop/site/1.0.0`) can list available groups and offer join buttons. This is the primary discovery path.
2. **Via direct chat** -- a host shares a group link in a 1:1 chat message.
3. **Via presence metadata** -- groups could optionally be announced in GossipSub presence, letting peers browse available groups from the Peers tab.

Once subscribed, the group appears in the top-level Groups tab for easy access.

### Data Flow: UI Perspective

```
Host creates group:
  Create > Groups > "New Group" form
  -> POST /api/groups (creates row in _groups)
  -> Auto-subscribes host as admin (row in _group_subscriptions with role=admin)
  -> Group appears in Groups tab under "My Groups (Admin)"

Visitor discovers group:
  Visits host's site, sees "Join Chess Game" button
  -> JS calls Goop.group.join("chess-42")
  -> Local viewer opens /goop/group/1.0.0 stream to host
  -> On successful welcome, local subscription record created
  -> Group appears in visitor's Groups tab under "Joined Groups"

Visitor opens Groups tab later:
  -> Tab loads _group_subscriptions
  -> Cross-references with online peer list
  -> Shows reachable groups (host online), hides or greys out unreachable ones
  -> Click on a reachable group reopens the stream and loads the app UI
```

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
3. **Permissions** -- should the host be able to define roles (player vs spectator vs admin)? The `welcome` message could include a role field. The subscription table already tracks role locally.
4. **WebSocket vs SSE** -- the browser needs a persistent connection to the local viewer. WebSocket is bidirectional (natural fit), SSE is simpler but requires a separate POST endpoint for sending.
5. **Offline group display** -- should groups from offline hosts be shown greyed-out (so the user remembers them) or hidden entirely? Greyed-out is more informative but adds visual noise.

---

*See also: [TEMPLATES_AND_GROUPS.md](TEMPLATES_AND_GROUPS.md) for the application-level view of groups and templates.*
