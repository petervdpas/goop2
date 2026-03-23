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
| `goop-group.js` | `Goop.group` | Group membership, messaging, SSE events |
| `goop-chat.js` | `Goop.chat` | Direct and broadcast chat over MQ |
| `goop-realtime.js` | `Goop.realtime` | Virtual MQ-based channels |
| `goop-call.js` | `Goop.call` | WebRTC audio/video calling |
| `goop-api.js` | `Goop.api` | Virtual REST API over Lua data functions |
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

// Describe a table's columns
const info = await db.describe("tasks");

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

// Insert → {status, id}
const {id} = await db.insert("tasks", {title: "Hello", score: 9.5});

// Query with options
const rows = await db.query("tasks", {limit: 10, where: "score > ?", args: [5]});

// Update by _id
await db.update("tasks", id, {score: 10});

// Delete by _id
await db.remove("tasks", id);

// Drop a table
await db.dropTable("tasks");

// Add a column
await db.addColumn("tasks", {name: "priority", type: "INTEGER", not_null: false});

// Call a Lua data function
const result = await db.call("score-quiz", {answers: [1, 2, 3]});

// List available Lua functions
const fns = await db.functions();
```

## Goop.api

Virtual REST-like API backed by a server-side Lua function. Requires `goop-data.js`. The template defines endpoints in `api.json`; if absent, all tables are exposed with default CRUD.

```javascript
const api = Goop.api;

// Get a single record by slug or id
const post = await api.get("posts", {slug: "hello-world"});
// → {found: true, item: {_id, title, body, ...}}

const post = await api.get("posts", {id: 42});

// List records (paginated)
const result = await api.list("posts");
// → {items: [{_id, title, ...}, ...]}

const page = await api.list("posts", {limit: 10, offset: 20});

// Insert a new record
const result = await api.insert("posts", {title: "New", body: "Content"});
// → {id: 5}

// Update a record by id
await api.update("posts", 42, {title: "Updated"});

// Delete a record by id
await api.delete("posts", 42);

// Get a config table as a key-value map
const config = await api.map("config");
// → {theme: "dark", accent: "#2d6a9f", ...}
```

### api.json

Endpoint declarations. Place in the site root alongside `index.html`.

```json
{
  "posts": {
    "table": "posts",
    "slug": "slug",
    "filter": "published = 1",
    "fields": ["title", "body", "author_name", "image", "slug", "_created_at"],
    "get": true,
    "list": {"order": "_id DESC", "limit": 50},
    "insert": true,
    "update": true,
    "delete": true
  },
  "config": {
    "table": "blog_config",
    "map": {"key": "key", "value": "value"}
  }
}
```

| Field | Description |
|-------|-------------|
| `table` | Source table name (defaults to resource name) |
| `slug` | Column used for slug lookups (default: `slug`) |
| `filter` | WHERE clause applied to all reads |
| `fields` | Columns to return (default: all) |
| `get` | Enable get-by-slug/id (default: `true`) |
| `list` | Enable listing — object with `order` and `limit`, or `true` for defaults |
| `insert` | Enable inserts (default: `true`) |
| `update` | Enable updates (default: `true`) |
| `delete` | Enable deletes (default: `true`) |
| `map` | Key-value mode — object with `key` and `value` column names |

Without `api.json`, all tables get default CRUD (get by `_id`, list by `_id DESC`, limit 50).

## Goop.identity

```javascript
const info  = await Goop.identity.get();    // {id, label, email}
const myId  = await Goop.identity.id();     // peer ID string
const name  = await Goop.identity.label();  // display name
const email = await Goop.identity.email();  // email string
Goop.identity.refresh();                    // clear cache, force re-fetch
```

## Goop.peers

```javascript
// One-time fetch
const peers = await Goop.peers.list();
// Each peer: {ID, Content, Email, AvatarHash, VideoDisabled,
//   ActiveTemplate, Verified, Reachable, Offline, LastSeen}

// Live updates (polls every 5s by default)
Goop.peers.subscribe({
  onSnapshot(peers) { /* full list on first load */ },
  onUpdate(peerId, peer) { /* peer came online or changed */ },
  onRemove(peerId) { /* peer pruned from list */ },
  onError() { /* optional error handler */ }
}, 5000);

// Stop polling
Goop.peers.unsubscribe();
```

## Goop.group

```javascript
// Create a hosted group
await Goop.group.create("My Room", "realtime", 10);

// List hosted groups
const groups = await Goop.group.list();
// Each: {id, name, app_type, max_members, volatile, host_joined,
//   host_in_group, created_at, member_count, members}

// Join a remote group
await Goop.group.join(hostPeerId, groupId);

// Send a message to the group
await Goop.group.send({action: "move", x: 5}, groupId);

// Leave a group
await Goop.group.leave();

// Host joins/leaves own group
await Goop.group.joinOwn(groupId);
await Goop.group.leaveOwn(groupId);

// Close a hosted group
await Goop.group.close(groupId);

// List subscriptions and active connections
const {subscriptions, active_groups} = await Goop.group.subscriptions();

// SSE event stream
const es = Goop.group.subscribe(function(evt) {
  // evt.type: "welcome", "members", "msg", "state", "leave", "close", "invite"
  // evt.group, evt.from, evt.payload
});

Goop.group.unsubscribe();
```

## Goop.chat

```javascript
// Direct message
await Goop.chat.send(peerId, "Hello!");

// Broadcast to all peers
await Goop.chat.broadcast("Server restarting");

// Subscribe to incoming messages via MQ
Goop.chat.subscribe(function(msg) {
  // msg: {from, content, type, timestamp}
  // type: "broadcast" or "direct"
});

Goop.chat.unsubscribe();
```

## Goop.realtime

Virtual MQ-based channels for real-time peer communication.

```javascript
// Connect to a peer — creates a channel
const ch = await Goop.realtime.connect(peerId);

// Channel methods
ch.send({action: "ping"});
ch.onMessage(function(msg, env) { /* env: {channel, from} */ });
ch.offMessage(handler);
ch.close();
// Properties: ch.id, ch.remotePeer

// Accept an incoming channel
const ch = await Goop.realtime.accept(channelId, hostPeerId);

// List active channels
const channels = await Goop.realtime.channels();

// Listen for incoming channel invitations
Goop.realtime.onIncoming(function(info) {
  // info: {channelId, hostPeerId}
});

// Global message handler (all channels)
Goop.realtime.onMessage(handler);
Goop.realtime.offMessage(handler);

// Check if MQ subscription is active
Goop.realtime.isConnected();
```

## Goop.call

WebRTC audio/video calling.

```javascript
// Start a call
const session = await Goop.call.start(peerId, {video: true, audio: true});

// Session methods
session.onRemoteStream(function(stream) { video.srcObject = stream; });
session.onHangup(function() { /* call ended */ });
session.onStateChange(function(state) { /* ICE state change */ });
session.hangup();
session.toggleAudio();  // returns enabled state
session.toggleVideo();  // returns enabled state
// Properties: session.channelId, session.remotePeer, session.isInitiator,
//   session.localStream, session.remoteStream

// Listen for incoming calls
Goop.call.onIncoming(function(info) {
  // info: {channelId, peerId, constraints}
  const session = await info.accept({video: true, audio: true});
  // or: info.reject();
});

// Get active calls
const session = Goop.call.getCall(channelId);
const all = Goop.call.activeCalls();
```

## Goop.site

File storage for the peer's site content directory.

```javascript
// List files
const files = await Goop.site.files();
// Each: {Path, IsDir, Depth}

// Read a file as text
const content = await Goop.site.read("data.json");

// Fetch as raw Response (for images, binary)
const response = await Goop.site.fetch("image.png");

// Upload a file (owner only)
await Goop.site.upload("data.json", fileOrBlob);

// Delete a file (owner only)
await Goop.site.remove("data.json");
```

## Goop.ui

Portable UI helpers. Auto-injects minimal CSS.

```javascript
// Toast notifications
Goop.ui.toast("Saved!");
Goop.ui.toast({title: "Error", message: "Something failed", duration: 6000});
// duration: ms, 0 = permanent

// Confirm dialog → true or null
const ok = await Goop.ui.confirm("Delete this item?", "Confirm");

// Prompt dialog → string or null
const name = await Goop.ui.prompt("Enter your name:", "Default", "Title");

// Get current theme
const theme = Goop.ui.theme();  // "dark" or "light"
```

## Goop.form

JSON-driven form renderer. Requires `goop-data.js` and `goop-identity.js`.

```javascript
await Goop.form.render(document.getElementById("form"), {
  table: "responses",
  fields: [
    {name: "name",    label: "Your Name", type: "text",     required: true},
    {name: "rating",  label: "Rating",    type: "number"},
    {name: "comment", label: "Comment",   type: "textarea"},
    {name: "color",   label: "Color",     type: "select",   options: ["Red", "Blue", "Green"]},
    {name: "agree",   label: "I agree",   type: "checkbox"},
    {name: "size",    label: "Size",      type: "radio",    options: ["S", "M", "L"]}
  ],
  submitLabel: "Send",
  singleResponse: true,  // one per peer (lookup by _owner)
  onDone: function() { /* callback after save */ }
});
```

## Goop.forms

Auto-generated CRUD UI from table schemas. Requires `goop-data.js` and `goop-ui.js`.

```javascript
// Full CRUD interface (list + insert + edit + delete)
await Goop.forms.render(document.getElementById("crud"), "tasks");

// Insert form only
await Goop.forms.insertForm(document.getElementById("form"), "tasks", function() {
  // called after row inserted
});
```

## Goop.drag

Reusable drag-and-drop with sortable lists. Auto-injects CSS.

```javascript
const instance = Goop.drag.sortable(container, {
  items: "> .card",          // selector for draggable children
  handle: ".drag-handle",   // optional handle selector
  group: "kanban",          // items move between containers sharing a group
  direction: "vertical",    // or "horizontal"
  placeholder: true,
  onStart(evt) { /* {item, container, index} */ },
  onMove(evt)  { /* {item, from, to, oldIndex, newIndex} */ },
  onEnd(evt)   { /* {item, from, to, oldIndex, newIndex} */ },
  onCancel(evt) { /* drag aborted (Escape) */ }
});

instance.destroy();
```

## Goop Engine

2D game engine for Canvas-based templates. Not namespaced under `Goop` — exposes global classes.

```javascript
// Core loop
const loop = new GameLoop(60);  // tick rate
loop.start(update, render);
loop.stop();

// Rendering
const renderer = new Renderer(canvas);
renderer.clear("#000");
renderer.drawRect(x, y, w, h, color);
renderer.drawCircle(x, y, radius, color);
renderer.drawSprite(image, sx, sy, sw, sh, dx, dy, dw, dh);
renderer.drawText(text, x, y, font, color, align);
renderer.drawTextCentered(text, y, font, color);

// Sprites
const sheet = new SpriteSheet("sprites.png", 16, 16);
sheet.draw(renderer, col, row, x, y, scale);

// Input
const input = new Input();
input.bind(canvas);
input.isDown("ArrowLeft");
input.justPressed("Space");
input.update();  // call each frame

// Collision (static)
Collision.rectRect(a, b);
Collision.pointRect(px, py, rect);
Collision.circleCircle(a, b);

// Tile map
const map = new TileMap(data2d, 16, 16, sheet);
map.draw(renderer, offsetX, offsetY);
map.getTileAt(worldX, worldY);
map.isSolid(worldX, worldY);
map.collideRect(rect);

// Audio
const audio = new GameAudio();
audio.load("jump", "sfx/jump.wav");
audio.play("jump");
audio.toggleMute();

// Scenes
const scenes = new SceneManager();
scenes.add("menu", { enter(data){}, update(dt){}, render(r){}, exit(){} });
scenes.switch("menu");

// Entity base class
const e = new Entity(x, y, w, h);
e.vx = 100; e.update(dt);
e.collidesWith(other);

// Utils
Utils.clamp(val, min, max);
Utils.lerp(a, b, t);
Utils.random(min, max);
Utils.distance(x1, y1, x2, y2);
```

## Context awareness

The SDK automatically detects whether the template is running on the local peer (`/`) or viewing a remote peer (`/p/<peerID>/`). API calls are routed to the correct peer transparently. The same template code works in both contexts.
