# Group File Sharing

## Overview

Group File Sharing turns Goop2 groups into shared folders. Each peer in a group can upload files to their local storage. The Files tab shows a merged virtual filesystem of all group members' files, namespaced by owner. File content is fetched on demand from the owning peer via a dedicated libp2p protocol. Only the owner can modify or delete their files.

This is not a sync engine. There is no replication, no conflict resolution, no eventual consistency. Files stay with their owner. When you download a file, you fetch it directly from the peer who uploaded it. When that peer goes offline, the file becomes unavailable. This is consistent with the ephemeral web philosophy: nothing persists without the owner's presence.

---

## How It Works

```
PeerA uploads "notes.txt" to group X
  -> stored locally: <peerA-dir>/shared/<group-x-id>/notes.txt
  -> broadcasts "doc-added" event via group relay

PeerB opens Files tab, selects group X
  -> viewer queries all online group members via /goop/docs/1.0.0
  -> sees "notes.txt" listed under PeerA
  -> clicks download
  -> viewer opens /goop/docs/1.0.0 stream to PeerA
  -> fetches file bytes on demand
  -> streams to browser
```

### Virtual FS Mesh

The Files tab shows a merged view of all group members' files. Files are namespaced by owner peer ID, so PeerA and PeerB can both have a file called `notes.txt` without conflict. Each peer's files live in their own `shared/<group-id>/` directory on their local filesystem.

### Access Control

Only peers who are members of a group can list or download files for that group. This is enforced at the libp2p protocol level: the stream handler verifies the requesting peer is a current group member before serving any data. The upload API also verifies group membership before accepting files.

### Constraints

- **Max file size**: 50 MB per file
- **No local caching**: Remote files are always fetched on demand
- **Owner-only modification**: Only the file owner can delete or overwrite their files
- **Ephemeral availability**: Files are only accessible while the owning peer is online

---

## Protocol: `/goop/docs/1.0.0`

A request/response protocol for listing and fetching files from a remote peer.

### Wire Format

**Request** (JSON, newline-terminated):

```json
{"op":"list","group_id":"<group-id>"}
{"op":"get","group_id":"<group-id>","file":"<filename>"}
```

**List response** (JSON):

```json
{"ok":true,"files":[{"name":"notes.txt","size":1234,"hash":"sha256:abc..."}]}
```

Error:

```json
{"ok":false,"error":"access denied"}
```

**Get response** (binary):

```
OK <content-type> <size-in-bytes>\n
<file bytes>
```

Error:

```
ERR <error message>\n
```

### Access Control Flow

When a peer opens a docs stream:

1. The handler extracts the remote peer ID from the libp2p connection (`s.Conn().RemotePeer()`)
2. For `list` and `get` operations, the handler checks if the requesting peer is a member of the specified group
3. If the peer is not a group member, the request is rejected with `"access denied"`
4. Group membership is checked via the `GroupChecker` interface, which delegates to the group manager's `IsPeerInGroup()` method

---

## Storage

### Local File Layout

Files are stored on the local filesystem under the peer directory:

```
<peer-dir>/
  shared/
    <group-id-1>/
      notes.txt
      report.pdf
    <group-id-2>/
      slides.pptx
```

### Path Safety

The file store uses the same `cleanAbs()` pattern as the content store to prevent path traversal attacks. Filenames are sanitized:

- Path separators (`/`, `\`) are rejected
- Directory traversal components (`.`, `..`) are rejected
- The resolved path must be within the group's storage directory

### Store API

```go
store := docs.NewStore(peerDir)

// Save a file (enforces 50 MB limit)
err := store.Save(groupID, filename, fileBytes)

// Read a file
data, err := store.Read(groupID, filename)

// Delete a file
err := store.Delete(groupID, filename)

// List files in a group
files, err := store.List(groupID)
// Returns []DocInfo{Name, Size, ModTime, Hash}

// List all groups with files
groups, err := store.ListGroups()

// Check if any files exist for a group
has := store.HasFiles(groupID)
```

---

## HTTP API

All endpoints are served by the viewer's HTTP server.

### `GET /api/docs/my?group_id=X`

List the local peer's own shared files for a group.

**Response:**

```json
{
  "files": [
    {"name": "notes.txt", "size": 1234, "hash": "sha256:abc..."}
  ]
}
```

### `POST /api/docs/upload`

Upload a file to share with a group. Multipart form with fields:

- `group_id` — the group to share the file with
- `file` — the file to upload

Enforces the 50 MB size limit. After saving, broadcasts a `doc-added` event through the group relay so other members see the update in real time.

**Response:**

```json
{"ok": true, "filename": "notes.txt"}
```

### `POST /api/docs/delete`

Delete one of your own shared files. JSON body:

```json
{"group_id": "xxx", "filename": "notes.txt"}
```

Broadcasts a `doc-removed` event through the group relay.

### `GET /api/docs/browse?group_id=X`

Aggregate view of all group members' files. The viewer backend:

1. Lists the local peer's own files from the filesystem
2. Queries each online group member in parallel via `/goop/docs/1.0.0`
3. Merges results into a single response

**Response:**

```json
{
  "peers": [
    {
      "peer_id": "12D3KooW...",
      "label": "Alice",
      "self": true,
      "files": [{"name": "notes.txt", "size": 1234}]
    },
    {
      "peer_id": "12D3KooX...",
      "label": "Bob",
      "self": false,
      "files": [{"name": "report.pdf", "size": 56789}]
    },
    {
      "peer_id": "12D3KooY...",
      "label": "Carol",
      "self": false,
      "error": "peer offline"
    }
  ]
}
```

### `GET /api/docs/download?peer_id=X&group_id=Y&file=Z`

Download a file. If `peer_id` matches the local peer, the file is served directly from the filesystem. Otherwise, the viewer proxies the request to the remote peer via `/goop/docs/1.0.0` and streams the response to the browser.

---

## Group Relay Integration

File events are broadcast through the existing group message system using `TypeMsg` with structured payloads:

```json
{"type": "msg", "group": "xxx", "payload": {"action": "doc-added", "file": {"name": "notes.txt", "size": 1234}}}
{"type": "msg", "group": "xxx", "payload": {"action": "doc-removed", "file": "notes.txt"}}
```

The frontend JavaScript listens for these events via the existing `/api/groups/events` SSE stream and auto-refreshes the file list when changes occur. No new group message types are needed.

---

## UI

### Navigation

The Files tab appears in the top navigation bar between Database and Logs:

```
Peers | Me | Create | Database | Files | Logs
```

### Page Layout

1. **Group selector** — Dropdown populated from hosted groups and subscribed groups. Selecting a group activates the file browser.

2. **Upload area** — File input with upload button. Appears after selecting a group. Shows upload progress.

3. **My Files** — Table listing the local peer's shared files with Name, Size, and Actions (Download, Delete) columns.

4. **Group Files** — Files from other group members, grouped by peer. Each peer block shows:
   - Peer label with online/offline status indicator
   - File count
   - File table with Name, Size, and Download action

### Real-Time Updates

The JavaScript subscribes to group events via `Goop.group.subscribe()`. When a `doc-added` or `doc-removed` event arrives, the file browser auto-refreshes without manual intervention.

---

## Implementation Files

| File | Purpose |
| -- | -- |
| `internal/docs/store.go` | Local file storage with path safety and size limits |
| `internal/p2p/docs.go` | libp2p stream handler and client for `/goop/docs/1.0.0` |
| `internal/viewer/routes/docs.go` | HTTP API endpoints |
| `internal/viewer/routes/docs_ui.go` | Files page route |
| `internal/ui/viewmodels/docs.go` | View model |
| `internal/ui/templates/documents.html` | HTML template |
| `internal/ui/assets/js/76-documents.js` | Frontend JavaScript |
| `internal/ui/assets/css/67-documents.css` | Styles |

### Integration Points

| File | Change |
| -- | -- |
| `internal/proto/proto.go` | Added `DocsProtoID` constant |
| `internal/group/manager.go` | Added `IsPeerInGroup()` for access control |
| `internal/p2p/node.go` | Added `EnableDocs()` method |
| `internal/app/run.go` | Initializes file store, wires to node and viewer |
| `internal/viewer/viewer.go` | Passes file store and group manager to routes |
| `internal/viewer/routes/register.go` | Registers file sharing routes, adds to `Deps` struct |
| `internal/ui/templates/layout.html` | Files nav item |
| `internal/ui/assets/app.js` | Loads `76-documents.js` |
| `internal/ui/assets/app.css` | Imports `67-documents.css` |

---

## Configuration

No configuration is required. File sharing is enabled automatically when the peer joins a group. The shared files directory (`<peer-dir>/shared/`) is created on first upload.

---

## Interaction with Other Protocols

| Task | Protocol |
| -- | -- |
| Upload/delete files | Local HTTP API (viewer) |
| Browse remote files | `/goop/docs/1.0.0` (list operation) |
| Download remote files | `/goop/docs/1.0.0` (get operation) |
| Real-time notifications | `/goop/group/1.0.0` (TypeMsg relay) |
| Group membership check | `/goop/group/1.0.0` (membership state) |
| Peer discovery/labels | GossipSub presence |
