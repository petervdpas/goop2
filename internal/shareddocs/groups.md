# Groups & Collaboration

Groups are real-time, multi-peer communication channels. They enable features like multiplayer games, live quizzes, collaborative editing, group chat, file sharing, and distributed compute.

## How groups work

Groups use a **host-relayed model**. The peer that creates a group acts as the hub. Every member opens a long-lived bidirectional stream to the host, and the host relays messages between members.

```mermaid
graph LR
    A["Member A"] <-->|stream| H["Host"]
    H <-->|stream| B["Member B"]
    H <-->|stream| C["Member C"]
```

This model works naturally because the host already serves the site, stores data, and knows its visitors. All group events flow through the unified MQ bus.

## Group types

| Type | App type | Lifecycle | Use case |
|------|----------|-----------|----------|
| **Ephemeral** | varies | Auto-dissolves when activity ends | Games, quizzes |
| **Persistent** | varies | Exists until explicitly closed | Study groups, teams |
| **Open** | varies | Any peer can join | Community chat |
| **Invite-only** | varies | Requires invitation | Private projects |
| **Files** | `files` | Persistent | Shared file storage |
| **Cluster** | `cluster` | Volatile | Distributed compute |
| **Listen** | `listen` | Varies | Audio listening sessions |

## Creating a group

Groups are created by the host peer through the viewer's **Groups** page or programmatically through the API. The group is stored in the host's SQLite database and made available to visitors.

## Joining a group

Members join by sending a join message to the host over the MQ bus. The join flow is:

1. Member sends a `join` message via MQ to the host (`group:{groupID}:join`).
2. Host validates and adds the member.
3. Host sends a `welcome` message with the current member list and state.
4. Host broadcasts an updated `members` list to all other members.

## Message types

| Type | Direction | Purpose |
|------|-----------|---------|
| `join` | Member to Host | Request to join a group |
| `welcome` | Host to Member | Confirmation with current state |
| `members` | Host to Members | Updated member list |
| `msg` | Both directions | Application message (chat, game move) |
| `meta` | Host to Members | Group metadata update |
| `ping` / `pong` | Both directions | Keep-alive |
| `leave` | Member to Host | Member leaving |
| `close` | Host to Members | Group is being closed |

All group events are published on the MQ bus under the topic `group:{groupID}:{type}`. Group invites use `group.invite`.

## File sharing

File groups let peers share documents within a group. Any member can upload files and browse or download files shared by other members.

### Creating a file group

Create a group with app type `files` from the **Groups** page. File groups are persistent by default.

### Uploading files

Upload files through the viewer's file sharing UI or via the API:

- `POST /api/docs/upload` -- Multipart upload (max 50 MB per file)
- `POST /api/docs/upload-local` -- Upload from a local filesystem path

### Browsing and downloading

- `GET /api/docs/browse` -- Aggregates file lists from all group members (parallel query, 8s timeout per peer)
- `GET /api/docs/download` -- Download a file from any member (local or proxied from remote peer)
- `GET /api/docs/my` -- List your own shared files in a group

Files are stored on each member's disk. When you browse, the viewer queries all online members and merges their file lists. Downloads are streamed directly from the owning peer.

## Cluster compute

Cluster groups enable distributed computation across peers. One peer acts as the **host** (dispatcher) and others join as **workers**. The host dispatches jobs to workers, which execute them using a configured executor binary.

### Roles

| Role | Responsibility |
|------|---------------|
| **Host** | Creates the cluster, dispatches jobs, collects results |
| **Worker** | Joins a cluster, executes jobs using an executor binary |

### Setting up a worker

Configure the executor binary in your `goop.json`:

```json
{
  "viewer": {
    "cluster_binary_path": "/path/to/my-executor",
    "cluster_binary_mode": "daemon"
  }
}
```

Binary modes:
- **oneshot** -- Started per job, exits after producing a result.
- **daemon** -- Started once, handles multiple jobs via stdin/stdout JSON.

See the [Executor Protocol](executor) page for the full binary contract and code examples.

### Submitting jobs

The host submits jobs via the API or UI. Jobs have a type, payload, optional priority, timeout, and retry policy. The dispatcher assigns jobs to available workers and streams output back to the host.

## Template groups

When a template's schemas use `group` access policies or define a roles map, Goop2 automatically creates a co-author group on apply. Members join via the Groups page. The owner always has full access.

### Role-based data access

Each schema can define custom roles with per-operation permissions:

```json
{
  "name": "posts",
  "access": { "read": "open", "insert": "group", "update": "group", "delete": "owner" },
  "roles": {
    "editor": { "read": true, "insert": true, "update": true, "delete": true },
    "viewer": { "read": true }
  }
}
```

When a remote peer performs a data operation on a `group`-policy table, the P2P data layer looks up the peer's role in the template group and checks it against the schema's roles map. Unknown roles are denied.

Roles are managed through the Schema editor's **Roles** tab in the Database page.

A peer can ask the host for its role and permissions on any schema:

```javascript
var r = await Goop.data.role("posts");
// r.role = "coauthor", r.permissions = {read: true, insert: true, ...}
```

This goes through the P2P data protocol — the host is the authority.

### Group lifecycle

- **Apply template** -- group created if any schema needs it, owner auto-joins
- **Re-apply same template** -- existing group and members preserved
- **Switch to different template** -- old group closed, new one created if needed
- **No group schemas** -- no group created

The group ID is tracked in `_meta("template_group_id")`.

## JavaScript API

Templates interact with groups through the `Goop.group` API:

```javascript
// List available groups
const groups = await Goop.data.query("_groups");

// Join a group
Goop.group.join("chess-42", function(msg) {
    console.log("Received:", msg);
});

// Send a message
Goop.group.send({ move: "e2e4" });

// Leave the group
Goop.group.leave();
```

## Use cases

### Multiplayer game

1. Host creates an ephemeral group for a chess match.
2. Opponent joins via the game UI.
3. Moves are exchanged in real time through the group channel.
4. Move history is stored in the host's database.
5. When the game ends, the group dissolves but the history persists.

### File sharing workspace

1. Host creates a file group for a project.
2. Team members join and upload documents.
3. Everyone can browse and download files from any member.
4. Files live on each member's machine -- no central storage.

### Distributed processing

1. Host creates a cluster and submits computation jobs.
2. Workers join and execute jobs using their local executor binary.
3. Results stream back to the host in real time.
4. Workers can join and leave dynamically; the dispatcher handles reassignment.

## Interaction with other protocols

| Task | Protocol |
|------|----------|
| Serve the UI | `/goop/site/1.0.0` |
| Store persistent data | `/goop/data/1.0.0` |
| Real-time messaging | MQ bus (`/goop/mq/1.0.0`) |
| Discover groups | Query `_groups` via `/goop/data/1.0.0` |
| Event delivery | MQ topics (`group:{groupID}:{type}`) |
