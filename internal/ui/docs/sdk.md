# JavaScript SDK

The Goop2 SDK is a set of JavaScript modules that templates load to interact with the peer. Each module is a single `<script>` tag served from `/sdk/`.

## Loading

```html
<script src="/sdk/goop-data.js"></script>
<script src="/sdk/goop-identity.js"></script>
<script src="/sdk/goop-ui.js"></script>
<script src="app.js"></script>
```

All modules attach to the global `Goop` object. Load only what you need.

## Modules

| Module | Global | Purpose |
|--------|--------|---------|
| `goop-data.js` | `Goop.data` | Database CRUD, schema, Lua function calls |
| `goop-identity.js` | `Goop.identity` | Peer ID, display name, email |
| `goop-peers.js` | `Goop.peers` | Peer discovery and status polling |
| `goop-group.js` | `Goop.group` | Group membership, messaging |
| `goop-chat.js` | `Goop.chat` | Direct and broadcast chat over MQ |
| `goop-realtime.js` | `Goop.realtime` | Virtual MQ-based channels |
| `goop-call.js` | `Goop.call` | WebRTC audio/video calling |
| `goop-site.js` | `Goop.site` | File storage (read, upload, delete) |
| `goop-ui.js` | `Goop.ui` | Toast, confirm, prompt dialogs |
| `goop-form.js` | `Goop.form` | JSON-driven form renderer |
| `goop-forms.js` | `Goop.forms` | Auto-generated CRUD UI from schema |
| `goop-drag.js` | `Goop.drag` | Drag-and-drop with sortable lists |
| `goop-engine.js` | `GameLoop, Renderer, ...` | 2D game engine (Canvas) |

## Goop.data

Database access. Works in both self and remote peer context (URL-aware).

```javascript
const db = Goop.data;

// List tables (includes mode: orm/classic)
const tables = await db.tables();

// Create table — classic format
await db.createTable("posts", [
  {name: "title", type: "TEXT", not_null: true},
  {name: "body",  type: "TEXT"}
]);

// Create table — ORM format (typed, with keys)
await db.createTable("tasks", [
  {name: "id",    type: "integer", key: true},
  {name: "title", type: "text",    required: true},
  {name: "score", type: "real"}
]);

// CRUD
const {id} = await db.insert("tasks", {title: "Hello", score: 9.5});
const rows = await db.query("tasks", {limit: 10});
await db.update("tasks", id, {score: 10});
await db.remove("tasks", id);

// Describe (ORM tables return typed schema)
const info = await db.describe("tasks");
// {mode: "orm", schema: {name: "tasks", columns: [...]}}

// Call a Lua function
const result = await db.call("score-quiz", {answers: [...]});
```

## Goop.identity

```javascript
const myId    = await Goop.identity.id();
const myName  = await Goop.identity.label();
const myEmail = await Goop.identity.email();
```

## Goop.peers

```javascript
// One-time fetch
const peers = await Goop.peers.list();

// Live updates (polls every 5s)
Goop.peers.subscribe(function(peers) {
  // peers = [{id, label, reachable, ...}]
});
```

## Goop.group

```javascript
// List active subscriptions
const subs = await Goop.group.subscriptions();

// Join a remote group
await Goop.group.join(hostPeerId, groupId);

// Send a message to a group
await Goop.group.send(groupId, {action: "move", x: 5});
```

## Goop.chat

```javascript
// Direct message
await Goop.chat.send(peerId, "Hello!");

// Broadcast to all peers
await Goop.chat.broadcast("Server restarting");
```

## Goop.ui

```javascript
Goop.ui.toast("Saved!", "success");
Goop.ui.toast("Something went wrong", "error");

const ok = await Goop.ui.confirm("Delete this item?");
const name = await Goop.ui.prompt("Enter your name:");
```

## Goop.site

```javascript
// List files
const files = await Goop.site.files();

// Read a file
const content = await Goop.site.read("data.json");

// Upload a file
await Goop.site.upload("data.json", jsonString);
```

## Context awareness

The SDK automatically detects whether the template is running on the local peer (`/`) or viewing a remote peer (`/p/<peerID>/`). API calls are routed to the correct peer transparently. The same template code works in both contexts.
