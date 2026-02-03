# Goop² Architecture

## The Manifesto

### The Web Was Supposed to Be Ours

The original web was a network of equals. Every participant could be both reader and publisher, client and server. There were no gatekeepers, no platforms, no terms of service. Your data lived on your machine, and the network existed because people chose to be part of it.

That web is gone. Today, your data lives on someone else's server. Your identity is owned by a corporation. Your content persists because a company pays the hosting bill — and disappears when they decide it should. The web became a handful of platforms masquerading as the open internet.

Goop² is a return to the original vision — rebuilt with modern tools.

### What Goop² Is

Goop² is a peer-to-peer desktop application that creates a decentralized web — a network where every participant is both client and server. Each peer runs a local application with its own database, its own web UI, and its own cryptographic identity.

We call it the **ephemeral web**: a web that exists only as long as its peers are alive, owned by no one centrally.

Every peer is a platform unto itself:

- **A web server** — serving its own ephemeral UI, accessible by other peers
- **A database** — local SQLite, queryable, editable, owned entirely by the user
- **An identity** — cryptographic key pair + verified email, self-sovereign
- **A node in the mesh** — discovering, communicating, and collaborating with other peers

This is not an app. It is infrastructure for a new kind of web.

### Communities, Not Platforms

In today's web, communities exist at the mercy of platforms. A Discord server, a Subreddit, a Facebook Group — none of them belong to the people in them. The platform owns the space, the data, and the rules. It can shut down, change terms, or sell your attention to advertisers at any time.

In Goop², **a community is a rendezvous server**. That is the fundamental equation.

A community hub is a lightweight server that handles peer discovery — helping peers find each other without sharing addresses manually. It provides NAT traversal, relay services, and a persistent meeting point. But it does not own the data. It does not host the conversations. It does not control the peers.

The hub is the town square. The peers are the people. When the square closes, the people still exist — and they can meet somewhere else.

#### Community Hubs

Each community hub is independent and sovereign:

- **Membership**: open, invite-only, or application-based
- **Economics**: free, paid, or donation-based
- **Governance**: the hub operator sets and enforces the rules
- **Scope**: peer discovery and broadcast chat are per-community
- **Identity**: peers verify their email to join — real identity, not throwaway accounts

A peer can belong to multiple communities simultaneously. Different communities for different contexts — just like real life.

#### Super Hubs: The Directory Layer

Above community hubs sit **super hubs** — directories that answer the question: *what communities exist?*

A super hub maintains a registry of community hubs: their names, descriptions, addresses, and membership types. It lets peers browse and search for communities to join. It does not relay traffic or know about individual peers. It is lightweight, cheap to run, and can federate with other super hubs.

Super hubs are **DNS for communities**:

```
Super Hub (directory)
  ├── Community Hub: "Go Developers"
  │     ├── peer-A
  │     ├── peer-B
  │     └── peer-C
  ├── Community Hub: "Music Production"
  │     ├── peer-D
  │     └── peer-A  (same peer, multiple communities)
  └── Community Hub: "Private Team X"  (invite-only, unlisted)
        ├── peer-E
        └── peer-F
```

Multiple super hubs can exist and sync their directories with each other. No single point of control. No single point of failure. Decentralized at every level.

### The Economics of Freedom

Goop² is free software built on a simple economic principle: **you only pay when you use infrastructure that costs money to run.**

**Free: Direct Connections** — Two peers can always connect directly if they know each other's addresses. No infrastructure required, no cost to anyone. Full functionality — chat, database, ephemeral web, everything. This is raw libp2p, peer-to-peer in its purest form.

**$9.99/Year: Community Hub Access** — For the price of a coffee, a peer gets automatic discovery within a community, NAT traversal and relay services, persistent availability across sessions, verified email identity, and access to community broadcast chat and peer directory. This is the tier where most users will live. It funds the infrastructure that makes communities work.

**Premium: Hub Hosting** — For organizations and community builders: managed community hub infrastructure, custom configuration, moderation tools, uptime guarantees.

**Super Hub Listings** — Community hub operators can pay to be listed and featured in the super hub directory — promoted placement, verified badges, visibility.

### A Web Inside the Web

| Traditional Web | Goop² Ephemeral Web |
|---|---|
| DNS root servers | Super hubs |
| Domain registrars | Community hubs |
| Web servers | Peers |
| Browsers | Goop² viewer |
| Databases | SQLite per peer |
| HTTP/HTTPS | libp2p protocols |
| REST APIs | `/goop/data/1.0.0` P2P data protocol |
| Domain names | Verified email + peer ID |
| Web hosting | Hub hosting (premium tier) |
| Search engines | Super hub directories |
| Social media platforms | Community hubs with broadcast |

But there is a fundamental difference.

In the traditional web, content lives on someone else's server and persists because a corporation pays the hosting bill. Your data is the product. Your identity is rented.

In the ephemeral web, data lives with the user. When a visitor interacts with a peer's site — filling out a form, posting a note, submitting a quiz answer — that data is stored in the **site owner's** database, with the visitor's cryptographic peer ID as the `_owner`. No tokens, no cookies, no OAuth. Identity is proven by the libp2p handshake itself. The network exists because people choose to participate — and it vanishes when they leave. Nothing persists without consent. Nothing is owned without agency.

**The web was supposed to be ours. Goop² makes it ours again.**

### Identity and Trust

- **Cryptographic identity**: Every peer has a libp2p key pair. This is the foundation — unforgeable, verifiable, permanent.
- **Email verification**: Required for hub access. Ties the cryptographic identity to something human-meaningful. Verified through a confirmation code flow during registration.
- **Community reputation**: Trust is built through behavior within a community. Peers that have been present longer, communicated reliably, and contributed meaningfully earn trust organically.
- **Blocklists and allowlists**: Every peer can curate who they interact with. Simple, effective, user-controlled.

No central identity provider. No "login with Google." Your identity is yours.

### What Can Be Built

Because every peer is a web server with a database, Goop² is a foundation for applications that don't exist yet:

- **Decentralized marketplaces** — peers list goods/services, transact directly
- **Collaborative wikis** — community knowledge bases, replicated across peers
- **Distributed social networks** — posts, follows, feeds — without a platform
- **Private team workspaces** — invite-only communities with shared data
- **IoT mesh networks** — devices as peers, communicating without cloud services
- **Offline-first applications** — everything works locally, syncs when connected

The SQLite-per-peer model means every user has a programmable data store. The P2P data protocol (`/goop/data/1.0.0`) means any peer can read from and write to another peer's database — with the caller's identity cryptographically stamped on every row. The ephemeral web UI means every user has a customizable interface. The libp2p mesh means every user is connected to every other user who chooses to be found.

### The Road Ahead

1. **Email verification** — Required identity for hub access
2. **Multi-hub support** — Peers join multiple communities simultaneously
3. **Community-scoped UI** — Peer lists and broadcast filtered by active community
4. **Subscription gating** — Hubs verify email and subscription before allowing discovery
5. **Super hub protocol** — Directory API for registering, searching, and listing hubs
6. **Browse Communities view** — UI for discovering and joining communities
7. **Hub federation** — Super hubs sync directories with each other
8. **Application layer** — SDKs and templates for building on top of the peer platform

---

## Templates & Applications

### The App Layer

Goop² is infrastructure. But infrastructure without applications is an empty road. To make the ephemeral web useful from day one, Goop² ships with ready-to-use templates — pre-built applications that turn a peer into a blog, a quiz, a corkboard, or a game server with zero configuration.

Every template is just HTML + JS + a SQLite schema. The peer serves the UI, the database stores the data, and the mesh handles communication. No cloud, no backend, no deployment pipeline. Pick a template, start your peer, you're live.

### Remote Data: How Visitors Interact

When PeerB visits PeerA's template site (at `/p/<peerA>/`), all data operations are automatically routed to **PeerA's database** via the P2P data protocol (`/goop/data/1.0.0`). The JavaScript client (`goop-data.js`) detects the remote context from the URL and transparently proxies API calls through the local server to the remote peer.

This means:
- A visitor filling out an enquete form writes to the **site owner's** database
- The `_owner` field is set to the **visitor's** peer ID (cryptographically authenticated by libp2p)
- The `_owner_email` is resolved from the site owner's peer table (from presence messages)
- Templates need **zero changes** to work in both local and remote contexts
- Single-response checks (e.g. "has this user already submitted?") work because the query runs against the site owner's DB with the visitor's `_owner` ID

### Template Library

#### Community & Social

**The Corkboard (Prikbord)** — The digital reincarnation of the supermarket advertisement board. Full skeuomorphic design — cork texture, pinned cards with pushpins, slightly rotated notes, handwritten-style fonts, torn paper edges. Pin a note: title, description, optional category, optional "contact me" info. Notes are visible to all peers. Color-coded by category: selling (yellow), looking for (blue), offering (green), event (pink). Notes expire after a configurable period (default 30 days).

**Blog** — Markdown posts with title, date, tags. Comments from visiting peers are written to the blog owner's database via P2P. RSS-like feed aggregation across peers.

**Guestbook** — Visitors leave messages on your peer's page. Each entry is signed with the visitor's cryptographic peer identity. Classic web nostalgia.

**Link Board** — Shared bookmarks within a community. Submit links with title, description, tags. Upvote/downvote by peers. A decentralized Reddit at community scale.

#### Education

**Quiz / Exam** — Teacher creates questions. Students visit the teacher's quiz page and answer — responses write to teacher's database via P2P. Each student's response is tagged with their peer ID. Results stored in teacher's database — instant grading. Timer, leaderboard. No Kahoot subscription required.

**Classroom Board** — Teacher posts announcements, assignments, resources. Students can submit work.

**Flashcards** — Create and share flashcard decks. Spaced repetition study mode.

#### Productivity

**Wiki** — Collaborative pages with edit history. Markdown-based. Peer-contributed.

**File Share** — Drag and drop file sharing between peers.

**Kanban Board** — Columns: To Do, In Progress, Done. Shared state across group members.

#### Games

**Trivia Night** — Host creates question sets. Players join, answer in real-time. Timed rounds, score tracking.

**Chess / Board Games** — Game state synchronized between players via group stream. Move history stored in SQLite. ELO rating tracked per community.

**Drawing Game** — Pictionary-style. Real-time stroke synchronization via group channel.

**Card Games** — Framework for turn-based card games.

### Template Architecture

Each template consists of:

```
templates/
  corkboard/
    schema.sql        -- SQLite tables for this template
    template.html     -- Go template for the UI
    style.css         -- Template-specific styles
    app.js            -- Client-side logic
    manifest.json     -- Template metadata
```

**Transparent Local/Remote Data Routing**: Templates include `goop-data.js`, which provides a unified data API (`Goop.data`). On load, the script checks `window.location.pathname`:

- **Local context** (e.g. `/site/index.html`) — API calls go to `/api/data/*` (direct local database)
- **Remote context** (e.g. `/p/<peerID>/index.html`) — API calls go to `/api/p/<peerID>/data/*` (proxied to remote peer via P2P)

This detection is fully transparent. Templates use the same `Goop.data.insert()`, `Goop.data.query()`, etc. regardless of context.

**manifest.json example:**

```json
{
    "name": "Corkboard",
    "description": "Community advertisement board — pin notes, share with neighbors",
    "version": "1.0.0",
    "icon": "pushpin",
    "category": "community",
    "requires_groups": false
}
```

For a game template:

```json
{
    "name": "Chess",
    "description": "Classic chess — play against a peer",
    "version": "1.0.0",
    "icon": "chess-knight",
    "category": "games",
    "requires_groups": true,
    "group_config": {
        "type": "ephemeral",
        "min_members": 2,
        "max_members": 2,
        "roles": ["white", "black"],
        "msg_types": ["game_move", "chat", "state_sync"]
    }
}
```

Templates can be **built-in** (shipped with the binary), **downloaded** (from a template registry / rendezvous server), or **custom** (user-created).

---

## Groups Protocol

### The Gap

Goop2 currently has three communication patterns:

| Pattern | Protocol | Scope |
|---------|----------|-------|
| Broadcast | GossipSub (presence) | Everyone |
| 1:1 | `/goop/chat/1.0.0` | Two peers |
| Request/response | `/goop/data/1.0.0` | Two peers |

Groups are the missing middle — N peers, scoped to a subset of the network.

### Communication Layers

| Layer | Scope | Visibility | Use Case |
|---|---|---|---|
| **Direct** | 1-to-1 | Private | Private chat, invitations |
| **Group** | N peers | Group members only | Games, collaboration, study groups |
| **Broadcast** | Entire hub | Everyone | Announcements, community chat |

### Group Types

| Type | Description | Lifecycle |
|---|---|---|
| **Ephemeral** | Created for a specific activity (game, quiz) | Auto-dissolves when activity ends |
| **Persistent** | Ongoing groups (study group, project team) | Exists until explicitly closed |
| **Open** | Any community member can join | Listed in community UI |
| **Invite-only** | Requires invitation from a member | Not listed, join via direct invite |

### Architecture: Host-Relayed

The group creator (host) acts as the hub. Each member opens a long-lived bidirectional stream to the host. The host relays messages between members.

Why host-relayed:
- The host **serves the site** (UI, templates) via `/goop/site/1.0.0`
- The host **stores the data** (game state, form responses) via `/goop/data/1.0.0`
- The host **knows the visitors** (peer table, presence)
- Low member counts (2-30) mean fan-out cost is negligible
- Private by design — only the host knows the full member list

```
Member A  <──stream──>  Host  <──stream──>  Member B
                         │
                    (fan-out relay)
```

### Protocol: `/goop/group/1.0.0`

A long-lived bidirectional stream between each member and the host. JSON messages flow in both directions, newline-delimited.

**Wire format:**

```json
{"type":"join","group":"chess-42","payload":{}}
{"type":"msg","group":"chess-42","from":"<peerID>","payload":{"move":"e2e4"}}
{"type":"leave","group":"chess-42"}
```

**Message Types:**

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

**Lifecycle:**

1. Host creates a group (local operation, stored in DB)
2. Host shares group ID (via site UI, direct chat, or presence)
3. Member opens stream to host on `/goop/group/1.0.0`
4. Member sends: `{"type":"join","group":"chess-42"}`
5. Host validates, adds to member list
6. Host sends: `{"type":"welcome","group":"chess-42","payload":{"members":[...],"state":{...}}}`
7. Host broadcasts to other members: `{"type":"members",...}`
8. Messages flow bidirectionally (host relays with `from` set server-side)
9. On leave: member sends leave, host broadcasts updated member list
10. On close: host sends close to all members, tears down streams

**Identity:** Same principle as the data protocol — the `from` field on relayed messages is set by the host from `s.Conn().RemotePeer()`, not from the message payload. Cryptographically authenticated, unforgeable.

### Group Storage

Groups are stored in the host's SQLite database:

```sql
CREATE TABLE _groups (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    app_type    TEXT,              -- 'chess', 'quiz', 'chat', etc.
    max_members INTEGER DEFAULT 0, -- 0 = unlimited
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

Membership is tracked in-memory (only online peers matter). Subscriptions are tracked locally on the subscriber side:

```sql
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

### Interaction with Existing Protocols

| Task | Protocol |
|------|----------|
| Serve game UI | `/goop/site/1.0.0` |
| Store game state, scores, history | `/goop/data/1.0.0` |
| Real-time moves, chat, sync | `/goop/group/1.0.0` |
| Discover available groups | Site UI + `/goop/data/1.0.0` (query `_groups` table) |

Templates use all three protocols transparently. A chess template's `app.js` would:

1. `Goop.data.query("_groups")` — list open games
2. `Goop.group.join("chess-42")` — join via group protocol
3. `Goop.group.send({move: "e2e4"})` — send moves in real-time
4. `Goop.data.query("chess_moves")` — review move history

### JavaScript API (goop-group.js)

```js
Goop.group = {
    join(groupId, onMessage) { ... },
    send(payload) { ... },
    leave() { ... },
    list() { return Goop.data.query("_groups"); },
};
```

### UI Design

**Navigation:**

```
Peers | Me | Create | Groups | Database | Logs
```

Inside the Create tab:

```
[Editor]  [Templates]  [Groups]
```

**Create > Groups (Host-side):** Where a host creates and manages the groups they offer. Create Group form, hosted groups list, per-group actions.

**Groups Tab (Subscriber-side):** All groups the peer is subscribed to, divided into "My Groups (Admin)" and "Joined Groups." Only groups whose host is currently online are shown as reachable.

**Discovery:** Peers find groups via the host's site UI, direct chat invitations, or presence metadata.

### How Templates Use Groups

**Quiz Flow:**
1. Teacher selects Quiz template, creates questions
2. Teacher shares site URL or creates an open, ephemeral group
3. Students visit the teacher's quiz page at `/p/<teacher>/`
4. Students submit answers — written to teacher's database with `_owner = student's peer ID`
5. Teacher's peer grades answers from its own SQLite
6. Group channel provides real-time coordination (timer, leaderboard sync)

**Chess Match:**
1. Peer-A selects Chess template
2. Peer-A creates an invite-only, ephemeral group
3. Peer-A sends invitation to Peer-B via direct message
4. Moves exchanged via group channel, state stored in SQLite
5. Game ends — group dissolves, move history persists

### Open Questions

1. **Reconnection** — if a member's stream drops, should they auto-rejoin?
2. **Persistence** — should group chat history be stored?
3. **Permissions** — should the host define roles (player vs spectator vs admin)?
4. **WebSocket vs SSE** — browser needs a persistent connection to local viewer
5. **Offline group display** — show greyed-out or hide entirely?

---

## Distributed Compute

### The Insight

Every Goop2 peer is already a compute node. It has a CPU, a database, a scripting engine, and a network identity. When a visitor calls a Lua function on a peer, the peer does work and returns a result. That's a remote procedure call. The infrastructure for distributed computation already exists — it just doesn't know it yet.

The missing piece is coordination. Today, one peer calls one function on one other peer. Add the ability to split work across a group and queue tasks for offline peers, and Goop2 becomes a serverless compute fabric running on consumer hardware.

### The Cloud Parallel

| Cloud Concept | Goop2 Equivalent |
|---|---|
| Azure Service Fabric / AWS Lambda | Lua data functions on peers |
| Service registry / discovery | Peer groups + `lua-list` |
| Message queue (SQS, Service Bus) | Work queue in host's database |
| Stateful services | SQLite per peer |
| API Gateway | Data protocol proxy |
| IAM / authentication | libp2p peer identity (cryptographic) |
| Container orchestration | Peer groups with role assignment |
| Health checks / heartbeat | GossipSub presence |

The critical difference: cloud compute runs on always-on VMs in data centers. Goop2 compute runs on devices that are already turned on for other reasons — laptops, desktops, home servers. The marginal energy cost is effectively zero.

### What Exists Today

- **Lua data functions** — `call(request)` functions in `site/lua/functions/`, sandboxed VM with database access, hot-reloaded on file change
- **Cross-peer function calls** — `lua-call` operation via data protocol, caller identity cryptographically verified
- **Function discovery** — `lua-list` operation returns available functions with descriptions
- **Peer groups** — host-relayed groups with real-time communication
- **Database per peer** — SQLite with remote read/write via data protocol
- **Presence** — GossipSub tells every peer who's online

### What's Missing

**1. Work Queue** — A table in the coordinator's database holding work items:

```sql
CREATE TABLE _work_queue (
    _id          INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id     TEXT NOT NULL,
    function     TEXT NOT NULL,
    params       TEXT NOT NULL DEFAULT '{}',
    status       TEXT NOT NULL DEFAULT 'pending',
                 -- pending | claimed | running | completed | failed
    assigned_to  TEXT,
    result       TEXT,
    error        TEXT,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    claimed_at   DATETIME,
    completed_at DATETIME,
    ttl_seconds  INTEGER DEFAULT 300,
    retries      INTEGER DEFAULT 0,
    max_retries  INTEGER DEFAULT 3
);
```

**2. Group-Aware Dispatch** — A Lua API for fan-out:

```lua
-- Fan out to all online members
local results = goop.group.fan_out("compute-group", "process_chunk", {
    chunks = split_data(dataset, member_count)
})

-- Queue work for offline peers
goop.group.enqueue("compute-group", "process_chunk", { data = chunk })
```

**3. Claim-and-Report Worker Loop** — Workers periodically poll the coordinator, claim items, execute locally, report results back.

### Execution Models

**Immediate Fan-Out** — Coordinator splits work and calls all online peers in parallel. Best for real-time computation.

**Queued (Lazy) Execution** — Coordinator posts work items. Peers claim and execute when they come online. Best for batch processing.

**MapReduce** — Fan-out + aggregation. Coordinator distributes map function, collects intermediates, runs reduce.

### Example: Distributed Text Analysis

```lua
-- Coordinator: fan out to all peers in the group
function call(request)
    local results = goop.group.fan_out("literature-club", "count_words", {
        pattern = request.params.pattern or ".*"
    })
    local totals = {}
    for peer_id, result in pairs(results) do
        if result.counts then
            for word, count in pairs(result.counts) do
                totals[word] = (totals[word] or 0) + count
            end
        end
    end
    return { word_counts = totals, peers_contributed = table_length(results) }
end
```

Each peer processes only its own data. No raw content leaves any peer's machine. Privacy is structural, not policy-based.

### Security Considerations

| Concern | Mitigation |
|---|---|
| Malicious work items | Workers only execute functions they've installed locally |
| Result tampering | Cross-validate by sending same work to multiple peers |
| Resource exhaustion | Per-invocation limits: 5s timeout, 10MB memory, 3 HTTP requests |
| Data leakage | Each peer processes its own local data |
| Coordinator abuse | Workers opt in to groups voluntarily |
| Free-riding | Track participation via work queue |

Key principle: **workers never execute code they didn't choose to install**. A coordinator says "call function X with parameters Y." If the worker doesn't have function X, it declines.

### What This Is Not

This is not a competitor to AWS Lambda. It offers zero infrastructure cost, zero operational overhead, structural privacy, and energy efficiency — but no SLAs, no millisecond cold-starts, no managed services.

It's useful for communities that want to compute together without renting cloud infrastructure. A research group aggregating results. A classroom running distributed experiments. A game community computing leaderboards.

### Implementation Path

1. **Phase 1: Fan-Out** — `goop.group.fan_out()` for real-time distributed queries
2. **Phase 2: Work Queue** — `_work_queue` table with `claim_work` and `report_result`
3. **Phase 3: Push Notifications** — Replace polling with group protocol push
4. **Phase 4: MapReduce Abstraction** — Higher-level API for map-shuffle-reduce

---

## The Big Picture

```
Super Hub (directory of communities)
    │
    └── Community Hub (rendezvous server)
            │
            ├── Peer A (blog template active)
            │     └── serves: blog posts, comments
            │
            ├── Peer B (corkboard template active)
            │     └── serves: pinned notes, community ads
            │
            ├── Peer C (quiz template active)
            │     └── hosts: "Math Quiz" group
            │           ├── Peer D (student)
            │           ├── Peer E (student)
            │           └── Peer F (student)
            │
            └── Group: "Chess Club"  (persistent, open)
                  ├── Peer A
                  ├── Peer D
                  └── ongoing matches as ephemeral sub-groups
```

Templates make Goop² useful. Groups make it interactive. Together, they turn every peer into a platform and every community into a living, breathing space.

*Goop² — the ephemeral web.*
