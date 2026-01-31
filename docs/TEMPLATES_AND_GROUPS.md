# Goop² Templates, Groups & Applications

## The App Layer

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

---

## Template Library

### Community & Social

#### The Corkboard (Prikbord)

The digital reincarnation of the supermarket advertisement board. Full skeuomorphic design — cork texture, pinned cards with pushpins, slightly rotated notes, handwritten-style fonts, torn paper edges.

- Pin a note: title, description, optional category, optional "contact me" info
- Notes are visible to all peers visiting the corkboard
- Visiting peers pin notes to the **corkboard owner's** database via P2P
- Each note is tagged with the visitor's peer ID (`_owner`) — no login required
- Cards appear scattered on the board, each with a random slight rotation
- Color-coded by category: selling (yellow), looking for (blue), offering (green), event (pink)
- Tear-off tabs at the bottom of cards for contact info — click to copy
- Notes expire after a configurable period (default 30 days)

This is the template that explains Goop² to anyone over 30 in five seconds.

#### Blog

- Markdown posts with title, date, tags
- Comments from visiting peers are written to the **blog owner's database** via P2P — each comment tagged with the commenter's peer ID
- RSS-like feed aggregation across peers in the community
- Simple, clean design — content first

#### Guestbook

- Visitors leave messages on your peer's page — entries stored in the **host's database** via P2P
- Each entry is signed with the visitor's cryptographic peer identity
- Classic web nostalgia — visitor counter, timestamps, optional avatar
- The digital equivalent of signing someone's yearbook

#### Link Board

- Shared bookmarks within a community
- Submit links with title, description, tags
- Upvote/downvote by peers
- Sort by new, popular, or category
- A decentralized Reddit at community scale

### Education

#### Quiz / Exam

- Teacher creates questions (multiple choice, open answer, true/false)
- Students visit the teacher's quiz page and answer — responses write to **teacher's database** via P2P
- Each student's response is tagged with their peer ID (`_owner`) — no accounts needed
- Results stored in teacher's database — instant grading
- Timer per question or per quiz
- Leaderboard displayed after completion
- No Kahoot subscription required

#### Classroom Board

- Teacher posts announcements, assignments, resources
- Students can submit work (text, links)
- Discussion threads per assignment
- Simple gradebook

#### Flashcards

- Create and share flashcard decks
- Spaced repetition study mode
- Peers can contribute cards to shared decks
- Track study progress in local database

### Productivity

#### Wiki

- Collaborative pages with edit history
- Markdown-based content
- Peer-contributed — anyone in the community can edit
- Conflict resolution: last write wins with edit history preserved
- Table of contents auto-generated

#### File Share

- Drag and drop file sharing
- Peers can browse and download
- File metadata stored in SQLite, files served from local filesystem
- Optional access control (public to community, or specific peers only)

#### Kanban Board

- Columns: To Do, In Progress, Done (customizable)
- Cards with title, description, assigned peer
- Drag and drop between columns
- Shared state across group members

### Games

#### Trivia Night

- Host creates question sets
- Players join a group, answer in real-time
- Timed rounds, score tracking, leaderboard
- Perfect for pub quiz, classroom review, community events

#### Chess / Board Games

- Classic board games served from a peer
- Game state synchronized between two (or more) players via group stream
- Move history stored in SQLite
- ELO rating tracked per community

#### Drawing Game

- Pictionary-style — one peer draws, others guess
- Canvas-based drawing tool
- Real-time stroke synchronization via group channel
- Word lists customizable by the host

#### Card Games

- Framework for turn-based card games
- Deck management, hand tracking, discard pile
- Supports multiple game rule sets (bridge, poker, etc.)
- Game state in SQLite, moves via group messages

---

## Groups: The Missing Layer

### The Problem

Goop² currently has two communication layers:

1. **Direct messages** — 1-to-1, private
2. **Broadcast** — entire community, public

This is enough for chat, but not enough for applications. A chess game between two peers shouldn't be broadcast to the entire community. A study group of five students needs a private channel that isn't 1-to-1. A quiz with 30 participants needs a scoped communication stream.

### The Solution: Groups

A group is a subset of peers within a community that share a private communication channel. Groups are ephemeral by default — they exist for a purpose and dissolve when that purpose is complete.

#### Communication Layers

| Layer | Scope | Visibility | Use Case |
|---|---|---|---|
| **Direct** | 1-to-1 | Private | Private chat, invitations |
| **Group** | N peers | Group members only | Games, collaboration, study groups |
| **Broadcast** | Entire hub | Everyone | Announcements, community chat |

#### Group Types

| Type | Description | Lifecycle |
|---|---|---|
| **Ephemeral** | Created for a specific activity (game, quiz) | Auto-dissolves when activity ends |
| **Persistent** | Ongoing groups (study group, project team) | Exists until explicitly closed |
| **Open** | Any community member can join | Listed in community UI |
| **Invite-only** | Requires invitation from a member | Not listed, join via direct invite |

#### How Groups Work

1. **Creation**: A peer creates a group with a name, type, and optional settings
2. **Invitation**: Creator sends group ID to other peers via direct message
3. **Joining**: Invited peers join the group, establishing a group stream
4. **Communication**: Messages within the group are routed only to group members
5. **State sync**: Application state (game moves, quiz answers) flows over the group channel
6. **Dissolution**: Group closes when the activity ends or the creator closes it

#### Data Model

```sql
-- Groups this peer participates in
CREATE TABLE groups (
    id          TEXT PRIMARY KEY,   -- unique group identifier
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,      -- 'ephemeral', 'persistent'
    visibility  TEXT NOT NULL,      -- 'open', 'invite'
    app_type    TEXT,               -- 'chess', 'quiz', 'chat', etc.
    created_by  TEXT NOT NULL,      -- peer ID of creator
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    closed_at   DATETIME
);

-- Members of each group
CREATE TABLE group_members (
    group_id    TEXT NOT NULL,
    peer_id     TEXT NOT NULL,
    role        TEXT DEFAULT 'member',  -- 'host', 'member', 'spectator'
    joined_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (group_id, peer_id),
    FOREIGN KEY (group_id) REFERENCES groups(id)
);

-- Messages/events within a group
CREATE TABLE group_messages (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id    TEXT NOT NULL,
    from_peer   TEXT NOT NULL,
    msg_type    TEXT DEFAULT 'chat',  -- 'chat', 'game_move', 'state_sync', 'system'
    content     TEXT NOT NULL,
    timestamp   DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (group_id) REFERENCES groups(id)
);
```

#### Protocol

Groups require a new message type on the libp2p layer:

```
Message {
    type:     "group"
    group_id: "abc123"
    from:     peer_id
    msg_type: "chat" | "game_move" | "state_sync" | "system"
    content:  { ... }
}
```

The hub does not need to know about groups. Group messages are routed directly between peers — the hub only facilitates discovery. This keeps groups private and reduces hub load.

#### UI Integration

**Peers page**: Active groups shown below the peer list, or as a separate tab. Each group shows:
- Group name and type
- Member count / who's in it
- Application type (if it's a game/quiz/etc.)
- "Join" or "Open" button

**Group view**: Clicking into a group opens the group's application:
- Chat groups → group chat interface
- Game groups → game UI (served by the host peer's template)
- Quiz groups → quiz interface
- Collaboration groups → shared workspace

**Create group**: Button on the peers page to create a new group. Options:
- Name
- Type (ephemeral/persistent)
- Visibility (open/invite)
- Application (chat, game, quiz, etc.)
- Max members (optional)

---

## How Templates Use Groups

Templates are the UI layer. Groups are the communication layer. Together they create applications.

### Example: Quiz Flow

1. Teacher selects the "Quiz" template and creates questions (stored in teacher's local DB)
2. Teacher shares their site URL or creates an **open, ephemeral group** named "History Quiz - Chapter 5"
3. Students visit the teacher's quiz page at `/p/<teacher>/`
4. `goop-data.js` detects the remote context — all API calls route to teacher's DB via P2P
5. Students submit answers — each response is written to **teacher's database** with `_owner = student's peer ID`
6. Teacher's peer grades answers directly from its own SQLite
7. Students can revisit to see results — queries return their own rows (matched by `_owner`)
8. Optionally, a group channel provides real-time coordination (timer, leaderboard sync)
9. Results persist in the teacher's database for grading

### Example: Chess Match

1. Peer-A selects the "Chess" template
2. Peer-A creates an **invite-only, ephemeral group** named "Chess: A vs B"
3. Peer-A sends the group invitation to Peer-B via **direct message**
4. Peer-B joins the group
5. Peer-A's peer serves the chess board UI via the **ephemeral web**
6. Moves are exchanged via the **group channel** as `game_move` messages
7. Game state is stored in both peers' **SQLite** databases
8. Game ends — group dissolves, move history persists

### Example: Study Group

1. Student creates a **persistent, invite-only group** named "Calculus Study"
2. Invites 4 classmates via **direct messages**
3. Group has a shared chat channel
4. Members can share links, notes, files via the group
5. Optional: attach a "Wiki" template for collaborative notes
6. Group persists across sessions — members reconnect when online

---

## Template Architecture

Each template consists of:

```
templates/
  corkboard/
    schema.sql        -- SQLite tables for this template
    template.html     -- Go template for the UI
    style.css         -- Template-specific styles
    app.js            -- Client-side logic
    manifest.json     -- Template metadata (name, description, icon, group requirements)
```

### Transparent Local/Remote Data Routing

Templates include `goop-data.js`, which provides a unified data API (`Goop.data`). On load, the script checks `window.location.pathname`:

- **Local context** (e.g. `/site/index.html`) — API calls go to `/api/data/*` (direct local database)
- **Remote context** (e.g. `/p/<peerID>/index.html`) — API calls go to `/api/p/<peerID>/data/*` (proxied to remote peer via P2P)

This detection is fully transparent. Templates use the same `Goop.data.insert()`, `Goop.data.query()`, etc. regardless of context. No template code changes are needed to support remote visitors.

### manifest.json

```json
{
    "name": "Corkboard",
    "description": "Community advertisement board — pin notes, share with neighbors",
    "version": "1.0.0",
    "icon": "pushpin",
    "category": "community",
    "requires_groups": false,
    "group_config": null
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

### Template Installation

Templates can be:

1. **Built-in** — shipped with the Goop² binary
2. **Downloaded** — from a template registry (future: hosted on a super hub)
3. **Custom** — user-created, dropped into the templates directory

The peer's "Me" page or a dedicated "Apps" page shows available templates. Activating a template runs its `schema.sql` against the peer's database and registers its routes.

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

The web was supposed to be this. Now it can be.

---

*See also: [ARCHITECTURE_VISION.md](ARCHITECTURE_VISION.md) for the foundational concepts of communities, hubs, and the ephemeral web.*
